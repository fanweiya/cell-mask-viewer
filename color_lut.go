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

// GoldenRatioColor calculates distinct RGB color from instance ID.
func GoldenRatioColor(id uint32) color.RGBA {
	if id == 0 {
		return color.RGBA{R: 0, G: 0, B: 0, A: 0}
	}
	const phi = 0.618033988749895
	h := math.Mod(float64(id)*phi, 1.0)
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

// RenderInstances generates an RGBA image from the InstanceMap and optional base image.
func RenderInstances(m *InstanceMap, mode RenderMode, opacity float64, baseImg image.Image) *image.RGBA {
	w := m.Width
	h := m.Height
	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	fillAlpha := uint8(math.Round(opacity * 255))

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			idx := y*w + x
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

			col := GoldenRatioColor(id)

			isEdge := false
			if mode == ModeOutline || mode == ModeBoth {
				if x == 0 || x == w-1 || y == 0 || y == h-1 {
					isEdge = true
				} else if m.Labels[idx-1] != id || m.Labels[idx+1] != id ||
					m.Labels[idx-w] != id || m.Labels[idx+w] != id {
					isEdge = true
				}
			}

			var fr, fg, fb, fa uint8
			if isEdge {
				fr, fg, fb, fa = col.R, col.G, col.B, 255
			} else if mode == ModeOutline {
				fr, fg, fb, fa = br, bg, bb, ba
			} else if mode == ModeBoth {
				fr, fg, fb, fa = col.R, col.G, col.B, uint8(float64(fillAlpha) * 0.5)
			} else {
				fr, fg, fb, fa = col.R, col.G, col.B, fillAlpha
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
