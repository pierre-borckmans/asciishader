package clip

import "fmt"

// I-frame interval: every 60 frames is a full keyframe for seeking.
const iFrameInterval = 60

// IsIFrame returns true if frame index should be an I-frame.
func IsIFrame(frameIdx int) bool {
	return frameIdx%iFrameInterval == 0
}

// EncodeTrack encodes a sequence of planar frames into raw bytes (before compression).
// I-frames are stored raw; P-frames are XOR'd with the previous frame.
// Every frame is exactly FrameSize(w,h) bytes, making the format trivially seekable.
func EncodeTrack(frames [][]byte) []byte {
	if len(frames) == 0 {
		return nil
	}
	frameSize := len(frames[0])
	out := make([]byte, 0, len(frames)*frameSize)

	for i, frame := range frames {
		if IsIFrame(i) {
			out = append(out, frame...)
		} else {
			prev := frames[i-1]
			for j := 0; j < frameSize; j++ {
				out = append(out, frame[j]^prev[j])
			}
		}
	}
	return out
}

// DecodeTrack decodes raw (decompressed) track data into individual planar frames.
func DecodeTrack(data []byte, w, h, numFrames int) ([][]byte, error) {
	frameSize := FrameSize(w, h)
	if len(data) != numFrames*frameSize {
		return nil, fmt.Errorf("track size mismatch: have %d, want %d (%d frames × %d)",
			len(data), numFrames*frameSize, numFrames, frameSize)
	}

	frames := make([][]byte, numFrames)
	for i := 0; i < numFrames; i++ {
		raw := data[i*frameSize : (i+1)*frameSize]

		if IsIFrame(i) {
			frame := make([]byte, frameSize)
			copy(frame, raw)
			frames[i] = frame
		} else {
			prev := frames[i-1]
			frame := make([]byte, frameSize)
			for j := range frame {
				frame[j] = prev[j] ^ raw[j]
			}
			frames[i] = frame
		}
	}
	return frames, nil
}
