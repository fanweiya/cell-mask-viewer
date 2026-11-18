package main

import (
	"image"
	"image/color"
	"math"
)

// RenderMode defines how instances are visually rendered.
type RenderMode int

const (
	ModeFill RenderMode = iota
	ModeOutline
	ModeBoth
)

// RenderOptions holds configuration parameters for instance map rendering.
type RenderOptions struct {
	Mode        RenderMode
	Opacity     float64
	Seed        uint32
	StrokeWidth int
	MaskVisible bool
	SelectedID  uint32
}

// CellMetrics encapsulates morphological measurements for an individual cell instance.
type CellMetrics struct {
	ID        uint32
	Area      int
	MinX      int
	MinY      int
	MaxX      int
	MaxY      int
	CentroidX float64
	CentroidY float64
}

// ComputeCellMetrics calculates morphology statistics for all cell instances in an InstanceMap.
func ComputeCellMetrics(m *InstanceMap) map[uint32]*CellMetrics {
	metrics := make(map[uint32]*CellMetrics)
	w := m.Width
	h := m.Height
	sumX := make(map[uint32]int64)
	sumY := make(map[uint32]int64)

	for y := 0; y < h; y++ {
		rowOffset := y * w
		for x := 0; x < w; x++ {
			id := m.Labels[rowOffset+x]
			if id == 0 {
				continue
			}
			cm, exists := metrics[id]
			if !exists {
				cm = &CellMetrics{
					ID:   id,
					MinX: x,
					MinY: y,
					MaxX: x,
					MaxY: y,
				}
				metrics[id] = cm
			}
			cm.Area++
			sumX[id] += int64(x)
			sumY[id] += int64(y)
			if x < cm.MinX {
				cm.MinX = x
			}
			if x > cm.MaxX {
				cm.MaxX = x
			}
			if y < cm.MinY {
				cm.MinY = y
			}
			if y > cm.MaxY {
				cm.MaxY = y
			}
		}
	}

	for id, cm := range metrics {
		if cm.Area > 0 {
			cm.CentroidX = float64(sumX[id]) / float64(cm.Area)
			cm.CentroidY = float64(sumY[id]) / float64(cm.Area)
		}
	}
	return metrics
}

// GoldenRatioColor calculates distinct RGB color from instance ID and random seed.
func GoldenRatioColor(id uint32, seed uint32) color.RGBA {
	if id == 0 {
		return color.RGBA{R: 0, G: 0, B: 0, A: 0}
	}
	const phi = 0.618033988749895
	offset := float64(seed) * 97.6180339887
	h := math.Mod((float64(id)+offset)*phi, 1.0)
	if h < 0 {
		h += 1.0
	}
	s := 0.85
	v := 0.95

	i := math.Floor(h * 6)
	f := h*6 - i
	p := v * (1 - s)
	q := v * (1 - f*s)
	t := v * (1 - (1-f)*s)

	var rf, gf, bf float64
	switch int(i) % 6 {
	case 0:
		rf, gf, bf = v, t, p
	case 1:
		rf, gf, bf = q, v, p
	case 2:
		rf, gf, bf = p, v, t
	case 3:
		rf, gf, bf = p, q, v
	case 4:
		rf, gf, bf = t, p, v
	case 5:
		rf, gf, bf = v, p, q
	}
	return color.RGBA{
		R: uint8(rf * 255),
		G: uint8(gf * 255),
		B: uint8(bf * 255),
		A: 255,
	}
}

// RenderInstances generates an RGBA image from the InstanceMap with configured options and optional base image.
func RenderInstances(m *InstanceMap, opts RenderOptions, baseImg image.Image) *image.RGBA {
	w := m.Width
	h := m.Height
	rgba := image.NewRGBA(image.Rect(0, 0, w, h))

	if !opts.MaskVisible {
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				if baseImg != nil {
					pr, pg, pb, pa := baseImg.At(x, y).RGBA()
					rgba.SetRGBA(x, y, color.RGBA{
						R: uint8(pr >> 8),
						G: uint8(pg >> 8),
						B: uint8(pb >> 8),
						A: uint8(pa >> 8),
					})
				} else {
					rgba.SetRGBA(x, y, color.RGBA{R: 20, G: 20, B: 24, A: 255})
				}
			}
		}
		return rgba
	}

	fillAlpha := uint8(math.Round(opts.Opacity * 255))
	strokeWidth := opts.StrokeWidth
	if strokeWidth <= 0 {
		strokeWidth = 1
	}

	for y := 0; y < h; y++ {
		rowOffset := y * w
		for x := 0; x < w; x++ {
			idx := rowOffset + x
			id := m.Labels[idx]

			var br, bg, bb, ba uint8
			if baseImg != nil {
				pr, pg, pb, pa := baseImg.At(x, y).RGBA()
				br = uint8(pr >> 8)
				bg = uint8(pg >> 8)
				bb = uint8(pb >> 8)
				ba = uint8(pa >> 8)
			}

			if id == 0 {
				if baseImg != nil {
					rgba.SetRGBA(x, y, color.RGBA{R: br, G: bg, B: bb, A: ba})
				}
				continue
			}

			isSelected := (opts.SelectedID > 0 && id == opts.SelectedID)
			col := GoldenRatioColor(id, opts.Seed)

			isEdge := false
			if opts.Mode == ModeOutline || opts.Mode == ModeBoth || isSelected {
				if strokeWidth <= 1 {
					if x == 0 || x == w-1 || y == 0 || y == h-1 {
						isEdge = true
					} else if m.Labels[idx-1] != id || m.Labels[idx+1] != id ||
						m.Labels[idx-w] != id || m.Labels[idx+w] != id {
						isEdge = true
					}
				} else {
					if x < 2 || x >= w-2 || y < 2 || y >= h-2 {
						isEdge = true
					} else if m.Labels[idx-1] != id || m.Labels[idx+1] != id ||
						m.Labels[idx-w] != id || m.Labels[idx+w] != id ||
						m.Labels[idx-2] != id || m.Labels[idx+2] != id ||
						m.Labels[idx-2*w] != id || m.Labels[idx+2*w] != id ||
						m.Labels[idx-w-1] != id || m.Labels[idx-w+1] != id ||
						m.Labels[idx+w-1] != id || m.Labels[idx+w+1] != id {
						isEdge = true
					}
				}
			}

			var fr, fg, fb, fa uint8
			if isSelected {
				if isEdge {
					fr, fg, fb, fa = 0, 255, 255, 255
				} else if opts.Mode == ModeOutline {
					fr, fg, fb, fa = 0, 220, 240, 80
				} else {
					fr, fg, fb, fa = 0, 230, 255, 220
				}
			} else {
				if isEdge {
					fr, fg, fb, fa = col.R, col.G, col.B, 255
				} else if opts.Mode == ModeOutline {
					fr, fg, fb, fa = br, bg, bb, ba
				} else if opts.Mode == ModeBoth {
					fr, fg, fb, fa = col.R, col.G, col.B, uint8(float64(fillAlpha) * 0.5)
				} else {
					fr, fg, fb, fa = col.R, col.G, col.B, fillAlpha
				}
			}

			if baseImg != nil && fa < 255 {
				alphaF := float64(fa) / 255.0
				invAlphaF := 1.0 - alphaF
				fr = uint8(float64(fr)*alphaF + float64(br)*invAlphaF)
				fg = uint8(float64(fg)*alphaF + float64(bg)*invAlphaF)
				fb = uint8(float64(fb)*alphaF + float64(bb)*invAlphaF)
				fa = 255
			}

			rgba.SetRGBA(x, y, color.RGBA{R: fr, G: fg, B: fb, A: fa})
		}
	}
	return rgba
}
