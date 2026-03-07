package clip

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"

	"github.com/klauspost/compress/zstd"
)

// Format constants.
const (
	FormatVersion = 2
	SubW          = 2           // sub-pixel columns per cell
	SubH          = 4           // sub-pixel rows per cell
	SubPixels     = SubW * SubH // 8 brightness values per cell
	NumPlanes     = SubPixels + 2

	// PlaybackModeCount is the number of render modes available during clip playback.
	// Shapes(0), Blocks(1), Braille(2) — Slice and Cost require GPU data.
	PlaybackModeCount = 3
)

var clipMagic = [4]byte{'A', 'R', 'E', 'C'}

// ClipHeader is the file header.
type ClipHeader struct {
	Magic      [4]byte
	Version    uint8
	FPS        uint8
	NumFrames  uint16
	Width      uint16
	Height     uint16
	DurationMs uint32
}

// Keyframe holds the state snapshot captured during live recording.
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

// FrameSize returns the byte size of one frame for the given cell dimensions.
func FrameSize(w, h int) int {
	return w * h * NumPlanes
}

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// RGB565Encode converts 0-1 float RGB to RGB565.
func RGB565Encode(r, g, b float64) uint16 {
	ri := uint16(clampF(r, 0, 1) * 31)
	gi := uint16(clampF(g, 0, 1) * 63)
	bi := uint16(clampF(b, 0, 1) * 31)
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

// Clip is a fully loaded .asciirec file.
type Clip struct {
	Header    ClipHeader
	Keyframes []Keyframe
	Width     int
	Height    int
	Frames    [][]byte // [frameIdx] = planar frame data, FrameSize bytes each
}

// WriteClip writes a clip to the given path.
// trackData must already be zstd-compressed.
func WriteClip(path string, header ClipHeader, keyframes []Keyframe, trackData []byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	header.Magic = clipMagic
	header.Version = FormatVersion
	if err := binary.Write(f, binary.LittleEndian, &header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	for i := range keyframes {
		if err := binary.Write(f, binary.LittleEndian, &keyframes[i]); err != nil {
			return fmt.Errorf("write keyframe %d: %w", i, err)
		}
	}

	if _, err := f.Write(trackData); err != nil {
		return fmt.Errorf("write track data: %w", err)
	}

	return nil
}

// LoadClip loads a .asciirec file.
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
	if header.Version != FormatVersion {
		return nil, fmt.Errorf("unsupported format version %d (want %d)", header.Version, FormatVersion)
	}

	keyframes := make([]Keyframe, header.NumFrames)
	for i := range keyframes {
		if err := binary.Read(r, binary.LittleEndian, &keyframes[i]); err != nil {
			return nil, fmt.Errorf("read keyframe %d: %w", i, err)
		}
	}

	compStart, _ := r.Seek(0, io.SeekCurrent)
	compData := data[compStart:]

	rawData, err := DecompressTrack(compData)
	if err != nil {
		return nil, fmt.Errorf("decompress: %w", err)
	}

	w := int(header.Width)
	h := int(header.Height)
	frames, err := DecodeTrack(rawData, w, h, int(header.NumFrames))
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	return &Clip{
		Header:    header,
		Keyframes: keyframes,
		Width:     w,
		Height:    h,
		Frames:    frames,
	}, nil
}

// CompressTrack zstd-compresses raw frame data.
func CompressTrack(rawData []byte) ([]byte, error) {
	enc, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedBestCompression))
	if err != nil {
		return nil, err
	}
	result := enc.EncodeAll(rawData, make([]byte, 0, len(rawData)/2))
	_ = enc.Close()
	return result, nil
}

// DecompressTrack zstd-decompresses track data.
func DecompressTrack(compData []byte) ([]byte, error) {
	dec, err := zstd.NewReader(nil)
	if err != nil {
		return nil, err
	}
	defer dec.Close()
	return dec.DecodeAll(compData, nil)
}

// PackFrame packs sub-pixel brightness and color data into a planar frame.
// brightness has SubPixels (8) values per cell, colors has one RGB565 per cell.
// Layout: 8 brightness planes + 2 color byte planes, each W*H bytes.
func PackFrame(brightness []uint8, colors []uint16, w, h int) []byte {
	cellCount := w * h
	frame := make([]byte, cellCount*NumPlanes)

	// 8 brightness planes
	for sp := 0; sp < SubPixels; sp++ {
		for i := 0; i < cellCount; i++ {
			frame[sp*cellCount+i] = brightness[i*SubPixels+sp]
		}
	}

	// 2 color byte planes (low, high)
	for i := 0; i < cellCount; i++ {
		frame[SubPixels*cellCount+i] = byte(colors[i])
		frame[(SubPixels+1)*cellCount+i] = byte(colors[i] >> 8)
	}

	return frame
}

// FrameBrightness returns the brightness (0-255) of a sub-pixel within a frame.
// plane is the sub-pixel index (0-7), cellIdx is the cell index.
func FrameBrightness(frame []byte, cellCount, cellIdx, plane int) uint8 {
	return frame[plane*cellCount+cellIdx]
}

// FrameColor returns the RGB565 color of a cell within a frame.
func FrameColor(frame []byte, cellCount, cellIdx int) uint16 {
	lo := frame[SubPixels*cellCount+cellIdx]
	hi := frame[(SubPixels+1)*cellCount+cellIdx]
	return uint16(lo) | uint16(hi)<<8
}
