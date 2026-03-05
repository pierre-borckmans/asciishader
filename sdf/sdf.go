package sdf

import (
	"math"

	"asciishader/core"
)

// --- SDF Primitives ---

func SdSphere(p core.Vec3, r float64) float64 {
	return p.Length() - r
}

func SdTorus(p core.Vec3, majorR, minorR float64) float64 {
	q := math.Sqrt(p.X*p.X+p.Z*p.Z) - majorR
	return math.Sqrt(q*q+p.Y*p.Y) - minorR
}

func SdBox(p core.Vec3, b core.Vec3) float64 {
	d := p.Abs().Sub(b)
	return d.Max(0).Length() + math.Min(math.Max(d.X, math.Max(d.Y, d.Z)), 0)
}

func SdCylinder(p core.Vec3, r, h float64) float64 {
	d := math.Sqrt(p.X*p.X+p.Z*p.Z) - r
	return math.Max(d, math.Abs(p.Y)-h)
}

func SdPlane(p core.Vec3, n core.Vec3, h float64) float64 {
	return p.Dot(n) + h
}

func SdCapsule(p, a, b core.Vec3, r float64) float64 {
	ab := b.Sub(a)
	ap := p.Sub(a)
	t := core.Clamp(ap.Dot(ab)/ab.Dot(ab), 0, 1)
	return p.Sub(a.Add(ab.Mul(t))).Length() - r
}

func SdOctahedron(p core.Vec3, s float64) float64 {
	p = p.Abs()
	m := p.X + p.Y + p.Z - s
	var q core.Vec3
	if 3.0*p.X < m {
		q = p
	} else if 3.0*p.Y < m {
		q = core.V(p.Y, p.Z, p.X)
	} else if 3.0*p.Z < m {
		q = core.V(p.Z, p.X, p.Y)
	} else {
		return m * 0.57735027
	}
	k := core.Clamp(0.5*(q.Z-q.Y+s), 0, s)
	return core.V(q.X, q.Y-s+k, q.Z-k).Length()
}

// --- SDF Operations ---

func OpUnion(a, b float64) float64 {
	return math.Min(a, b)
}

func OpSubtract(a, b float64) float64 {
	return math.Max(a, -b)
}

func OpIntersect(a, b float64) float64 {
	return math.Max(a, b)
}

func OpSmoothUnion(a, b, k float64) float64 {
	return core.Smoothmin(a, b, k)
}

func OpRound(d, r float64) float64 {
	return d - r
}

// --- Repetition ---

func OpRep(p core.Vec3, c core.Vec3) core.Vec3 {
	return core.Vec3{
		X: math.Mod(p.X+0.5*c.X, c.X) - 0.5*c.X,
		Y: math.Mod(p.Y+0.5*c.Y, c.Y) - 0.5*c.Y,
		Z: math.Mod(p.Z+0.5*c.Z, c.Z) - 0.5*c.Z,
	}
}

func OpRepXZ(p core.Vec3, cx, cz float64) core.Vec3 {
	return core.Vec3{
		X: math.Mod(p.X+0.5*cx, cx) - 0.5*cx,
		Y: p.Y,
		Z: math.Mod(p.Z+0.5*cz, cz) - 0.5*cz,
	}
}
