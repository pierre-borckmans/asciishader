package clip

import "asciishader/pkg/core"

// CellsFromFrame reconstructs a core.Cell grid from a planar sub-pixel frame.
// The render mode determines which algorithm maps brightness patterns to characters.
func CellsFromFrame(frame []byte, w, h, mode int, st *core.ShapeTable, contrast float64) [][]core.Cell {
	switch mode {
	case core.RenderBlocks:
		return cellsBlocks(frame, w, h)
	case core.RenderBraille:
		return cellsBraille(frame, w, h)
	default:
		return cellsShaped(frame, w, h, st, contrast)
	}
}

// cellsShaped reconstructs cells using shape matching on brightness patterns.
// Stored 2×4 sub-pixels are downsampled to the 2×3 shape vector the shape table expects.
func cellsShaped(frame []byte, w, h int, st *core.ShapeTable, contrast float64) [][]core.Cell {
	cc := w * h
	lines := makeCellGrid(w, h)
	const inv255 = 1.0 / 255.0

	for cy := 0; cy < h; cy++ {
		line := lines[cy]
		for cx := 0; cx < w; cx++ {
			ci := cy*w + cx

			// Read 8 brightness values (planes 0-7)
			b0 := float64(FrameBrightness(frame, cc, ci, 0)) * inv255
			b1 := float64(FrameBrightness(frame, cc, ci, 1)) * inv255
			b2 := float64(FrameBrightness(frame, cc, ci, 2)) * inv255
			b3 := float64(FrameBrightness(frame, cc, ci, 3)) * inv255
			b4 := float64(FrameBrightness(frame, cc, ci, 4)) * inv255
			b5 := float64(FrameBrightness(frame, cc, ci, 5)) * inv255
			b6 := float64(FrameBrightness(frame, cc, ci, 6)) * inv255
			b7 := float64(FrameBrightness(frame, cc, ci, 7)) * inv255

			// Downsample 2×4 → 2×3 shape vector
			// Row 0 → TL, TR; avg(Row 1, Row 2) → ML, MR; Row 3 → BL, BR
			var sv core.ShapeVec
			sv[0] = b0
			sv[1] = b1
			sv[2] = (b2 + b4) * 0.5
			sv[3] = (b3 + b5) * 0.5
			sv[4] = b6
			sv[5] = b7

			avgBright := sv[0] + sv[1] + sv[2] + sv[3] + sv[4] + sv[5]
			if avgBright < 0.06 {
				line[cx] = core.Cell{Ch: ' '}
				continue
			}

			// Directional contrast from neighbor cells' sub-pixels
			var ext core.ShapeVec
			ext[0] = neighborBrightness(frame, cc, w, h, cx-1, cy-1, 7) // bottom-right of cell above-left
			ext[1] = neighborBrightness(frame, cc, w, h, cx+1, cy-1, 6) // bottom-left of cell above-right
			ext[2] = neighborBrightness(frame, cc, w, h, cx-1, cy, 3)   // mid-right of cell left
			ext[3] = neighborBrightness(frame, cc, w, h, cx+1, cy, 2)   // mid-left of cell right
			ext[4] = neighborBrightness(frame, cc, w, h, cx-1, cy+1, 1) // top-right of cell below-left
			ext[5] = neighborBrightness(frame, cc, w, h, cx+1, cy+1, 0) // top-left of cell below-right

			sv = core.DirectionalContrast(sv, ext, contrast)
			sv = core.EnhanceContrast(sv, contrast)
			ch := st.Match(sv)

			color := FrameColor(frame, cc, ci)
			cr, cg, cb := RGB565Decode(color)

			line[cx] = core.Cell{
				Ch:  rune(ch),
				Col: core.Vec3{X: float64(cr) / 255, Y: float64(cg) / 255, Z: float64(cb) / 255},
			}
		}
	}
	return lines
}

// cellsBlocks reconstructs cells using quadrant block characters.
// Uses all 8 sub-pixel brightness values directly — each quadrant checks
// whether ANY of its 2 vertical sub-pixels are lit (preserves sharp edges).
func cellsBlocks(frame []byte, w, h int) [][]core.Cell {
	cc := w * h
	lines := makeCellGrid(w, h)
	const hitThresh uint8 = 2

	// Quadrant → sub-pixel indices: each quadrant spans 2 vertical rows
	// TL: planes 0,2 (col0, rows 0-1)
	// TR: planes 1,3 (col1, rows 0-1)
	// BL: planes 4,6 (col0, rows 2-3)
	// BR: planes 5,7 (col1, rows 2-3)
	qIdx := [4][2]int{{0, 2}, {1, 3}, {4, 6}, {5, 7}}

	for cy := 0; cy < h; cy++ {
		line := lines[cy]
		for cx := 0; cx < w; cx++ {
			ci := cy*w + cx

			// Detect hits per quadrant using max of sub-pixels
			var hit [4]bool
			var bright [4]float64
			hitCount := 0
			for qi := 0; qi < 4; qi++ {
				a := FrameBrightness(frame, cc, ci, qIdx[qi][0])
				b := FrameBrightness(frame, cc, ci, qIdx[qi][1])
				// Quadrant is lit if either sub-pixel is lit
				hit[qi] = a > hitThresh || b > hitThresh
				// Use max brightness for edge detection
				if a > b {
					bright[qi] = float64(a) / 255
				} else {
					bright[qi] = float64(b) / 255
				}
				if hit[qi] {
					hitCount++
				}
			}

			if hitCount == 0 {
				line[cx] = core.Cell{Ch: ' '}
				continue
			}

			mean := (bright[0] + bright[1] + bright[2] + bright[3]) * 0.25

			// Uniform surface → full block
			if hitCount == 4 {
				minB, maxB := bright[0], bright[0]
				for i := 1; i < 4; i++ {
					if bright[i] < minB {
						minB = bright[i]
					}
					if bright[i] > maxB {
						maxB = bright[i]
					}
				}
				if maxB-minB < 0.08 {
					color := FrameColor(frame, cc, ci)
					cr, cg, cb := RGB565Decode(color)
					line[cx] = core.Cell{Ch: '█', Col: core.Vec3{
						X: float64(cr) / 255, Y: float64(cg) / 255, Z: float64(cb) / 255,
					}}
					continue
				}
			}

			// Edge cell: threshold quadrants around mean
			var pattern int
			for i := 0; i < 4; i++ {
				bit := 3 - i // TL=bit3, TR=bit2, BL=bit1, BR=bit0
				if hit[i] && bright[i] > mean {
					pattern |= 1 << uint(bit)
				}
			}

			if pattern == 0 {
				line[cx] = core.Cell{Ch: ' '}
				continue
			}

			color := FrameColor(frame, cc, ci)
			cr, cg, cb := RGB565Decode(color)
			ch := core.QuadrantChars[pattern]
			line[cx] = core.Cell{
				Ch:  ch,
				Col: core.Vec3{X: float64(cr) / 255, Y: float64(cg) / 255, Z: float64(cb) / 255},
			}
		}
	}
	return lines
}

// cellsBraille reconstructs cells using braille dot patterns.
// 2×4 sub-pixels map directly to the 2×4 braille dot grid.
func cellsBraille(frame []byte, w, h int) [][]core.Cell {
	cc := w * h
	lines := makeCellGrid(w, h)
	const hitThresh uint8 = 2

	// Braille bit mapping: plane index → braille bit
	// Layout: col0=left, col1=right; rows 0-3 top to bottom
	// plane 0 (col0,row0)→bit0, plane 1 (col1,row0)→bit3
	// plane 2 (col0,row1)→bit1, plane 3 (col1,row1)→bit4
	// plane 4 (col0,row2)→bit2, plane 5 (col1,row2)→bit5
	// plane 6 (col0,row3)→bit6, plane 7 (col1,row3)→bit7
	brailleBit := [8]rune{0x01, 0x08, 0x02, 0x10, 0x04, 0x20, 0x40, 0x80}

	for cy := 0; cy < h; cy++ {
		line := lines[cy]
		for cx := 0; cx < w; cx++ {
			ci := cy*w + cx

			var pattern rune
			for p := 0; p < 8; p++ {
				if FrameBrightness(frame, cc, ci, p) > hitThresh {
					pattern |= brailleBit[p]
				}
			}

			if pattern == 0 {
				line[cx] = core.Cell{Ch: ' '}
				continue
			}

			color := FrameColor(frame, cc, ci)
			cr, cg, cb := RGB565Decode(color)
			line[cx] = core.Cell{
				Ch:  0x2800 + pattern,
				Col: core.Vec3{X: float64(cr) / 255, Y: float64(cg) / 255, Z: float64(cb) / 255},
			}
		}
	}
	return lines
}

// neighborBrightness returns the brightness (0-1) of a specific sub-pixel
// in a neighboring cell. Returns 0 if the neighbor is out of bounds.
func neighborBrightness(frame []byte, cellCount, w, h, cx, cy, plane int) float64 {
	if cx < 0 || cx >= w || cy < 0 || cy >= h {
		return 0
	}
	ci := cy*w + cx
	return float64(FrameBrightness(frame, cellCount, ci, plane)) / 255
}

func makeCellGrid(w, h int) [][]core.Cell {
	lines := make([][]core.Cell, h)
	for i := range lines {
		lines[i] = make([]core.Cell, w)
	}
	return lines
}
