package gpu

import (
	"strconv"
	"unicode/utf8"

	"asciishader/pkg/core"
)

// QuadrantChars maps 4-bit patterns (TL=bit3, TR=bit2, BL=bit1, BR=bit0) to Unicode quadrant block characters.
var QuadrantChars = [16]rune{
	' ', '\u2597', '\u2596', '\u2584', '\u259D', '\u2590', '\u259E', '\u259F',
	'\u2598', '\u259A', '\u258C', '\u2599', '\u2580', '\u259C', '\u259B', '\u2588',
}

// BuildANSI converts a core.Cell grid to an ANSI true-color string with fg+bg support.
func BuildANSI(lines [][]core.Cell) string {
	return string(AppendANSI(nil, lines))
}

// AppendANSI appends ANSI-encoded core.Cell grid to buf and returns the result.
// Reuses buf capacity to avoid allocation when the caller retains the buffer.
func AppendANSI(buf []byte, lines [][]core.Cell) []byte {
	if len(lines) == 0 {
		return buf
	}
	w := len(lines[0])
	h := len(lines)
	need := w*h*20 + h*10
	if cap(buf) < need {
		buf = make([]byte, 0, need)
	}
	out := buf
	prevFR, prevFG, prevFB := -1, -1, -1
	prevBR, prevBG, prevBB := -1, -1, -1
	var runeBuf [utf8.UTFMax]byte
	for y := 0; y < h; y++ {
		if y > 0 {
			out = append(out, '\n')
		}
		prevFR, prevFG, prevFB = -1, -1, -1
		prevBR, prevBG, prevBB = -1, -1, -1
		for x := 0; x < len(lines[y]); x++ {
			c := lines[y][x]
			if c.Ch == ' ' && !c.HasBg {
				if prevBR != -1 || prevBG != -1 || prevBB != -1 {
					out = append(out, "\033[0m"...)
					prevFR, prevFG, prevFB = -1, -1, -1
					prevBR, prevBG, prevBB = -1, -1, -1
				}
				out = append(out, ' ')
				continue
			}
			// If previous core.Cell set a bg but this core.Cell doesn't use bg, reset it
			if !c.HasBg && (prevBR != -1 || prevBG != -1 || prevBB != -1) {
				out = append(out, "\033[0m"...)
				prevFR, prevFG, prevFB = -1, -1, -1
				prevBR, prevBG, prevBB = -1, -1, -1
			}

			cr := int(c.Col.X * 255)
			cg := int(c.Col.Y * 255)
			cb := int(c.Col.Z * 255)
			fgChanged := cr != prevFR || cg != prevFG || cb != prevFB
			bgChanged := false
			var br, bg2, bb int
			if c.HasBg {
				br = int(c.Bg.X * 255)
				bg2 = int(c.Bg.Y * 255)
				bb = int(c.Bg.Z * 255)
				bgChanged = br != prevBR || bg2 != prevBG || bb != prevBB
			}
			if fgChanged || bgChanged {
				out = append(out, "\033["...)
				if fgChanged {
					out = append(out, "38;2;"...)
					out = strconv.AppendInt(out, int64(cr), 10)
					out = append(out, ';')
					out = strconv.AppendInt(out, int64(cg), 10)
					out = append(out, ';')
					out = strconv.AppendInt(out, int64(cb), 10)
					prevFR, prevFG, prevFB = cr, cg, cb
				}
				if bgChanged {
					if fgChanged {
						out = append(out, ';')
					}
					out = append(out, "48;2;"...)
					out = strconv.AppendInt(out, int64(br), 10)
					out = append(out, ';')
					out = strconv.AppendInt(out, int64(bg2), 10)
					out = append(out, ';')
					out = strconv.AppendInt(out, int64(bb), 10)
					prevBR, prevBG, prevBB = br, bg2, bb
				}
				out = append(out, 'm')
			}
			n := utf8.EncodeRune(runeBuf[:], c.Ch)
			out = append(out, runeBuf[:n]...)
		}
		out = append(out, "\033[0m"...)
	}
	return out
}
