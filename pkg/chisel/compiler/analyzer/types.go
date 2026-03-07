// Package analyzer implements semantic analysis for the Chisel language,
// including type checking, name resolution, and validation.
package analyzer

import "fmt"

// Type represents a Chisel type.
type Type int

const (
	TypeUnknown Type = iota
	TypeFloat
	TypeVec2
	TypeVec3
	TypeBool
	TypeSDF2D
	TypeSDF3D
	TypeColor
	TypeVoid
)

var typeNames = [...]string{
	TypeUnknown: "unknown",
	TypeFloat:   "float",
	TypeVec2:    "vec2",
	TypeVec3:    "vec3",
	TypeBool:    "bool",
	TypeSDF2D:   "sdf2d",
	TypeSDF3D:   "sdf3d",
	TypeColor:   "color",
	TypeVoid:    "void",
}

// String returns the human-readable name of the type.
func (t Type) String() string {
	if int(t) >= 0 && int(t) < len(typeNames) {
		return typeNames[t]
	}
	return fmt.Sprintf("Type(%d)", int(t))
}
