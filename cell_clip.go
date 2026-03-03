package main

import "asciishader/clip"

// CellToClipCell converts a render cell to a clip cell.
func CellToClipCell(c cell) clip.ClipCell {
	return clip.ClipCell{
		Ch:    byte(c.ch),
		Color: clip.RGB565Encode(c.col.X, c.col.Y, c.col.Z),
	}
}
