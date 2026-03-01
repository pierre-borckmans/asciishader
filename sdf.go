package main

import "math"

// --- SDF Primitives ---

func sdSphere(p Vec3, r float64) float64 {
	return p.Length() - r
}

func sdTorus(p Vec3, majorR, minorR float64) float64 {
	q := math.Sqrt(p.X*p.X+p.Z*p.Z) - majorR
	return math.Sqrt(q*q+p.Y*p.Y) - minorR
}

func sdBox(p Vec3, b Vec3) float64 {
	d := p.Abs().Sub(b)
	return d.Max(0).Length() + math.Min(math.Max(d.X, math.Max(d.Y, d.Z)), 0)
}

func sdCylinder(p Vec3, r, h float64) float64 {
	d := math.Sqrt(p.X*p.X+p.Z*p.Z) - r
	return math.Max(d, math.Abs(p.Y)-h)
}

func sdPlane(p Vec3, n Vec3, h float64) float64 {
	return p.Dot(n) + h
}

func sdCapsule(p, a, b Vec3, r float64) float64 {
	ab := b.Sub(a)
	ap := p.Sub(a)
	t := clamp(ap.Dot(ab)/ab.Dot(ab), 0, 1)
	return p.Sub(a.Add(ab.Mul(t))).Length() - r
}

func sdOctahedron(p Vec3, s float64) float64 {
	p = p.Abs()
	m := p.X + p.Y + p.Z - s
	var q Vec3
	if 3.0*p.X < m {
		q = p
	} else if 3.0*p.Y < m {
		q = V(p.Y, p.Z, p.X)
	} else if 3.0*p.Z < m {
		q = V(p.Z, p.X, p.Y)
	} else {
		return m * 0.57735027
	}
	k := clamp(0.5*(q.Z-q.Y+s), 0, s)
	return V(q.X, q.Y-s+k, q.Z-k).Length()
}

// --- SDF Operations ---

func opUnion(a, b float64) float64 {
	return math.Min(a, b)
}

func opSubtract(a, b float64) float64 {
	return math.Max(a, -b)
}

func opIntersect(a, b float64) float64 {
	return math.Max(a, b)
}

func opSmoothUnion(a, b, k float64) float64 {
	return smoothmin(a, b, k)
}

func opRound(d, r float64) float64 {
	return d - r
}

// --- Repetition ---

func opRep(p Vec3, c Vec3) Vec3 {
	return Vec3{
		math.Mod(p.X+0.5*c.X, c.X) - 0.5*c.X,
		math.Mod(p.Y+0.5*c.Y, c.Y) - 0.5*c.Y,
		math.Mod(p.Z+0.5*c.Z, c.Z) - 0.5*c.Z,
	}
}

func opRepXZ(p Vec3, cx, cz float64) Vec3 {
	return Vec3{
		math.Mod(p.X+0.5*cx, cx) - 0.5*cx,
		p.Y,
		math.Mod(p.Z+0.5*cz, cz) - 0.5*cz,
	}
}
