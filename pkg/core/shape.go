package core

import (
	"image"
	"image/draw"
	"math"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// ShapeVec is a 6D brightness distribution vector (2 cols × 3 rows).
// Layout: [TL, TR, ML, MR, BL, BR]
type ShapeVec [6]float64

// CharShape pairs a printable ASCII character with its shape vector.
type CharShape struct {
	Char byte
	Vec  ShapeVec
}

// ShapeTable holds pre-computed shape vectors for all printable ASCII chars.
type ShapeTable struct {
	Chars []CharShape
}

// NewShapeTable renders each printable ASCII char (0x20-0x7E) using
// basicfont.Face7x13 and computes the pixel density in 6 sub-regions (2×3).
func NewShapeTable() *ShapeTable {
	face := basicfont.Face7x13
	st := &ShapeTable{}

	for c := byte(0x20); c <= byte(0x7E); c++ {
		vec := rasterizeChar(face, c)
		st.Chars = append(st.Chars, CharShape{Char: c, Vec: vec})
	}

	// Global normalization: scale so max component across all chars = 1.0
	globalMax := 0.0
	for _, cs := range st.Chars {
		for _, v := range cs.Vec {
			if v > globalMax {
				globalMax = v
			}
		}
	}
	if globalMax > 1e-10 {
		for i := range st.Chars {
			for j := range st.Chars[i].Vec {
				st.Chars[i].Vec[j] /= globalMax
			}
		}
	}

	return st
}

func rasterizeChar(face *basicfont.Face, c byte) ShapeVec {
	const (
		glyphW = 6
		glyphH = 13
	)

	img := image.NewGray(image.Rect(0, 0, glyphW, glyphH))

	d := &font.Drawer{
		Dst:  img,
		Src:  image.White,
		Face: face,
		Dot:  fixed.P(0, 11),
	}
	d.DrawString(string(c))

	regions := [6]struct{ x0, x1, y0, y1 int }{
		{0, 3, 0, 4},  // TL
		{3, 6, 0, 4},  // TR
		{0, 3, 4, 9},  // ML
		{3, 6, 4, 9},  // MR
		{0, 3, 9, 13}, // BL
		{3, 6, 9, 13}, // BR
	}

	var sv ShapeVec
	for i, r := range regions {
		sum := 0.0
		count := 0
		for y := r.y0; y < r.y1; y++ {
			for x := r.x0; x < r.x1; x++ {
				px := img.GrayAt(x, y).Y
				sum += float64(px) / 255.0
				count++
			}
		}
		if count > 0 {
			sv[i] = sum / float64(count)
		}
	}

	return sv
}

// Ensure img implements draw.Image (compile-time check).
var _ draw.Image = (*image.Gray)(nil)

// EnhanceContrast applies global contrast enhancement to a shape vector.
func EnhanceContrast(sv ShapeVec, exponent float64) ShapeVec {
	maxComp := sv[0]
	for i := 1; i < 6; i++ {
		if sv[i] > maxComp {
			maxComp = sv[i]
		}
	}
	if maxComp < 1e-10 {
		return sv
	}

	invMax := 1.0 / maxComp
	var out ShapeVec
	for i := 0; i < 6; i++ {
		n := sv[i] * invMax
		out[i] = math.Exp(exponent*math.Log(n)) * maxComp
	}
	return out
}

// DirectionalContrast enhances edges using external reference samples.
func DirectionalContrast(sv ShapeVec, ext ShapeVec, exponent float64) ShapeVec {
	var out ShapeVec
	for i := 0; i < 6; i++ {
		maxVal := sv[i]
		if ext[i] > maxVal {
			maxVal = ext[i]
		}
		if maxVal < 1e-10 {
			continue
		}
		out[i] = math.Exp(exponent*math.Log(sv[i]/maxVal)) * maxVal
	}
	return out
}

// Match finds the character whose shape vector is closest to sv.
func (st *ShapeTable) Match(sv ShapeVec) byte {
	bestChar := byte(' ')
	bestDist := math.MaxFloat64

	for _, cs := range st.Chars {
		d0 := sv[0] - cs.Vec[0]
		dist := d0 * d0
		if dist >= bestDist {
			continue
		}
		d1 := sv[1] - cs.Vec[1]
		dist += d1 * d1
		if dist >= bestDist {
			continue
		}
		d2 := sv[2] - cs.Vec[2]
		dist += d2 * d2
		if dist >= bestDist {
			continue
		}
		d3 := sv[3] - cs.Vec[3]
		dist += d3 * d3
		d4 := sv[4] - cs.Vec[4]
		dist += d4 * d4
		d5 := sv[5] - cs.Vec[5]
		dist += d5 * d5
		if dist < bestDist {
			bestDist = dist
			bestChar = cs.Char
		}
	}

	return bestChar
}
