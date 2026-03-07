package analyzer

import (
	"fmt"

	"asciishader/pkg/chisel/lang"
)

// Symbol represents a named entity in a scope (variable, function, built-in).
type Symbol struct {
	Name string
	Type Type
	Node interface{} // definition site; nil for built-ins
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

// Define adds a symbol to this scope. Returns an error if already defined
// in this scope (shadowing parent scopes is allowed).
func (s *Scope) Define(name string, sym *Symbol) error {
	if _, exists := s.Symbols[name]; exists {
		return fmt.Errorf("'%s' already defined in this scope", name)
	}
	s.Symbols[name] = sym
	return nil
}

// Lookup resolves a name by walking up the scope chain.
func (s *Scope) Lookup(name string) *Symbol {
	if sym, ok := s.Symbols[name]; ok {
		return sym
	}
	if s.Parent != nil {
		return s.Parent.Lookup(name)
	}
	return nil
}

// AllSymbolNames returns names of all reachable symbols (for fuzzy matching).
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

// NewBuiltinScope creates a scope populated from the lang registry.
func NewBuiltinScope() *Scope {
	s := NewScope(nil)

	for _, shape := range lang.Shapes3D {
		s.Symbols[shape.Name] = &Symbol{Name: shape.Name, Type: TypeSDF3D}
	}
	for _, shape := range lang.Shapes2D {
		s.Symbols[shape.Name] = &Symbol{Name: shape.Name, Type: TypeSDF2D}
	}
	for _, fn := range lang.Functions {
		s.Symbols[fn.Name] = &Symbol{Name: fn.Name, Type: TypeFloat}
	}
	for _, c := range lang.Constants {
		typ := TypeFloat
		if c.Vec {
			typ = TypeVec3
		}
		s.Symbols[c.Name] = &Symbol{Name: c.Name, Type: typ}
	}
	for _, c := range lang.Colors {
		s.Symbols[c.Name] = &Symbol{Name: c.Name, Type: TypeVec3}
	}

	return s
}
