package core

import "math"

// Vec3 is a 3D vector.
type Vec3 struct {
	X, Y, Z float64
}

func V(x, y, z float64) Vec3 {
	return Vec3{x, y, z}
}

func (a Vec3) Add(b Vec3) Vec3 {
	return Vec3{a.X + b.X, a.Y + b.Y, a.Z + b.Z}
}

func (a Vec3) Sub(b Vec3) Vec3 {
	return Vec3{a.X - b.X, a.Y - b.Y, a.Z - b.Z}
}

func (a Vec3) Mul(s float64) Vec3 {
	return Vec3{a.X * s, a.Y * s, a.Z * s}
}

func (a Vec3) Dot(b Vec3) float64 {
	return a.X*b.X + a.Y*b.Y + a.Z*b.Z
}

func (a Vec3) Cross(b Vec3) Vec3 {
	return Vec3{
		a.Y*b.Z - a.Z*b.Y,
		a.Z*b.X - a.X*b.Z,
		a.X*b.Y - a.Y*b.X,
	}
}

func (a Vec3) Length() float64 {
	return math.Sqrt(a.Dot(a))
}

func (a Vec3) Normalize() Vec3 {
	l := a.Length()
	if l < 1e-10 {
		return Vec3{}
	}
	return a.Mul(1.0 / l)
}

func (a Vec3) Lerp(b Vec3, t float64) Vec3 {
	return a.Mul(1 - t).Add(b.Mul(t))
}

func (a Vec3) RotateY(angle float64) Vec3 {
	c, s := math.Cos(angle), math.Sin(angle)
	return Vec3{a.X*c + a.Z*s, a.Y, -a.X*s + a.Z*c}
}

func (a Vec3) RotateX(angle float64) Vec3 {
	c, s := math.Cos(angle), math.Sin(angle)
	return Vec3{a.X, a.Y*c - a.Z*s, a.Y*s + a.Z*c}
}

func (a Vec3) Abs() Vec3 {
	return Vec3{math.Abs(a.X), math.Abs(a.Y), math.Abs(a.Z)}
}

func (a Vec3) Max(s float64) Vec3 {
	return Vec3{math.Max(a.X, s), math.Max(a.Y, s), math.Max(a.Z, s)}
}

func (a Vec3) CompMax(b Vec3) Vec3 {
	return Vec3{math.Max(a.X, b.X), math.Max(a.Y, b.Y), math.Max(a.Z, b.Z)}
}

func (a Vec3) CompMin(b Vec3) Vec3 {
	return Vec3{math.Min(a.X, b.X), math.Min(a.Y, b.Y), math.Min(a.Z, b.Z)}
}

func Clamp(v, lo, hi float64) float64 {
	return math.Max(lo, math.Min(hi, v))
}

func Mix(a, b, t float64) float64 {
	return a*(1-t) + b*t
}

func Smoothmin(a, b, k float64) float64 {
	h := Clamp(0.5+0.5*(b-a)/k, 0, 1)
	return Mix(b, a, h) - k*h*(1-h)
}
