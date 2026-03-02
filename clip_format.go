package main

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// .asciirec file magic
var clipMagic = [4]byte{'A', 'R', 'E', 'C'}

const clipVersion = 1

// ClipHeader is the 32-byte file header.
type ClipHeader struct {
	Magic      [4]byte
	Version    uint8
	FPS        uint8
	NumFrames  uint16
	NumScales  uint8
	BaseWidth  uint16
	BaseHeight uint16
	DurationMs uint32
	Reserved   [13]byte
}

// ScaleEntry is the 8-byte per-scale table entry.
type ScaleEntry struct {
	Width      uint16
	Height     uint16
	DataOffset uint32 // offset from start of frame data section
}

// Keyframe holds the state snapshot captured during live recording (~48 bytes on disk).
type Keyframe struct {
	TimeMs      uint32
	ShaderTime  float32
	CamAngleX   float32
	CamAngleY   float32
	CamDist     float32
	CamTargetX  float32
	CamTargetY  float32
	CamTargetZ  float32
	Contrast    float32
	Ambient     float32
	SpecPower   float32
	ShadowSteps uint16
	AOSteps     uint16
}

// ClipCell is a single cell in the clip format: character + RGB565 color.
type ClipCell struct {
	Ch    byte
	Color uint16 // RGB565
}

// ClipCellBytes is the on-disk size of one cell.
const ClipCellBytes = 3

// RGB565Encode converts 0-1 float RGB to RGB565.
func RGB565Encode(r, g, b float64) uint16 {
	ri := uint16(clamp(r, 0, 1) * 31)
	gi := uint16(clamp(g, 0, 1) * 63)
	bi := uint16(clamp(b, 0, 1) * 31)
	return (ri << 11) | (gi << 5) | bi
}

// RGB565Decode converts RGB565 to 0-255 integer RGB.
func RGB565Decode(c uint16) (r, g, b uint8) {
	r5 := (c >> 11) & 0x1F
	g6 := (c >> 5) & 0x3F
	b5 := c & 0x1F
	r = uint8((r5*255 + 15) / 31)
	g = uint8((g6*255 + 31) / 63)
	b = uint8((b5*255 + 15) / 31)
	return
}

// CellToClipCell converts a render cell to a clip cell.
func CellToClipCell(c cell) ClipCell {
	return ClipCell{
		Ch:    c.ch,
		Color: RGB565Encode(c.col.X, c.col.Y, c.col.Z),
	}
}

// ScaleTrack holds the decompressed frame data for one scale.
type ScaleTrack struct {
	Width  int
	Height int
	Frames [][]ClipCell // each frame is Width*Height cells
}

// Clip is a fully loaded .asciirec file.
type Clip struct {
	Header    ClipHeader
	Scales    []ScaleEntry
	Keyframes []Keyframe
	Tracks    []ScaleTrack // one per scale, decompressed on load
}

// WriteClip writes a clip to the given path.
func WriteClip(path string, header ClipHeader, scales []ScaleEntry, keyframes []Keyframe, trackData [][]byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// Write header
	header.Magic = clipMagic
	header.Version = clipVersion
	if err := binary.Write(f, binary.LittleEndian, &header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	// Write scale table (placeholder offsets, fill after)
	scaleTablePos, _ := f.Seek(0, io.SeekCurrent)
	for i := range scales {
		if err := binary.Write(f, binary.LittleEndian, &scales[i]); err != nil {
			return fmt.Errorf("write scale %d: %w", i, err)
		}
	}

	// Write keyframes
	for i := range keyframes {
		if err := binary.Write(f, binary.LittleEndian, &keyframes[i]); err != nil {
			return fmt.Errorf("write keyframe %d: %w", i, err)
		}
	}

	// Frame data section start
	frameDataStart, _ := f.Seek(0, io.SeekCurrent)

	// Write each scale's compressed data and record offsets
	for i, data := range trackData {
		offset, _ := f.Seek(0, io.SeekCurrent)
		scales[i].DataOffset = uint32(offset - frameDataStart)
		if _, err := f.Write(data); err != nil {
			return fmt.Errorf("write track %d: %w", i, err)
		}
	}

	// Go back and fix up scale table with correct offsets
	if _, err := f.Seek(scaleTablePos, io.SeekStart); err != nil {
		return err
	}
	for i := range scales {
		if err := binary.Write(f, binary.LittleEndian, &scales[i]); err != nil {
			return fmt.Errorf("fixup scale %d: %w", i, err)
		}
	}

	return nil
}

// LoadClip loads a .asciirec file and decompresses all scale tracks.
func LoadClip(path string) (*Clip, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return LoadClipFromBytes(data)
}

// LoadClipFromBytes parses a .asciirec from a byte slice.
func LoadClipFromBytes(data []byte) (*Clip, error) {
	r := bytes.NewReader(data)

	var header ClipHeader
	if err := binary.Read(r, binary.LittleEndian, &header); err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	if header.Magic != clipMagic {
		return nil, fmt.Errorf("bad magic: %v", header.Magic)
	}
	if header.Version != clipVersion {
		return nil, fmt.Errorf("unsupported version: %d", header.Version)
	}

	// Read scale table
	scales := make([]ScaleEntry, header.NumScales)
	for i := range scales {
		if err := binary.Read(r, binary.LittleEndian, &scales[i]); err != nil {
			return nil, fmt.Errorf("read scale %d: %w", i, err)
		}
	}

	// Read keyframes
	keyframes := make([]Keyframe, header.NumFrames)
	for i := range keyframes {
		if err := binary.Read(r, binary.LittleEndian, &keyframes[i]); err != nil {
			return nil, fmt.Errorf("read keyframe %d: %w", i, err)
		}
	}

	// Frame data section starts here
	frameDataStart, _ := r.Seek(0, io.SeekCurrent)

	clip := &Clip{
		Header:    header,
		Scales:    scales,
		Keyframes: keyframes,
		Tracks:    make([]ScaleTrack, header.NumScales),
	}

	// Decompress each scale track
	for i, se := range scales {
		w := int(se.Width)
		h := int(se.Height)

		// Find compressed data bounds
		start := frameDataStart + int64(se.DataOffset)
		var end int64
		if i+1 < len(scales) {
			end = frameDataStart + int64(scales[i+1].DataOffset)
		} else {
			end = int64(len(data))
		}

		compData := data[start:end]
		zr, err := zlib.NewReader(bytes.NewReader(compData))
		if err != nil {
			return nil, fmt.Errorf("zlib open scale %d: %w", i, err)
		}
		rawData, err := io.ReadAll(zr)
		zr.Close()
		if err != nil {
			return nil, fmt.Errorf("zlib read scale %d: %w", i, err)
		}

		frames, err := DecodeTrack(rawData, w, h, int(header.NumFrames))
		if err != nil {
			return nil, fmt.Errorf("decode scale %d: %w", i, err)
		}

		clip.Tracks[i] = ScaleTrack{
			Width:  w,
			Height: h,
			Frames: frames,
		}
	}

	return clip, nil
}

// CompressTrack zlib-compresses raw frame data for one scale.
func CompressTrack(rawData []byte) ([]byte, error) {
	var buf bytes.Buffer
	w, err := zlib.NewWriterLevel(&buf, zlib.BestCompression)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(rawData); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
