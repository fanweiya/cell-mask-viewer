package main

import (
	"testing"
)

func TestReadSinglePageTIFF(t *testing.T) {
	path := `C:\算法\AIsoftware2\tests_images\cellsegments\result\test_23700\B2_13_source_BF_mask_multimasks.tif`
	m := ReadTIFFMask(path)
	if m.Width != 3200 || m.Height != 2176 {
		t.Fatalf("unexpected dimensions: %dx%d", m.Width, m.Height)
	}
	if m.TotalCells != 1091 {
		t.Fatalf("unexpected cell count: %d", m.TotalCells)
	}

	rgba := RenderInstances(m, ModeOutline, 0.5, nil)
	if rgba.Bounds().Dx() != 3200 || rgba.Bounds().Dy() != 2176 {
		t.Fatalf("unexpected rendered bounds: %v", rgba.Bounds())
	}
}

func TestReadMultiPageCropTIFF(t *testing.T) {
	path := `C:\算法\AIsoftware2\tests_images\blue_foci\result\A1_13_output_BF_multimasks.tif`
	m := ReadTIFFMask(path)
	if m.TotalCells != 19 {
		t.Fatalf("unexpected cell count: %d", m.TotalCells)
	}
	if m.Width == 0 || m.Height == 0 {
		t.Fatalf("zero dimensions: %dx%d", m.Width, m.Height)
	}

	rgba := RenderInstances(m, ModeBoth, 0.5, nil)
	if rgba.Bounds().Dx() != m.Width || rgba.Bounds().Dy() != m.Height {
		t.Fatalf("unexpected rendered bounds: %v", rgba.Bounds())
	}
}
