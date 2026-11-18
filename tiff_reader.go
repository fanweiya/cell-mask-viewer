package main

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/google/tiff"
)

// InstanceMap holds the decoded 2D instance label grid.
type InstanceMap struct {
	Width      int
	Height     int
	TotalCells int
	Channel    string
	Labels     []uint32 // Row-major: Labels[y*Width + x]
}

type tiffMetadata struct {
	CellNumber int   `json:"cell_number"`
	TotalCells int   `json:"total_cells"`
	Boxs       []int `json:"boxs"` // [tx, ty, bx, by]
	Shape      []int `json:"shape"`
	Channels   any   `json:"channels"`
}

// ReadTIFFMask parses the given TIFF file using github.com/google/tiff and zlib decompression.
func ReadTIFFMask(filePath string) *InstanceMap {
	f, err := os.Open(filePath)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	parsed, err := tiff.Parse(f, nil, nil)
	if err != nil {
		panic(err)
	}

	ifds := parsed.IFDs()
	if len(ifds) == 0 {
		panic(fmt.Errorf("tiff: no IFD found in %s", filePath))
	}

	firstIFD := ifds[0]
	firstMeta := extractMeta(firstIFD)

	if len(ifds) > 1 && len(firstMeta.Boxs) == 4 {
		return decodeMultiPageCrop(f, ifds)
	}
	return decodeSinglePage(f, firstIFD, firstMeta)
}

func extractMeta(ifd tiff.IFD) tiffMetadata {
	var meta tiffMetadata
	descField := ifd.GetField(270) // ImageDescription
	if descField == nil {
		return meta
	}
	descBytes := descField.Value().Bytes()
	cleanStr := string(bytes.TrimRight(descBytes, "\x00"))
	if len(cleanStr) > 0 {
		_ = json.Unmarshal([]byte(cleanStr), &meta)
	}
	return meta
}

func getFieldUint(ifd tiff.IFD, tagID uint16) uint32 {
	f := ifd.GetField(tagID)
	if f == nil {
		return 0
	}
	v := f.Value()
	b := v.Bytes()
	if len(b) < 2 {
		return 0
	}
	if len(b) == 2 {
		return uint32(v.Order().Uint16(b))
	}
	return v.Order().Uint32(b[:4])
}

func getFieldUintArray(ifd tiff.IFD, tagID uint16) []uint32 {
	f := ifd.GetField(tagID)
	if f == nil {
		return nil
	}
	v := f.Value()
	b := v.Bytes()
	count := int(f.Count())
	result := make([]uint32, count)

	elemSize := len(b) / count
	for i := 0; i < count; i++ {
		start := i * elemSize
		if elemSize == 2 {
			result[i] = uint32(v.Order().Uint16(b[start : start+2]))
		} else {
			result[i] = v.Order().Uint32(b[start : start+4])
		}
	}
	return result
}

func readStrips(r io.ReaderAt, ifd tiff.IFD) []byte {
	offsets := getFieldUintArray(ifd, 273)    // StripOffsets
	byteCounts := getFieldUintArray(ifd, 279) // StripByteCounts
	compression := getFieldUint(ifd, 259)    // Compression

	var output bytes.Buffer
	for i := 0; i < len(offsets); i++ {
		stripBuf := make([]byte, byteCounts[i])
		_, err := r.ReadAt(stripBuf, int64(offsets[i]))
		if err != nil {
			panic(err)
		}

		if compression == 8 || compression == 32946 { // Adobe Deflate / zlib
			zr, err := zlib.NewReader(bytes.NewReader(stripBuf))
			if err != nil {
				panic(err)
			}
			_, err = io.Copy(&output, zr)
			zr.Close()
			if err != nil {
				panic(err)
			}
		} else {
			output.Write(stripBuf)
		}
	}
	return output.Bytes()
}

func decodeSinglePage(r io.ReaderAt, ifd tiff.IFD, meta tiffMetadata) *InstanceMap {
	width := int(getFieldUint(ifd, 256))
	height := int(getFieldUint(ifd, 257))
	bits := int(getFieldUint(ifd, 258))
	order := ifd.GetField(256).Value().Order()

	raw := readStrips(r, ifd)
	labels := make([]uint32, width*height)

	if bits == 16 {
		for i := 0; i < len(labels); i++ {
			labels[i] = uint32(order.Uint16(raw[i*2 : i*2+2]))
		}
	} else if bits == 32 {
		for i := 0; i < len(labels); i++ {
			labels[i] = order.Uint32(raw[i*4 : i*4+4])
		}
	} else {
		for i := 0; i < len(labels); i++ {
			labels[i] = uint32(raw[i])
		}
	}

	return &InstanceMap{
		Width:      width,
		Height:     height,
		TotalCells: meta.TotalCells,
		Labels:     labels,
	}
}

func decodeMultiPageCrop(r io.ReaderAt, ifds []tiff.IFD) *InstanceMap {
	type cropInfo struct {
		cellNumber int
		tx, ty     int
		bw, bh     int
		mask       []byte
	}

	crops := make([]cropInfo, 0, len(ifds))
	maxX, maxY := 0, 0

	for _, ifd := range ifds {
		meta := extractMeta(ifd)
		bw := int(getFieldUint(ifd, 256))
		bh := int(getFieldUint(ifd, 257))
		raw := readStrips(r, ifd)

		tx, ty := meta.Boxs[0], meta.Boxs[1]
		bx, by := meta.Boxs[2], meta.Boxs[3]
		if bx > maxX {
			maxX = bx
		}
		if by > maxY {
			maxY = by
		}

		crops = append(crops, cropInfo{
			cellNumber: meta.CellNumber,
			tx:         tx,
			ty:         ty,
			bw:         bw,
			bh:         bh,
			mask:       raw,
		})
	}

	labels := make([]uint32, maxX*maxY)
	for _, c := range crops {
		for row := 0; row < c.bh; row++ {
			for col := 0; col < c.bw; col++ {
				if c.mask[row*c.bw+col] > 0 {
					gy := c.ty + row
					gx := c.tx + col
					if gy < maxY && gx < maxX {
						labels[gy*maxX+gx] = uint32(c.cellNumber)
					}
				}
			}
		}
	}

	return &InstanceMap{
		Width:      maxX,
		Height:     maxY,
		TotalCells: len(crops),
		Labels:     labels,
	}
}
