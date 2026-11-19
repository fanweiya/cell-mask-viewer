package main

import (
	"image"
	"image/color"
	"image/draw"
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

	// Fast pre-conversion of base image if present
	var baseRGBA *image.RGBA
	if baseImg != nil {
		if r, ok := baseImg.(*image.RGBA); ok && r.Rect.Dx() == w && r.Rect.Dy() == h {
			baseRGBA = r
		} else {
			baseRGBA = image.NewRGBA(image.Rect(0, 0, w, h))
			draw.Draw(baseRGBA, baseRGBA.Bounds(), baseImg, baseImg.Bounds().Min, draw.Src)
		}
	}

	if !opts.MaskVisible {
		if baseRGBA != nil {
			copy(rgba.Pix, baseRGBA.Pix)
		} else {
			for i := 0; i < len(rgba.Pix); i += 4 {
				rgba.Pix[i] = 20
				rgba.Pix[i+1] = 20
				rgba.Pix[i+2] = 24
				rgba.Pix[i+3] = 255
			}
		}
		return rgba
	}

	// Precompute Color Table once to avoid per-pixel float/HSV calculations
	maxID := uint32(m.TotalCells)
	for _, id := range m.Labels {
		if id > maxID {
			maxID = id
		}
	}
	colorTable := make([]color.RGBA, maxID+1)
	for i := uint32(1); i <= maxID; i++ {
		colorTable[i] = GoldenRatioColor(i, opts.Seed)
	}

	fillAlpha := uint8(math.Round(opts.Opacity * 255))
	halfFillAlpha := uint8(float64(fillAlpha) * 0.5)
	strokeWidth := opts.StrokeWidth
	if strokeWidth <= 0 {
		strokeWidth = 1
	}

	labels := m.Labels
	outPix := rgba.Pix
	var basePix []byte
	baseStride := 0
	if baseRGBA != nil {
		basePix = baseRGBA.Pix
		baseStride = baseRGBA.Stride
	}
	outStride := rgba.Stride

	for y := 0; y < h; y++ {
		rowOffset := y * w
		pRow := y * outStride
		bRow := y * baseStride

		for x := 0; x < w; x++ {
			idx := rowOffset + x
			id := labels[idx]
			p := pRow + x*4

			if id == 0 {
				if basePix != nil {
					bp := bRow + x*4
					outPix[p] = basePix[bp]
					outPix[p+1] = basePix[bp+1]
					outPix[p+2] = basePix[bp+2]
					outPix[p+3] = basePix[bp+3]
				} else {
					outPix[p] = 20
					outPix[p+1] = 20
					outPix[p+2] = 24
					outPix[p+3] = 255
				}
				continue
			}

			var br, bg, bb uint8
			if basePix != nil {
				bp := bRow + x*4
				br = basePix[bp]
				bg = basePix[bp+1]
				bb = basePix[bp+2]
			}

			isSelected := (opts.SelectedID > 0 && id == opts.SelectedID)
			col := colorTable[id]

			isEdge := false
			if opts.Mode == ModeOutline || opts.Mode == ModeBoth || isSelected {
				if strokeWidth <= 1 {
					if x == 0 || x == w-1 || y == 0 || y == h-1 {
						isEdge = true
					} else if labels[idx-1] != id || labels[idx+1] != id ||
						labels[idx-w] != id || labels[idx+w] != id {
						isEdge = true
					}
				} else {
					if x < 2 || x >= w-2 || y < 2 || y >= h-2 {
						isEdge = true
					} else if labels[idx-1] != id || labels[idx+1] != id ||
						labels[idx-w] != id || labels[idx+w] != id ||
						labels[idx-2] != id || labels[idx+2] != id ||
						labels[idx-2*w] != id || labels[idx+2*w] != id ||
						labels[idx-w-1] != id || labels[idx-w+1] != id ||
						labels[idx+w-1] != id || labels[idx+w+1] != id {
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
					fr, fg, fb, fa = br, bg, bb, 255
				} else if opts.Mode == ModeBoth {
					fr, fg, fb, fa = col.R, col.G, col.B, halfFillAlpha
				} else {
					fr, fg, fb, fa = col.R, col.G, col.B, fillAlpha
				}
			}

			if basePix != nil && fa < 255 {
				alpha := int(fa)
				invAlpha := 255 - alpha
				fr = uint8((int(fr)*alpha + int(br)*invAlpha) / 255)
				fg = uint8((int(fg)*alpha + int(bg)*invAlpha) / 255)
				fb = uint8((int(fb)*alpha + int(bb)*invAlpha) / 255)
				fa = 255
			}

			outPix[p] = fr
			outPix[p+1] = fg
			outPix[p+2] = fb
			outPix[p+3] = fa
		}
	}
	return rgba
}
