package main

import (
	"asciishader/clip"
	"asciishader/core"
)

// CellToClipCell converts a render cell to a clip cell.
func CellToClipCell(c core.Cell) clip.ClipCell {
	return clip.ClipCell{
		Ch:    byte(c.Ch),
		Color: clip.RGB565Encode(c.Col.X, c.Col.Y, c.Col.Z),
	}
}
