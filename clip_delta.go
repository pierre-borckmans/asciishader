package main

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
// Format: numChanges (uint16) + per change: index (uint16) + char (uint8) + rgb565 (uint16) = 5 bytes.
func EncodePFrame(prev, curr []ClipCell) []byte {
	// Count changes
	var changes [][2]int // index, position in curr
	for i := range curr {
		if i >= len(prev) || curr[i].Ch != prev[i].Ch || curr[i].Color != prev[i].Color {
			changes = append(changes, [2]int{i, i})
		}
	}

	numChanges := len(changes)
	if numChanges > 65535 {
		numChanges = 65535
	}

	out := make([]byte, 2+numChanges*5)
	binary.LittleEndian.PutUint16(out[0:], uint16(numChanges))

	for j := 0; j < numChanges; j++ {
		idx := changes[j][0]
		off := 2 + j*5
		binary.LittleEndian.PutUint16(out[off:], uint16(idx))
		out[off+2] = curr[idx].Ch
		binary.LittleEndian.PutUint16(out[off+3:], curr[idx].Color)
	}

	return out
}

// EncodeTrack encodes a full sequence of frames into raw bytes (before zlib).
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
			// I-frame: full grid
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
			// P-frame: delta from previous
			if pos+2 > len(data) {
				return nil, fmt.Errorf("P-frame %d: missing header at pos %d", i, pos)
			}
			numChanges := int(binary.LittleEndian.Uint16(data[pos:]))
			pos += 2

			need := numChanges * 5
			if pos+need > len(data) {
				return nil, fmt.Errorf("P-frame %d: need %d bytes at pos %d, have %d", i, need, pos, len(data))
			}

			// Start from copy of previous frame
			prev := frames[i-1]
			frame := make([]ClipCell, cellCount)
			copy(frame, prev)

			for j := 0; j < numChanges; j++ {
				off := pos + j*5
				idx := int(binary.LittleEndian.Uint16(data[off:]))
				if idx < cellCount {
					frame[idx] = ClipCell{
						Ch:    data[off+2],
						Color: binary.LittleEndian.Uint16(data[off+3:]),
					}
				}
			}
			frames[i] = frame
			pos += need
		}
	}

	return frames, nil
}
