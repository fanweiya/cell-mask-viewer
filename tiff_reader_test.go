package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadSinglePageTIFF(t *testing.T) {
	path := `C:\算法\AIsoftware2\tests_images\cellsegments\result\test_23700\B2_13_source_BF_mask_multimasks.tif`
	if _, err := os.Stat(path); err != nil {
		t.Skip("Single page TIFF sample not found, skipping integration test")
	}
	m, err := ReadTIFFMask(path)
	if err != nil {
		t.Fatalf("ReadTIFFMask failed: %v", err)
	}
	if m.Width != 3200 || m.Height != 2176 {
		t.Fatalf("unexpected dimensions: %dx%d", m.Width, m.Height)
	}
	if m.TotalCells != 1091 {
		t.Fatalf("unexpected cell count: %d", m.TotalCells)
	}

	opts := RenderOptions{
		Mode:        ModeOutline,
		Opacity:     0.5,
		StrokeWidth: 1,
		MaskVisible: true,
	}
	rgba := RenderInstances(m, opts, nil)
	if rgba.Bounds().Dx() != 3200 || rgba.Bounds().Dy() != 2176 {
		t.Fatalf("unexpected rendered bounds: %v", rgba.Bounds())
	}

	metrics := ComputeCellMetrics(m)
	if len(metrics) != 1091 {
		t.Fatalf("unexpected metrics count: %d", len(metrics))
	}
}

func TestReadMultiPageCropTIFF(t *testing.T) {
	path := `C:\算法\AIsoftware2\tests_images\blue_foci\result\A1_13_output_BF_multimasks.tif`
	if _, err := os.Stat(path); err != nil {
		t.Skip("Multi page crop TIFF sample not found, skipping integration test")
	}
	m, err := ReadTIFFMask(path)
	if err != nil {
		t.Fatalf("ReadTIFFMask failed: %v", err)
	}
	if m.TotalCells != 19 {
		t.Fatalf("unexpected cell count: %d", m.TotalCells)
	}
	if m.Width == 0 || m.Height == 0 {
		t.Fatalf("zero dimensions: %dx%d", m.Width, m.Height)
	}

	opts := RenderOptions{
		Mode:        ModeBoth,
		Opacity:     0.5,
		StrokeWidth: 2,
		MaskVisible: true,
		SelectedID:  1,
	}
	rgba := RenderInstances(m, opts, nil)
	if rgba.Bounds().Dx() != m.Width || rgba.Bounds().Dy() != m.Height {
		t.Fatalf("unexpected rendered bounds: %v", rgba.Bounds())
	}
}

func TestReadCorruptOrNonTIFFReturnsError(t *testing.T) {
	// Test nonexistent file
	_, err := ReadTIFFMask("nonexistent_file.tif")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}

	// Test non-TIFF file content
	tempFile := filepath.Join(t.TempDir(), "corrupt.tif")
	if err := os.WriteFile(tempFile, []byte("this is plain text, not a tiff image"), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	m, err := ReadTIFFMask(tempFile)
	if err == nil {
		t.Fatal("expected error for non-TIFF file, got nil")
	}
	if m != nil {
		t.Fatal("expected nil instance map for corrupt file")
	}
}

func BenchmarkRenderInstances_Synthetic4096(b *testing.B) {
	w, h := 4096, 4096
	labels := make([]uint32, w*h)
	// Simulate ~30,000 cells
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			cellIdx := (y/24)*170 + (x / 24)
			if cellIdx%3 != 0 {
				labels[y*w+x] = uint32(cellIdx + 1)
			}
		}
	}
	m := &InstanceMap{
		Width:      w,
		Height:     h,
		TotalCells: 30000,
		Labels:     labels,
	}
	opts := RenderOptions{
		Mode:        ModeBoth,
		Opacity:     0.5,
		StrokeWidth: 1,
		MaskVisible: true,
		Seed:        42,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = RenderInstances(m, opts, nil)
	}
}
