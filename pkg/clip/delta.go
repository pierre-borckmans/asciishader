package clip

import (
	"encoding/binary"
	"fmt"
)

// I-frame interval: every 60 frames is a full keyframe for seeking.
const iFrameInterval = 60

// IsIFrame returns true if frame index should be an I-frame.
func IsIFrame(frameIdx int) bool {
	return frameIdx%iFrameInterval == 0
}

// appendVarint appends a varint-encoded uint32.
func appendVarint(out []byte, v uint32) []byte {
	for v >= 0x80 {
		out = append(out, byte(v)|0x80)
		v >>= 7
	}
	return append(out, byte(v))
}

// readVarint reads a varint from data at pos.
func readVarint(data []byte, pos int) (uint32, int, error) {
	var v uint32
	var shift uint
	for i := 0; i < 5; i++ {
		if pos >= len(data) {
			return 0, pos, fmt.Errorf("varint: unexpected EOF at %d", pos)
		}
		b := data[pos]
		pos++
		v |= uint32(b&0x7F) << shift
		if b < 0x80 {
			return v, pos, nil
		}
		shift += 7
	}
	return 0, pos, fmt.Errorf("varint: too long at %d", pos)
}

// EncodeIFrame writes a full W×H grid as raw bytes.
// Each cell = 3 bytes: char (uint8) + color (uint16 LE).
func EncodeIFrame(grid []ClipCell) []byte {
	out := make([]byte, len(grid)*ClipCellBytes)
	for i, c := range grid {
		off := i * ClipCellBytes
		out[off] = c.Ch
		binary.LittleEndian.PutUint16(out[off+1:], c.Color)
	}
	return out
}

// EncodePFrame computes the delta between prev and curr grids.
// Format: numChanges(varint) + per change: deltaIndex(varint) + ch(u8) + xorColor(u16 LE).
func EncodePFrame(prev, curr []ClipCell) []byte {
	type change struct {
		idx      int
		ch       byte
		xorColor uint16
	}
	var changes []change
	for i := range curr {
		if i >= len(prev) || curr[i].Ch != prev[i].Ch || curr[i].Color != prev[i].Color {
			prevColor := uint16(0)
			if i < len(prev) {
				prevColor = prev[i].Color
			}
			changes = append(changes, change{
				idx:      i,
				ch:       curr[i].Ch,
				xorColor: curr[i].Color ^ prevColor,
			})
		}
	}

	out := appendVarint(nil, uint32(len(changes)))

	prevIdx := 0
	for j, c := range changes {
		delta := c.idx
		if j > 0 {
			delta = c.idx - prevIdx
		}
		out = appendVarint(out, uint32(delta))
		out = append(out, c.ch)
		out = append(out, byte(c.xorColor), byte(c.xorColor>>8))
		prevIdx = c.idx
	}

	return out
}

// EncodeTrack encodes a full sequence of frames into raw bytes (before compression).
func EncodeTrack(frames [][]ClipCell) []byte {
	var out []byte
	for i, frame := range frames {
		if IsIFrame(i) {
			out = append(out, EncodeIFrame(frame)...)
		} else {
			out = append(out, EncodePFrame(frames[i-1], frame)...)
		}
	}
	return out
}

// DecodeTrack decodes raw (decompressed) track data into individual frames.
func DecodeTrack(data []byte, w, h, numFrames int) ([][]ClipCell, error) {
	cellCount := w * h
	frames := make([][]ClipCell, numFrames)
	pos := 0

	for i := 0; i < numFrames; i++ {
		if IsIFrame(i) {
			need := cellCount * ClipCellBytes
			if pos+need > len(data) {
				return nil, fmt.Errorf("I-frame %d: need %d bytes at pos %d, have %d", i, need, pos, len(data))
			}
			frame := make([]ClipCell, cellCount)
			for j := 0; j < cellCount; j++ {
				off := pos + j*ClipCellBytes
				frame[j] = ClipCell{
					Ch:    data[off],
					Color: binary.LittleEndian.Uint16(data[off+1:]),
				}
			}
			frames[i] = frame
			pos += need
		} else {
			numChanges, newPos, err := readVarint(data, pos)
			if err != nil {
				return nil, fmt.Errorf("P-frame %d: %w", i, err)
			}
			pos = newPos

			prev := frames[i-1]
			frame := make([]ClipCell, cellCount)
			copy(frame, prev)

			idx := 0
			for j := uint32(0); j < numChanges; j++ {
				delta, newPos, err := readVarint(data, pos)
				if err != nil {
					return nil, fmt.Errorf("P-frame %d change %d: %w", i, j, err)
				}
				pos = newPos

				if j == 0 {
					idx = int(delta)
				} else {
					idx += int(delta)
				}

				if pos+3 > len(data) {
					return nil, fmt.Errorf("P-frame %d change %d: need 3 bytes at pos %d", i, j, pos)
				}

				ch := data[pos]
				xorColor := binary.LittleEndian.Uint16(data[pos+1:])
				pos += 3

				if idx < cellCount {
					frame[idx] = ClipCell{
						Ch:    ch,
						Color: prev[idx].Color ^ xorColor,
					}
				}
			}
			frames[i] = frame
		}
	}

	return frames, nil
}
