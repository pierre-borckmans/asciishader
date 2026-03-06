package analyzer

import (
	"fmt"

	"asciishader/pkg/chisel/ast"
)

// Symbol represents a named entity in a scope (variable, function, built-in).
type Symbol struct {
	Name string
	Type Type
	Node ast.Node // definition site; nil for built-ins
	Used bool
}

// Scope represents a lexical scope with a parent chain for name resolution.
type Scope struct {
	Parent  *Scope
	Symbols map[string]*Symbol
}

// NewScope creates a new scope with the given parent.
func NewScope(parent *Scope) *Scope {
	return &Scope{
		Parent:  parent,
		Symbols: make(map[string]*Symbol),
	}
}

// Define adds a symbol to this scope. Returns an error if a symbol with the
// same name is already defined in this scope (not parent scopes — shadowing
// is allowed).
func (s *Scope) Define(name string, sym *Symbol) error {
	if _, exists := s.Symbols[name]; exists {
		return fmt.Errorf("'%s' already defined in this scope", name)
	}
	s.Symbols[name] = sym
	return nil
}

// Lookup resolves a name by walking up the scope chain. Returns nil if not found.
func (s *Scope) Lookup(name string) *Symbol {
	if sym, ok := s.Symbols[name]; ok {
		return sym
	}
	if s.Parent != nil {
		return s.Parent.Lookup(name)
	}
	return nil
}

// AllSymbolNames returns the names of all symbols reachable from this scope
// (walking up the parent chain). Used for fuzzy matching suggestions.
func (s *Scope) AllSymbolNames() []string {
	seen := make(map[string]bool)
	var names []string
	for scope := s; scope != nil; scope = scope.Parent {
		for name := range scope.Symbols {
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	return names
}

// NewBuiltinScope creates a scope populated with all built-in identifiers:
// shapes, math functions, color functions, constants, and named colors.
func NewBuiltinScope() *Scope {
	s := NewScope(nil)

	// 3D shape primitives.
	for _, name := range []string{
		"sphere", "box", "cylinder", "torus", "capsule", "cone",
		"plane", "octahedron", "pyramid", "ellipsoid",
		"rounded_box", "box_frame", "capped_torus", "hex_prism",
		"octagon_prism", "round_cone", "tri_prism", "capped_cone",
		"solid_angle", "rhombus", "horseshoe",
		"rounded_cylinder", "tetrahedron", "dodecahedron", "icosahedron", "slab",
	} {
		s.Symbols[name] = &Symbol{Name: name, Type: TypeSDF3D}
	}

	// 2D shape primitives.
	for _, name := range []string{
		"circle", "rect", "hexagon", "polygon",
	} {
		s.Symbols[name] = &Symbol{Name: name, Type: TypeSDF2D}
	}

	// Math functions.
	for _, name := range []string{
		"sin", "cos", "tan", "abs", "min", "max", "sqrt", "pow", "exp", "log",
		"floor", "ceil", "fract", "sign", "length", "normalize", "dot", "cross",
		"distance", "reflect", "mix", "smoothstep", "step", "clamp",
		"atan", "asin", "acos", "atan2", "mod", "noise", "fbm", "voronoi",
	} {
		s.Symbols[name] = &Symbol{Name: name, Type: TypeFloat}
	}

	// Color functions.
	for _, name := range []string{
		"rgb", "hsl", "hsla", "rgba",
	} {
		s.Symbols[name] = &Symbol{Name: name, Type: TypeColor}
	}

	// Constants.
	for _, name := range []string{"PI", "TAU", "E", "t"} {
		s.Symbols[name] = &Symbol{Name: name, Type: TypeFloat}
	}
	s.Symbols["p"] = &Symbol{Name: "p", Type: TypeVec3}

	// Axis constants (usable as vec3 directions).
	for _, name := range []string{"x", "y", "z"} {
		s.Symbols[name] = &Symbol{Name: name, Type: TypeVec3}
	}

	// Named colors (stored as vec3 RGB values).
	for _, name := range []string{
		"red", "green", "blue", "white", "black", "yellow",
		"cyan", "magenta", "gray", "orange", "purple", "pink",
	} {
		s.Symbols[name] = &Symbol{Name: name, Type: TypeVec3}
	}

	return s
}
