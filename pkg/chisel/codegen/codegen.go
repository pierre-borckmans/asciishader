// Package codegen translates a Chisel AST into GLSL source code.
// The output defines sceneSDF(vec3 p) and sceneColor(vec3 p), which are
// consumed by shader.Assemble to produce a complete fragment shader.
package codegen

import (
	"fmt"
	"math"
	"strings"

	"asciishader/pkg/chisel/ast"
	"asciishader/pkg/chisel/diagnostic"
	"asciishader/pkg/chisel/token"
)

// maxUnrollIterations is the compile-time limit for for-loop unrolling.
const maxUnrollIterations = 256

// namedColors maps named color method names to their GLSL vec3 representations.
var namedColors = map[string]string{
	"red":     "vec3(1.0, 0.0, 0.0)",
	"blue":    "vec3(0.0, 0.0, 1.0)",
	"green":   "vec3(0.0, 1.0, 0.0)",
	"white":   "vec3(1.0, 1.0, 1.0)",
	"black":   "vec3(0.0, 0.0, 0.0)",
	"yellow":  "vec3(1.0, 1.0, 0.0)",
	"cyan":    "vec3(0.0, 1.0, 1.0)",
	"magenta": "vec3(1.0, 0.0, 1.0)",
	"orange":  "vec3(1.0, 0.5, 0.0)",
	"gray":    "vec3(0.5, 0.5, 0.5)",
}

// isNamedColor reports whether name is a named color method.
func isNamedColor(name string) bool {
	_, ok := namedColors[name]
	return ok
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// Generate takes a parsed AST and produces GLSL code that defines
// sceneSDF(vec3 p) and sceneColor(vec3 p). It returns any diagnostics.
func Generate(prog *ast.Program) (string, []diagnostic.Diagnostic) {
	g := &generator{
		scope:     make(map[string]scopeEntry),
		funcs:     make(map[string]*ast.AssignStmt),
		helpers:   make(map[string]bool),
		emittedFn: make(map[string]bool),
	}

	// First pass: collect top-level assignments (variables & functions)
	// and find the scene expression.
	var sceneExpr ast.Expr
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.AssignStmt:
			if s.Params != nil {
				// Function definition — record for later emission.
				g.funcs[s.Name] = s
			} else {
				// Variable assignment — could be scalar or SDF.
				g.scope[s.Name] = scopeEntry{kind: entryAST, node: s.Value}
			}
		case *ast.ExprStmt:
			sceneExpr = s.Expression
		}
	}

	// If no scene expression, emit default sphere.
	if sceneExpr == nil {
		sceneExpr = &ast.Ident{Name: "sphere"}
	}

	// Second pass: emit GLSL function definitions for user functions.
	var fnDefs strings.Builder
	for name, fn := range g.funcs {
		if !g.emittedFn[name] {
			g.emitFuncDef(&fnDefs, name, fn)
		}
	}

	// Third pass: emit sceneSDF body.
	g.pointVar = "p"
	g.tmpCounter = 0
	result := g.emitSDF(sceneExpr)

	// Build the GLSL output.
	var out strings.Builder

	// Emit helper functions (smooth subtract, etc.) if needed.
	g.writeHelpers(&out)

	// Emit user function definitions.
	out.WriteString(fnDefs.String())

	// Emit sceneSDF.
	out.WriteString("float sceneSDF(vec3 p) {\n")
	out.WriteString(g.body.String())
	fmt.Fprintf(&out, "    return %s;\n", result)
	out.WriteString("}\n\n")

	// Emit sceneColor.
	out.WriteString("vec3 sceneColor(vec3 p) {\n")
	if len(g.colors) == 0 {
		// No colors specified — default white.
		out.WriteString("    return vec3(1.0);\n")
	} else if len(g.colors) == 1 {
		// Single colored shape — always return its color.
		fmt.Fprintf(&out, "    return %s;\n", g.colors[0].colorExpr)
	} else {
		// Multiple colored shapes — duplicate sceneSDF body to get SDF variables,
		// then compare distances to pick the closest color.
		out.WriteString(g.body.String())

		// Compare distances: return the closest shape's color.
		for i := 0; i < len(g.colors)-1; i++ {
			cmp := g.colors[i].sdfVar
			// Build comparison against the remaining colors.
			restCmp := g.colors[i+1].sdfVar
			for j := i + 2; j < len(g.colors); j++ {
				restCmp = fmt.Sprintf("min(%s, %s)", restCmp, g.colors[j].sdfVar)
			}
			fmt.Fprintf(&out, "    if (%s < %s) return %s;\n", cmp, restCmp, g.colors[i].colorExpr)
		}
		fmt.Fprintf(&out, "    return %s;\n", g.colors[len(g.colors)-1].colorExpr)
	}
	out.WriteString("}\n")

	return out.String(), g.diags
}

// ---------------------------------------------------------------------------
// Generator state
// ---------------------------------------------------------------------------

type entryKind int

const (
	entryAST   entryKind = iota // value is an AST node (SDF or scalar)
	entryFloat                  // value is a GLSL float variable name
)

type scopeEntry struct {
	kind    entryKind
	node    ast.Expr // for entryAST
	varName string   // for entryFloat (GLSL variable name)
}

// colorEntry tracks the color associated with a specific SDF variable.
type colorEntry struct {
	sdfVar    string // the GLSL variable name holding the SDF distance (e.g. "d0")
	colorExpr string // the GLSL color expression (e.g. "vec3(1.0, 0.0, 0.0)")
}

type generator struct {
	body       strings.Builder // accumulated GLSL statements for current function
	tmpCounter int             // monotonically increasing temp variable counter
	pointVar   string          // current evaluation point variable (e.g. "p", "p0", "p1")
	diags      []diagnostic.Diagnostic
	scope      map[string]scopeEntry
	funcs      map[string]*ast.AssignStmt // user-defined functions
	helpers    map[string]bool            // which GLSL helper functions are needed
	emittedFn  map[string]bool            // which user functions have been emitted
	indent     int                        // current indent level (for nested code)

	// loopVars tracks current loop variable substitution values during unrolling.
	loopVars map[string]float64

	// colors tracks the color associated with each SDF variable for sceneColor generation.
	colors []colorEntry
}

// freshVar returns a fresh temporary variable name like "d0", "d1", etc.
func (g *generator) freshVar(prefix string) string {
	name := fmt.Sprintf("%s%d", prefix, g.tmpCounter)
	g.tmpCounter++
	return name
}

// emit writes a line of GLSL to the body builder with proper indentation.
func (g *generator) emit(format string, args ...interface{}) {
	indent := strings.Repeat("    ", g.indent+1)
	fmt.Fprintf(&g.body, indent+format+"\n", args...)
}

// addDiag records a diagnostic.
func (g *generator) addDiag(sev diagnostic.Severity, msg string, span token.Span) {
	g.diags = append(g.diags, diagnostic.Diagnostic{
		Severity: sev,
		Message:  msg,
		Span:     span,
	})
}

// formatFloat formats a float64 as a GLSL float literal.
func formatFloat(v float64) string {
	// Check if it's a whole number
	if v == math.Trunc(v) && !math.IsInf(v, 0) && !math.IsNaN(v) {
		return fmt.Sprintf("%.1f", v)
	}
	s := fmt.Sprintf("%g", v)
	// Ensure there's a decimal point for GLSL
	if !strings.Contains(s, ".") && !strings.Contains(s, "e") && !strings.Contains(s, "E") {
		s += ".0"
	}
	return s
}

// ---------------------------------------------------------------------------
// SDF emission — recursive descent over AST
// ---------------------------------------------------------------------------

// emitSDF generates GLSL for an SDF expression and returns the variable name
// holding the distance value (e.g. "d0", "d1").
func (g *generator) emitSDF(expr ast.Expr) string {
	if expr == nil {
		return "0.0"
	}

	switch e := expr.(type) {
	case *ast.Ident:
		return g.emitIdent(e)
	case *ast.FuncCall:
		return g.emitFuncCall(e)
	case *ast.MethodCall:
		return g.emitMethodCall(e)
	case *ast.BinaryExpr:
		return g.emitBinaryExpr(e)
	case *ast.UnaryExpr:
		return g.emitUnaryExpr(e)
	case *ast.NumberLit:
		return formatFloat(e.Value)
	case *ast.Block:
		return g.emitBlock(e)
	case *ast.ForExpr:
		return g.emitForExpr(e)
	case *ast.IfExpr:
		return g.emitIfExpr(e)
	case *ast.GlslEscape:
		return g.emitGlslEscape(e)
	case *ast.VecLit:
		return g.emitVecLit(e)
	case *ast.HexColorLit:
		return fmt.Sprintf("vec3(%s, %s, %s)", formatFloat(e.R), formatFloat(e.G), formatFloat(e.B))
	case *ast.BoolLit:
		if e.Value {
			return "true"
		}
		return "false"
	case *ast.Swizzle:
		recv := g.emitSDF(e.Receiver)
		return fmt.Sprintf("%s.%s", recv, e.Components)
	default:
		g.addDiag(diagnostic.Error, fmt.Sprintf("unsupported expression type %T", expr), expr.NodeSpan())
		return "0.0"
	}
}

// emitScalarExpr generates GLSL for a scalar (non-SDF) expression, returning
// the GLSL expression string (not a variable assignment).
func (g *generator) emitScalarExpr(expr ast.Expr) string {
	if expr == nil {
		return "0.0"
	}
	switch e := expr.(type) {
	case *ast.NumberLit:
		return formatFloat(e.Value)
	case *ast.Ident:
		// Check if this is a loop variable.
		if g.loopVars != nil {
			if v, ok := g.loopVars[e.Name]; ok {
				return formatFloat(v)
			}
		}
		// Check scope for float variables.
		if entry, ok := g.scope[e.Name]; ok {
			if entry.kind == entryFloat {
				return entry.varName
			}
		}
		// Axis constants.
		switch e.Name {
		case "x":
			return "vec3(1.0, 0.0, 0.0)"
		case "y":
			return "vec3(0.0, 1.0, 0.0)"
		case "z":
			return "vec3(0.0, 0.0, 1.0)"
		case "t":
			return "uTime"
		case "p":
			return g.pointVar
		case "PI":
			return "PI"
		case "TAU":
			return "(2.0 * PI)"
		case "E":
			return "2.71828183"
		}
		return e.Name
	case *ast.BinaryExpr:
		left := g.emitScalarExpr(e.Left)
		right := g.emitScalarExpr(e.Right)
		op := scalarBinaryOp(e.Op)
		return fmt.Sprintf("(%s %s %s)", left, op, right)
	case *ast.UnaryExpr:
		operand := g.emitScalarExpr(e.Operand)
		switch e.Op {
		case ast.Neg:
			return fmt.Sprintf("(-%s)", operand)
		case ast.Not:
			return fmt.Sprintf("(!%s)", operand)
		}
		return operand
	case *ast.FuncCall:
		return g.emitScalarFuncCall(e)
	case *ast.VecLit:
		return g.emitVecLit(e)
	case *ast.Swizzle:
		recv := g.emitScalarExpr(e.Receiver)
		return fmt.Sprintf("%s.%s", recv, e.Components)
	case *ast.BoolLit:
		if e.Value {
			return "true"
		}
		return "false"
	case *ast.HexColorLit:
		return fmt.Sprintf("vec3(%s, %s, %s)", formatFloat(e.R), formatFloat(e.G), formatFloat(e.B))
	case *ast.MethodCall:
		// Handle swizzle-like method calls on scalars
		recv := g.emitScalarExpr(e.Receiver)
		var argStrs []string
		for _, a := range e.Args {
			argStrs = append(argStrs, g.emitScalarExpr(a.Value))
		}
		return fmt.Sprintf("%s.%s(%s)", recv, e.Name, strings.Join(argStrs, ", "))
	default:
		return g.emitSDF(expr)
	}
}

// scalarBinaryOp returns the GLSL operator string for a scalar binary op.
func scalarBinaryOp(op ast.BinaryOp) string {
	switch op {
	case ast.Add:
		return "+"
	case ast.Sub:
		return "-"
	case ast.Mul:
		return "*"
	case ast.Div:
		return "/"
	case ast.Mod:
		return "%"
	case ast.Eq:
		return "=="
	case ast.Neq:
		return "!="
	case ast.Lt:
		return "<"
	case ast.Gt:
		return ">"
	case ast.Lte:
		return "<="
	case ast.Gte:
		return ">="
	default:
		return "+"
	}
}

// emitVecLit generates a GLSL vector constructor.
func (g *generator) emitVecLit(v *ast.VecLit) string {
	var elems []string
	for _, e := range v.Elems {
		elems = append(elems, g.emitScalarExpr(e))
	}
	switch len(elems) {
	case 2:
		return fmt.Sprintf("vec2(%s)", strings.Join(elems, ", "))
	case 3:
		return fmt.Sprintf("vec3(%s)", strings.Join(elems, ", "))
	case 4:
		return fmt.Sprintf("vec4(%s)", strings.Join(elems, ", "))
	default:
		return strings.Join(elems, ", ")
	}
}

// emitScalarFuncCall generates GLSL for built-in math functions used in scalar context.
func (g *generator) emitScalarFuncCall(e *ast.FuncCall) string {
	// Check for built-in math/GLSL functions.
	builtins := map[string]bool{
		"sin": true, "cos": true, "tan": true, "asin": true, "acos": true,
		"atan": true, "atan2": true, "pow": true, "sqrt": true, "exp": true,
		"log": true, "floor": true, "ceil": true, "round": true, "fract": true,
		"min": true, "max": true, "abs": true, "sign": true, "mix": true,
		"smoothstep": true, "step": true, "clamp": true, "mod": true,
		"length": true, "normalize": true, "dot": true, "cross": true,
		"distance": true, "reflect": true, "radians": true, "degrees": true,
	}

	if builtins[e.Name] {
		var args []string
		for _, a := range e.Args {
			args = append(args, g.emitScalarExpr(a.Value))
		}
		return fmt.Sprintf("%s(%s)", e.Name, strings.Join(args, ", "))
	}

	// User-defined function call in scalar context: fn_name(p, args...)
	if fn, ok := g.funcs[e.Name]; ok {
		_ = fn
		var args []string
		args = append(args, g.pointVar)
		for _, a := range e.Args {
			args = append(args, g.emitScalarExpr(a.Value))
		}
		return fmt.Sprintf("fn_%s(%s)", e.Name, strings.Join(args, ", "))
	}

	// Fallback: emit as-is.
	var args []string
	for _, a := range e.Args {
		args = append(args, g.emitScalarExpr(a.Value))
	}
	return fmt.Sprintf("%s(%s)", e.Name, strings.Join(args, ", "))
}

// ---------------------------------------------------------------------------
// Identifiers
// ---------------------------------------------------------------------------

func (g *generator) emitIdent(e *ast.Ident) string {
	name := e.Name

	// Check loop variables first.
	if g.loopVars != nil {
		if v, ok := g.loopVars[name]; ok {
			return formatFloat(v)
		}
	}

	// Check scope.
	if entry, ok := g.scope[name]; ok {
		switch entry.kind {
		case entryAST:
			// SDF variable: re-emit the AST expression.
			return g.emitSDF(entry.node)
		case entryFloat:
			return entry.varName
		}
	}

	// Built-in shapes (bare idents without parens).
	if isBuiltinShape(name) {
		d := g.freshVar("d")
		g.emit("float %s = %s;", d, shapeDefault(name, g.pointVar))
		return d
	}

	// Time.
	if name == "t" {
		return "uTime"
	}

	// Point.
	if name == "p" {
		return g.pointVar
	}

	// Constants.
	switch name {
	case "PI":
		return "PI"
	case "TAU":
		return "(2.0 * PI)"
	case "E":
		return "2.71828183"
	}

	g.addDiag(diagnostic.Error, fmt.Sprintf("undefined identifier %q", name), e.NodeSpan())
	return "0.0"
}

// ---------------------------------------------------------------------------
// Function calls (shapes and user-defined)
// ---------------------------------------------------------------------------

func (g *generator) emitFuncCall(e *ast.FuncCall) string {
	// Check for built-in shapes.
	if isBuiltinShape(e.Name) {
		d := g.freshVar("d")
		g.emit("float %s = %s;", d, g.shapeCall(e.Name, g.pointVar, e.Args))
		return d
	}

	// User-defined function.
	if fn, ok := g.funcs[e.Name]; ok {
		_ = fn
		// Ensure the function has been emitted.
		d := g.freshVar("d")
		var args []string
		args = append(args, g.pointVar)
		for i, a := range e.Args {
			_ = i
			args = append(args, g.emitScalarExpr(a.Value))
		}
		// Fill in defaults for missing positional args.
		if fn.Params != nil {
			for i := len(e.Args); i < len(fn.Params); i++ {
				if fn.Params[i].Default != nil {
					args = append(args, g.emitScalarExpr(fn.Params[i].Default))
				}
			}
		}
		g.emit("float %s = fn_%s(%s);", d, e.Name, strings.Join(args, ", "))
		return d
	}

	// Built-in math functions used as SDF? Emit as scalar.
	builtinMath := map[string]bool{
		"sin": true, "cos": true, "tan": true, "abs": true, "min": true,
		"max": true, "mix": true, "smoothstep": true, "step": true,
		"clamp": true, "length": true, "normalize": true, "pow": true,
		"sqrt": true, "fract": true, "floor": true, "ceil": true,
		"dot": true, "cross": true, "mod": true,
	}
	if builtinMath[e.Name] {
		var args []string
		for _, a := range e.Args {
			args = append(args, g.emitScalarExpr(a.Value))
		}
		return fmt.Sprintf("%s(%s)", e.Name, strings.Join(args, ", "))
	}

	g.addDiag(diagnostic.Error, fmt.Sprintf("undefined function %q", e.Name), e.NodeSpan())
	return "0.0"
}

// ---------------------------------------------------------------------------
// Method calls (transforms)
// ---------------------------------------------------------------------------

func (g *generator) emitMethodCall(e *ast.MethodCall) string {
	// Named color methods: .red, .blue, etc.
	if isNamedColor(e.Name) {
		result := g.emitSDF(e.Receiver)
		g.colors = append(g.colors, colorEntry{sdfVar: result, colorExpr: namedColors[e.Name]})
		return result
	}

	switch e.Name {
	case "at":
		return g.emitTransformAt(e)
	case "scale":
		return g.emitTransformScale(e)
	case "rot":
		return g.emitTransformRot(e)
	case "color":
		return g.emitColor(e)
	case "mirror":
		return g.emitMirror(e)
	case "rep":
		return g.emitRep(e)
	case "morph":
		return g.emitMorph(e)
	case "shell", "onion":
		return g.emitShell(e)
	case "displace":
		return g.emitDisplace(e)
	case "dilate":
		return g.emitDilate(e)
	case "erode":
		return g.emitErode(e)
	case "round":
		return g.emitRound(e)
	case "elongate":
		return g.emitElongate(e)
	case "twist":
		return g.emitTwist(e)
	case "bend":
		return g.emitBend(e)
	default:
		// Unknown method — try to emit receiver and treat as pass-through.
		g.addDiag(diagnostic.Warning, fmt.Sprintf("unknown method .%s(), ignoring", e.Name), e.NodeSpan())
		return g.emitSDF(e.Receiver)
	}
}

// emitTransformAt handles .at(x, y, z)
func (g *generator) emitTransformAt(e *ast.MethodCall) string {
	args := e.Args
	if len(args) < 1 {
		g.addDiag(diagnostic.Error, ".at() requires at least 1 argument", e.NodeSpan())
		return g.emitSDF(e.Receiver)
	}

	// Check for named args: .at(x: 2), .at(y: -1), etc.
	var x, y, z string
	x, y, z = "0.0", "0.0", "0.0"

	hasNamed := false
	for _, a := range args {
		if a.Name != "" {
			hasNamed = true
			break
		}
	}

	if hasNamed {
		for _, a := range args {
			val := g.emitScalarExpr(a.Value)
			switch a.Name {
			case "x":
				x = val
			case "y":
				y = val
			case "z":
				z = val
			}
		}
	} else {
		// Positional: 1 arg → (x, 0, 0)? No, .at(x, y, z) requires 3.
		// Actually per the spec, .at(x, y, z) takes 3 positional args.
		if len(args) >= 1 {
			x = g.emitScalarExpr(args[0].Value)
		}
		if len(args) >= 2 {
			y = g.emitScalarExpr(args[1].Value)
		}
		if len(args) >= 3 {
			z = g.emitScalarExpr(args[2].Value)
		}
	}

	// Create a new point variable with the translation applied.
	pNew := g.freshVar("p")
	g.emit("vec3 %s = %s - vec3(%s, %s, %s);", pNew, g.pointVar, x, y, z)

	// Save and restore point variable.
	oldPoint := g.pointVar
	g.pointVar = pNew
	result := g.emitSDF(e.Receiver)
	g.pointVar = oldPoint

	return result
}

// emitTransformScale handles .scale(s) and .scale(x, y, z)
func (g *generator) emitTransformScale(e *ast.MethodCall) string {
	args := e.Args
	if len(args) < 1 {
		g.addDiag(diagnostic.Error, ".scale() requires at least 1 argument", e.NodeSpan())
		return g.emitSDF(e.Receiver)
	}

	if len(args) == 1 {
		// Uniform scale.
		s := g.emitScalarExpr(args[0].Value)
		pNew := g.freshVar("p")
		g.emit("vec3 %s = %s / %s;", pNew, g.pointVar, s)

		oldPoint := g.pointVar
		g.pointVar = pNew
		inner := g.emitSDF(e.Receiver)
		g.pointVar = oldPoint

		d := g.freshVar("d")
		g.emit("float %s = %s * %s;", d, inner, s)
		return d
	}

	// Non-uniform scale.
	sx := g.emitScalarExpr(args[0].Value)
	sy := g.emitScalarExpr(args[1].Value)
	sz := "1.0"
	if len(args) >= 3 {
		sz = g.emitScalarExpr(args[2].Value)
	}

	pNew := g.freshVar("p")
	g.emit("vec3 %s = %s / vec3(%s, %s, %s);", pNew, g.pointVar, sx, sy, sz)

	oldPoint := g.pointVar
	g.pointVar = pNew
	inner := g.emitSDF(e.Receiver)
	g.pointVar = oldPoint

	d := g.freshVar("d")
	g.emit("float %s = %s * min(%s, min(%s, %s));", d, inner, sx, sy, sz)
	return d
}

// emitTransformRot handles .rot(deg, axis)
func (g *generator) emitTransformRot(e *ast.MethodCall) string {
	args := e.Args
	if len(args) < 2 {
		g.addDiag(diagnostic.Error, ".rot() requires 2 arguments (degrees, axis)", e.NodeSpan())
		return g.emitSDF(e.Receiver)
	}

	deg := g.emitScalarExpr(args[0].Value)

	// Determine axis.
	axisName := ""
	if ident, ok := args[1].Value.(*ast.Ident); ok {
		axisName = ident.Name
	}

	pNew := g.freshVar("p")
	switch axisName {
	case "x":
		g.emit("vec3 %s = rotateX(%s, radians(%s));", pNew, g.pointVar, deg)
	case "y":
		g.emit("vec3 %s = rotateY(%s, radians(%s));", pNew, g.pointVar, deg)
	case "z":
		// rotateZ isn't in the prefix; emit it inline.
		g.helpers["rotateZ"] = true
		g.emit("vec3 %s = rotateZ(%s, radians(%s));", pNew, g.pointVar, deg)
	default:
		// Arbitrary axis — fall back to Y as default.
		g.emit("vec3 %s = rotateY(%s, radians(%s));", pNew, g.pointVar, deg)
	}

	oldPoint := g.pointVar
	g.pointVar = pNew
	result := g.emitSDF(e.Receiver)
	g.pointVar = oldPoint

	return result
}

// ---------------------------------------------------------------------------
// Color method
// ---------------------------------------------------------------------------

// emitColor handles .color(r, g, b) and .color(#hex)
func (g *generator) emitColor(e *ast.MethodCall) string {
	result := g.emitSDF(e.Receiver)

	var colorExpr string
	if len(e.Args) == 1 {
		// Could be .color(#hex) where the arg is a HexColorLit
		if hex, ok := e.Args[0].Value.(*ast.HexColorLit); ok {
			colorExpr = fmt.Sprintf("vec3(%s, %s, %s)", formatFloat(hex.R), formatFloat(hex.G), formatFloat(hex.B))
		} else {
			colorExpr = g.emitScalarExpr(e.Args[0].Value)
		}
	} else if len(e.Args) == 3 {
		r := g.emitScalarExpr(e.Args[0].Value)
		gv := g.emitScalarExpr(e.Args[1].Value)
		b := g.emitScalarExpr(e.Args[2].Value)
		colorExpr = fmt.Sprintf("vec3(%s, %s, %s)", r, gv, b)
	} else {
		g.addDiag(diagnostic.Error, ".color() requires 1 or 3 arguments", e.NodeSpan())
		return result
	}

	g.colors = append(g.colors, colorEntry{sdfVar: result, colorExpr: colorExpr})
	return result
}

// ---------------------------------------------------------------------------
// Mirror
// ---------------------------------------------------------------------------

// emitMirror handles .mirror(x), .mirror(x, z), etc.
func (g *generator) emitMirror(e *ast.MethodCall) string {
	if len(e.Args) < 1 {
		g.addDiag(diagnostic.Error, ".mirror() requires at least 1 axis argument", e.NodeSpan())
		return g.emitSDF(e.Receiver)
	}

	pNew := g.freshVar("p")
	g.emit("vec3 %s = %s;", pNew, g.pointVar)

	for _, a := range e.Args {
		if a.Name != "" {
			continue // skip named args like origin:
		}
		if ident, ok := a.Value.(*ast.Ident); ok {
			switch ident.Name {
			case "x":
				g.emit("%s.x = abs(%s.x);", pNew, pNew)
			case "y":
				g.emit("%s.y = abs(%s.y);", pNew, pNew)
			case "z":
				g.emit("%s.z = abs(%s.z);", pNew, pNew)
			}
		}
	}

	oldPoint := g.pointVar
	g.pointVar = pNew
	result := g.emitSDF(e.Receiver)
	g.pointVar = oldPoint

	return result
}

// ---------------------------------------------------------------------------
// Repetition
// ---------------------------------------------------------------------------

// emitRep handles .rep(spacing), .rep(sx, sy, sz), .rep(spacing, count: N)
func (g *generator) emitRep(e *ast.MethodCall) string {
	if len(e.Args) < 1 {
		g.addDiag(diagnostic.Error, ".rep() requires at least 1 argument", e.NodeSpan())
		return g.emitSDF(e.Receiver)
	}

	// Check for count: named arg
	var countArg ast.Expr
	var positionalArgs []ast.Arg
	for _, a := range e.Args {
		if a.Name == "count" {
			countArg = a.Value
		} else if a.Name == "" {
			positionalArgs = append(positionalArgs, a)
		}
	}

	pNew := g.freshVar("p")

	if countArg != nil {
		// Clamped repetition: p - s * clamp(round(p/s), -N, N)
		s := g.emitScalarExpr(positionalArgs[0].Value)
		n := g.emitScalarExpr(countArg)
		g.emit("vec3 %s = %s - vec3(%s) * clamp(round(%s / vec3(%s)), vec3(-%s), vec3(%s));",
			pNew, g.pointVar, s, g.pointVar, s, n, n)
	} else if len(positionalArgs) == 1 {
		// Infinite repeat, uniform spacing
		s := g.emitScalarExpr(positionalArgs[0].Value)
		g.emit("vec3 %s = mod(%s + 0.5 * %s, vec3(%s)) - 0.5 * %s;",
			pNew, g.pointVar, s, s, s)
	} else if len(positionalArgs) >= 3 {
		// Per-axis spacing
		sx := g.emitScalarExpr(positionalArgs[0].Value)
		sy := g.emitScalarExpr(positionalArgs[1].Value)
		sz := g.emitScalarExpr(positionalArgs[2].Value)
		sVar := fmt.Sprintf("vec3(%s, %s, %s)", sx, sy, sz)
		g.emit("vec3 %s = mod(%s + 0.5 * %s, %s) - 0.5 * %s;",
			pNew, g.pointVar, sVar, sVar, sVar)
	} else {
		g.addDiag(diagnostic.Error, ".rep() requires 1, 3 positional args, or spacing + count:", e.NodeSpan())
		return g.emitSDF(e.Receiver)
	}

	oldPoint := g.pointVar
	g.pointVar = pNew
	result := g.emitSDF(e.Receiver)
	g.pointVar = oldPoint

	return result
}

// ---------------------------------------------------------------------------
// Morph
// ---------------------------------------------------------------------------

// emitMorph handles .morph(other, t)
func (g *generator) emitMorph(e *ast.MethodCall) string {
	if len(e.Args) < 2 {
		g.addDiag(diagnostic.Error, ".morph() requires 2 arguments (other shape, blend factor)", e.NodeSpan())
		return g.emitSDF(e.Receiver)
	}

	// Emit the receiver SDF
	sdfA := g.emitSDF(e.Receiver)
	// Emit the other shape SDF
	sdfB := g.emitSDF(e.Args[0].Value)
	// Emit the blend factor
	t := g.emitScalarExpr(e.Args[1].Value)

	d := g.freshVar("d")
	g.emit("float %s = mix(%s, %s, %s);", d, sdfA, sdfB, t)
	return d
}

// ---------------------------------------------------------------------------
// Shell / Onion
// ---------------------------------------------------------------------------

// emitShell handles .shell(thickness) and .onion(thickness)
func (g *generator) emitShell(e *ast.MethodCall) string {
	if len(e.Args) < 1 {
		g.addDiag(diagnostic.Error, fmt.Sprintf(".%s() requires 1 argument (thickness)", e.Name), e.NodeSpan())
		return g.emitSDF(e.Receiver)
	}

	inner := g.emitSDF(e.Receiver)
	thickness := g.emitScalarExpr(e.Args[0].Value)

	d := g.freshVar("d")
	g.emit("float %s = abs(%s) - %s;", d, inner, thickness)
	return d
}

// ---------------------------------------------------------------------------
// Displace
// ---------------------------------------------------------------------------

// emitDisplace handles .displace(expr) where expr can reference p
func (g *generator) emitDisplace(e *ast.MethodCall) string {
	if len(e.Args) < 1 {
		g.addDiag(diagnostic.Error, ".displace() requires 1 argument", e.NodeSpan())
		return g.emitSDF(e.Receiver)
	}

	inner := g.emitSDF(e.Receiver)
	displacement := g.emitScalarExpr(e.Args[0].Value)

	d := g.freshVar("d")
	g.emit("float %s = %s + %s;", d, inner, displacement)
	return d
}

// ---------------------------------------------------------------------------
// Dilate
// ---------------------------------------------------------------------------

// emitDilate handles .dilate(r) → sdf(p) - r
func (g *generator) emitDilate(e *ast.MethodCall) string {
	if len(e.Args) < 1 {
		g.addDiag(diagnostic.Error, ".dilate() requires 1 argument", e.NodeSpan())
		return g.emitSDF(e.Receiver)
	}

	inner := g.emitSDF(e.Receiver)
	r := g.emitScalarExpr(e.Args[0].Value)

	d := g.freshVar("d")
	g.emit("float %s = %s - %s;", d, inner, r)
	return d
}

// ---------------------------------------------------------------------------
// Erode
// ---------------------------------------------------------------------------

// emitErode handles .erode(r) → sdf(p) + r
func (g *generator) emitErode(e *ast.MethodCall) string {
	if len(e.Args) < 1 {
		g.addDiag(diagnostic.Error, ".erode() requires 1 argument", e.NodeSpan())
		return g.emitSDF(e.Receiver)
	}

	inner := g.emitSDF(e.Receiver)
	r := g.emitScalarExpr(e.Args[0].Value)

	d := g.freshVar("d")
	g.emit("float %s = %s + %s;", d, inner, r)
	return d
}

// ---------------------------------------------------------------------------
// Round
// ---------------------------------------------------------------------------

// emitRound handles .round(r) → sdf(p) - r
func (g *generator) emitRound(e *ast.MethodCall) string {
	if len(e.Args) < 1 {
		g.addDiag(diagnostic.Error, ".round() requires 1 argument", e.NodeSpan())
		return g.emitSDF(e.Receiver)
	}

	inner := g.emitSDF(e.Receiver)
	r := g.emitScalarExpr(e.Args[0].Value)

	d := g.freshVar("d")
	g.emit("float %s = %s - %s;", d, inner, r)
	return d
}

// ---------------------------------------------------------------------------
// Elongate
// ---------------------------------------------------------------------------

// emitElongate handles .elongate(x, y, z)
func (g *generator) emitElongate(e *ast.MethodCall) string {
	if len(e.Args) < 3 {
		g.addDiag(diagnostic.Error, ".elongate() requires 3 arguments (x, y, z)", e.NodeSpan())
		return g.emitSDF(e.Receiver)
	}

	ex := g.emitScalarExpr(e.Args[0].Value)
	ey := g.emitScalarExpr(e.Args[1].Value)
	ez := g.emitScalarExpr(e.Args[2].Value)

	pNew := g.freshVar("p")
	g.emit("vec3 %s = %s - clamp(%s, -vec3(%s, %s, %s), vec3(%s, %s, %s));",
		pNew, g.pointVar, g.pointVar, ex, ey, ez, ex, ey, ez)

	oldPoint := g.pointVar
	g.pointVar = pNew
	result := g.emitSDF(e.Receiver)
	g.pointVar = oldPoint

	return result
}

// ---------------------------------------------------------------------------
// Twist
// ---------------------------------------------------------------------------

// emitTwist handles .twist(strength) — twist around Y axis
func (g *generator) emitTwist(e *ast.MethodCall) string {
	if len(e.Args) < 1 {
		g.addDiag(diagnostic.Error, ".twist() requires 1 argument (strength)", e.NodeSpan())
		return g.emitSDF(e.Receiver)
	}

	k := g.emitScalarExpr(e.Args[0].Value)

	pNew := g.freshVar("p")
	angleVar := g.freshVar("ta")
	g.emit("float %s = %s * %s.y;", angleVar, k, g.pointVar)
	g.emit("vec3 %s = vec3(cos(%s) * %s.x - sin(%s) * %s.z, %s.y, sin(%s) * %s.x + cos(%s) * %s.z);",
		pNew, angleVar, g.pointVar, angleVar, g.pointVar, g.pointVar, angleVar, g.pointVar, angleVar, g.pointVar)

	oldPoint := g.pointVar
	g.pointVar = pNew
	result := g.emitSDF(e.Receiver)
	g.pointVar = oldPoint

	return result
}

// ---------------------------------------------------------------------------
// Bend
// ---------------------------------------------------------------------------

// emitBend handles .bend(strength)
func (g *generator) emitBend(e *ast.MethodCall) string {
	if len(e.Args) < 1 {
		g.addDiag(diagnostic.Error, ".bend() requires 1 argument (strength)", e.NodeSpan())
		return g.emitSDF(e.Receiver)
	}

	k := g.emitScalarExpr(e.Args[0].Value)

	pNew := g.freshVar("p")
	angleVar := g.freshVar("ba")
	g.emit("float %s = %s * %s.x;", angleVar, k, g.pointVar)
	g.emit("vec3 %s = vec3(cos(%s) * %s.x - sin(%s) * %s.y, sin(%s) * %s.x + cos(%s) * %s.y, %s.z);",
		pNew, angleVar, g.pointVar, angleVar, g.pointVar, angleVar, g.pointVar, angleVar, g.pointVar, g.pointVar)

	oldPoint := g.pointVar
	g.pointVar = pNew
	result := g.emitSDF(e.Receiver)
	g.pointVar = oldPoint

	return result
}

// ---------------------------------------------------------------------------
// Binary expressions (boolean ops & arithmetic)
// ---------------------------------------------------------------------------

func (g *generator) emitBinaryExpr(e *ast.BinaryExpr) string {
	switch e.Op {
	case ast.Union:
		left := g.emitSDF(e.Left)
		right := g.emitSDF(e.Right)
		d := g.freshVar("d")
		g.emit("float %s = opUnion(%s, %s);", d, left, right)
		return d

	case ast.SmoothUnion:
		left := g.emitSDF(e.Left)
		right := g.emitSDF(e.Right)
		d := g.freshVar("d")
		k := "0.5" // default blend
		if e.Blend != nil {
			k = formatFloat(*e.Blend)
		}
		g.emit("float %s = opSmoothUnion(%s, %s, %s);", d, left, right, k)
		return d

	case ast.Subtract:
		left := g.emitSDF(e.Left)
		right := g.emitSDF(e.Right)
		d := g.freshVar("d")
		g.emit("float %s = opSubtract(%s, %s);", d, left, right)
		return d

	case ast.SmoothSubtract:
		g.helpers["opSmoothSubtract"] = true
		left := g.emitSDF(e.Left)
		right := g.emitSDF(e.Right)
		d := g.freshVar("d")
		k := "0.5"
		if e.Blend != nil {
			k = formatFloat(*e.Blend)
		}
		g.emit("float %s = opSmoothSubtract(%s, %s, %s);", d, left, right, k)
		return d

	case ast.Intersect:
		left := g.emitSDF(e.Left)
		right := g.emitSDF(e.Right)
		d := g.freshVar("d")
		g.emit("float %s = opIntersect(%s, %s);", d, left, right)
		return d

	case ast.SmoothIntersect:
		g.helpers["opSmoothIntersect"] = true
		left := g.emitSDF(e.Left)
		right := g.emitSDF(e.Right)
		d := g.freshVar("d")
		k := "0.5"
		if e.Blend != nil {
			k = formatFloat(*e.Blend)
		}
		g.emit("float %s = opSmoothIntersect(%s, %s, %s);", d, left, right, k)
		return d

	case ast.ChamferUnion:
		g.helpers["opChamferUnion"] = true
		left := g.emitSDF(e.Left)
		right := g.emitSDF(e.Right)
		d := g.freshVar("d")
		k := "0.5"
		if e.Blend != nil {
			k = formatFloat(*e.Blend)
		}
		g.emit("float %s = opChamferUnion(%s, %s, %s);", d, left, right, k)
		return d

	case ast.ChamferSubtract:
		g.helpers["opChamferSubtract"] = true
		left := g.emitSDF(e.Left)
		right := g.emitSDF(e.Right)
		d := g.freshVar("d")
		k := "0.5"
		if e.Blend != nil {
			k = formatFloat(*e.Blend)
		}
		g.emit("float %s = opChamferSubtract(%s, %s, %s);", d, left, right, k)
		return d

	case ast.ChamferIntersect:
		g.helpers["opChamferIntersect"] = true
		left := g.emitSDF(e.Left)
		right := g.emitSDF(e.Right)
		d := g.freshVar("d")
		k := "0.5"
		if e.Blend != nil {
			k = formatFloat(*e.Blend)
		}
		g.emit("float %s = opChamferIntersect(%s, %s, %s);", d, left, right, k)
		return d

	// Arithmetic and comparison — emit as scalar expression.
	case ast.Add, ast.Sub, ast.Mul, ast.Div, ast.Mod,
		ast.Eq, ast.Neq, ast.Lt, ast.Gt, ast.Lte, ast.Gte:
		left := g.emitScalarExpr(e.Left)
		right := g.emitScalarExpr(e.Right)
		op := scalarBinaryOp(e.Op)
		return fmt.Sprintf("(%s %s %s)", left, op, right)

	default:
		g.addDiag(diagnostic.Error, fmt.Sprintf("unsupported binary op %s", e.Op), e.NodeSpan())
		return "0.0"
	}
}

// emitUnaryExpr handles unary prefix operators.
func (g *generator) emitUnaryExpr(e *ast.UnaryExpr) string {
	operand := g.emitScalarExpr(e.Operand)
	switch e.Op {
	case ast.Neg:
		return fmt.Sprintf("(-%s)", operand)
	case ast.Not:
		return fmt.Sprintf("(!%s)", operand)
	}
	return operand
}

// ---------------------------------------------------------------------------
// Block expressions
// ---------------------------------------------------------------------------

func (g *generator) emitBlock(block *ast.Block) string {
	// Process statements in the block.
	// Save and restore scope.
	savedScope := make(map[string]scopeEntry)
	for k, v := range g.scope {
		savedScope[k] = v
	}

	for _, stmt := range block.Stmts {
		switch s := stmt.(type) {
		case *ast.AssignStmt:
			if s.Params != nil {
				// Function def inside block.
				g.funcs[s.Name] = s
			} else {
				// Variable assignment.
				g.scope[s.Name] = scopeEntry{kind: entryAST, node: s.Value}
			}
		case *ast.ExprStmt:
			// Side-effectful expression in a block — skip unless it's the last.
			// (handled via Result)
		}
	}

	// Emit the result expression.
	var result string
	if block.Result != nil {
		result = g.emitSDF(block.Result)
	} else {
		result = "0.0"
	}

	// Restore scope.
	g.scope = savedScope

	return result
}

// ---------------------------------------------------------------------------
// For expression (compile-time unrolling)
// ---------------------------------------------------------------------------

func (g *generator) emitForExpr(e *ast.ForExpr) string {
	if g.loopVars == nil {
		g.loopVars = make(map[string]float64)
	}

	return g.unrollIterators(e.Iterators, 0, e.Body, e.NodeSpan())
}

func (g *generator) unrollIterators(iterators []ast.Iterator, idx int, body *ast.Block, span token.Span) string {
	if idx >= len(iterators) {
		// All iterators bound — emit the body.
		return g.emitSDF(body)
	}

	it := iterators[idx]
	startVal := g.evalConstant(it.Start)
	endVal := g.evalConstant(it.End)
	stepVal := 1.0
	if it.Step != nil {
		stepVal = g.evalConstant(it.Step)
	}

	if stepVal == 0 {
		g.addDiag(diagnostic.Error, "for loop step cannot be 0", span)
		return "0.0"
	}

	// Count iterations.
	count := 0
	if stepVal > 0 {
		for v := startVal; v < endVal; v += stepVal {
			count++
		}
	} else {
		for v := startVal; v > endVal; v += stepVal {
			count++
		}
	}

	if count > maxUnrollIterations {
		g.addDiag(diagnostic.Error,
			fmt.Sprintf("for loop would produce %d iterations (max %d)", count, maxUnrollIterations),
			span)
		return "0.0"
	}

	if count == 0 {
		return "0.0"
	}

	var results []string
	v := startVal
	for i := 0; i < count; i++ {
		g.loopVars[it.Name] = v
		result := g.unrollIterators(iterators, idx+1, body, span)
		results = append(results, result)
		v += stepVal
	}
	delete(g.loopVars, it.Name)

	// Chain results with opUnion.
	if len(results) == 1 {
		return results[0]
	}

	combined := results[0]
	for _, r := range results[1:] {
		d := g.freshVar("d")
		g.emit("float %s = opUnion(%s, %s);", d, combined, r)
		combined = d
	}

	return combined
}

// evalConstant evaluates a constant expression at compile time.
func (g *generator) evalConstant(expr ast.Expr) float64 {
	switch e := expr.(type) {
	case *ast.NumberLit:
		return e.Value
	case *ast.Ident:
		if g.loopVars != nil {
			if v, ok := g.loopVars[e.Name]; ok {
				return v
			}
		}
		return 0
	case *ast.UnaryExpr:
		if e.Op == ast.Neg {
			return -g.evalConstant(e.Operand)
		}
		return 0
	case *ast.BinaryExpr:
		l := g.evalConstant(e.Left)
		r := g.evalConstant(e.Right)
		switch e.Op {
		case ast.Add:
			return l + r
		case ast.Sub, ast.Subtract:
			return l - r
		case ast.Mul:
			return l * r
		case ast.Div:
			if r != 0 {
				return l / r
			}
			return 0
		case ast.Mod:
			if r != 0 {
				return math.Mod(l, r)
			}
			return 0
		}
	}
	return 0
}

// ---------------------------------------------------------------------------
// If/Else expression
// ---------------------------------------------------------------------------

func (g *generator) emitIfExpr(e *ast.IfExpr) string {
	cond := g.emitScalarExpr(e.Cond)

	// Emit both branches.
	thenResult := g.emitSDF(e.Then)

	var elseResult string
	if e.Else != nil {
		elseResult = g.emitSDF(e.Else)
	} else {
		elseResult = "0.0"
	}

	d := g.freshVar("d")
	g.emit("float %s = (%s) ? %s : %s;", d, cond, thenResult, elseResult)
	return d
}

// ---------------------------------------------------------------------------
// GLSL escape
// ---------------------------------------------------------------------------

func (g *generator) emitGlslEscape(e *ast.GlslEscape) string {
	// Wrap the raw GLSL code in a helper function.
	fnName := g.freshVar("glsl_sdf")
	// We can't easily emit a function at this point in the body, so we
	// inline the GLSL code. The code must evaluate to a float.
	// We replace the param name with the current point variable.
	code := strings.ReplaceAll(e.Code, e.Param, g.pointVar)

	// If the code contains "return", we need to wrap in a block or function.
	// For simplicity, inline it as an immediately-invoked block isn't possible
	// in GLSL, so emit a helper function.
	_ = fnName
	d := g.freshVar("d")

	// Simple case: the code is just an expression "return expr;"
	if strings.HasPrefix(strings.TrimSpace(code), "return ") {
		expr := strings.TrimSpace(code)
		expr = strings.TrimPrefix(expr, "return ")
		expr = strings.TrimSuffix(expr, ";")
		g.emit("float %s = %s;", d, expr)
		return d
	}

	// Complex case: multi-statement GLSL. Emit inline with a scope block.
	// GLSL supports { } blocks, but we need to get a value out.
	// Emit the statements and use a variable to capture the result.
	g.emit("float %s;", d)
	g.emit("{")
	// Split the code into lines and emit each.
	for _, line := range strings.Split(code, ";") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "return ") {
			expr := strings.TrimPrefix(line, "return ")
			g.emit("    %s = %s;", d, expr)
		} else {
			g.emit("    %s;", line)
		}
	}
	g.emit("}")
	return d
}

// ---------------------------------------------------------------------------
// User-defined function emission
// ---------------------------------------------------------------------------

func (g *generator) emitFuncDef(out *strings.Builder, name string, fn *ast.AssignStmt) {
	if g.emittedFn[name] {
		return
	}
	g.emittedFn[name] = true

	// Build parameter list: always starts with vec3 p.
	var params []string
	params = append(params, "vec3 p")
	for _, p := range fn.Params {
		params = append(params, fmt.Sprintf("float %s", p.Name))
	}

	// Emit the function body into a separate generator context.
	fnGen := &generator{
		scope:     make(map[string]scopeEntry),
		funcs:     g.funcs,
		helpers:   g.helpers,
		emittedFn: g.emittedFn,
		pointVar:  "p",
		loopVars:  nil,
	}

	// Put parameters into scope as float variables.
	for _, p := range fn.Params {
		fnGen.scope[p.Name] = scopeEntry{kind: entryFloat, varName: p.Name}
	}
	// Copy parent scope entries (but not the function's own params).
	for k, v := range g.scope {
		if _, exists := fnGen.scope[k]; !exists {
			fnGen.scope[k] = v
		}
	}

	result := fnGen.emitSDF(fn.Value)

	// Merge any helpers/diags from the function emission.
	for k, v := range fnGen.helpers {
		g.helpers[k] = v
	}
	g.diags = append(g.diags, fnGen.diags...)

	fmt.Fprintf(out, "float fn_%s(%s) {\n", name, strings.Join(params, ", "))
	out.WriteString(fnGen.body.String())
	fmt.Fprintf(out, "    return %s;\n", result)
	out.WriteString("}\n\n")
}

// ---------------------------------------------------------------------------
// GLSL helper functions (emitted only when needed)
// ---------------------------------------------------------------------------

func (g *generator) writeHelpers(out *strings.Builder) {
	if g.helpers["opSmoothSubtract"] {
		out.WriteString(`float opSmoothSubtract(float d1, float d2, float k) {
    float h = clamp(0.5 - 0.5*(d2+d1)/k, 0.0, 1.0);
    return mix(d2, -d1, h) + k*h*(1.0-h);
}
`)
	}

	if g.helpers["opSmoothIntersect"] {
		out.WriteString(`float opSmoothIntersect(float d1, float d2, float k) {
    float h = clamp(0.5 - 0.5*(d2-d1)/k, 0.0, 1.0);
    return mix(d2, d1, h) + k*h*(1.0-h);
}
`)
	}

	if g.helpers["opChamferUnion"] {
		out.WriteString(`float opChamferUnion(float a, float b, float r) {
    return min(min(a, b), (a - r + b) * 0.7071067811865476);
}
`)
	}

	if g.helpers["opChamferSubtract"] {
		out.WriteString(`float opChamferSubtract(float a, float b, float r) {
    return max(max(a, -b), (a + r - b) * 0.7071067811865476);
}
`)
	}

	if g.helpers["opChamferIntersect"] {
		out.WriteString(`float opChamferIntersect(float a, float b, float r) {
    return max(max(a, b), (a + r + b) * 0.7071067811865476);
}
`)
	}

	if g.helpers["rotateZ"] {
		out.WriteString(`vec3 rotateZ(vec3 p, float a) {
    float c = cos(a), s = sin(a);
    return vec3(p.x*c - p.y*s, p.x*s + p.y*c, p.z);
}
`)
	}
}

// ---------------------------------------------------------------------------
// Built-in shapes
// ---------------------------------------------------------------------------

// builtinShapeNames lists all recognized built-in shape identifiers.
var builtinShapeNames = map[string]bool{
	"sphere":    true,
	"box":       true,
	"cylinder":  true,
	"torus":     true,
	"plane":     true,
	"octahedron": true,
	"capsule":   true,
}

// isBuiltinShape reports whether name is a built-in shape.
func isBuiltinShape(name string) bool {
	return builtinShapeNames[name]
}

// shapeDefault returns the GLSL call for a bare shape identifier (no args).
func shapeDefault(name, pv string) string {
	switch name {
	case "sphere":
		return fmt.Sprintf("sdSphere(%s, 1.0)", pv)
	case "box":
		return fmt.Sprintf("sdBox(%s, vec3(1.0))", pv)
	case "cylinder":
		return fmt.Sprintf("sdCylinder(%s, 0.5, 2.0)", pv)
	case "torus":
		return fmt.Sprintf("sdTorus(%s, 1.0, 0.3)", pv)
	case "plane":
		return fmt.Sprintf("sdPlane(%s, vec3(0.0, 1.0, 0.0), 0.0)", pv)
	case "octahedron":
		return fmt.Sprintf("sdOctahedron(%s, 1.0)", pv)
	case "capsule":
		return fmt.Sprintf("sdCapsule(%s, vec3(0.0, -1.0, 0.0), vec3(0.0, 1.0, 0.0), 0.25)", pv)
	}
	return fmt.Sprintf("sdSphere(%s, 1.0)", pv) // fallback
}

// shapeCall returns the GLSL call for a shape function call with arguments.
func (g *generator) shapeCall(name, pv string, args []ast.Arg) string {
	switch name {
	case "sphere":
		if len(args) == 0 {
			return fmt.Sprintf("sdSphere(%s, 1.0)", pv)
		}
		r := g.emitScalarExpr(args[0].Value)
		return fmt.Sprintf("sdSphere(%s, %s)", pv, r)

	case "box":
		if len(args) == 0 {
			return fmt.Sprintf("sdBox(%s, vec3(1.0))", pv)
		}
		if len(args) == 1 {
			s := g.emitScalarExpr(args[0].Value)
			return fmt.Sprintf("sdBox(%s, vec3(%s))", pv, s)
		}
		w := g.emitScalarExpr(args[0].Value)
		h := g.emitScalarExpr(args[1].Value)
		d := "1.0"
		if len(args) >= 3 {
			d = g.emitScalarExpr(args[2].Value)
		}
		return fmt.Sprintf("sdBox(%s, vec3(%s, %s, %s))", pv, w, h, d)

	case "cylinder":
		if len(args) == 0 {
			return fmt.Sprintf("sdCylinder(%s, 0.5, 2.0)", pv)
		}
		r := g.emitScalarExpr(args[0].Value)
		h := "2.0"
		if len(args) >= 2 {
			h = g.emitScalarExpr(args[1].Value)
		}
		return fmt.Sprintf("sdCylinder(%s, %s, %s)", pv, r, h)

	case "torus":
		if len(args) == 0 {
			return fmt.Sprintf("sdTorus(%s, 1.0, 0.3)", pv)
		}
		R := g.emitScalarExpr(args[0].Value)
		r := "0.3"
		if len(args) >= 2 {
			r = g.emitScalarExpr(args[1].Value)
		}
		return fmt.Sprintf("sdTorus(%s, %s, %s)", pv, R, r)

	case "plane":
		if len(args) == 0 {
			return fmt.Sprintf("sdPlane(%s, vec3(0.0, 1.0, 0.0), 0.0)", pv)
		}
		n := g.emitScalarExpr(args[0].Value)
		h := "0.0"
		if len(args) >= 2 {
			h = g.emitScalarExpr(args[1].Value)
		}
		return fmt.Sprintf("sdPlane(%s, %s, %s)", pv, n, h)

	case "octahedron":
		if len(args) == 0 {
			return fmt.Sprintf("sdOctahedron(%s, 1.0)", pv)
		}
		s := g.emitScalarExpr(args[0].Value)
		return fmt.Sprintf("sdOctahedron(%s, %s)", pv, s)

	case "capsule":
		if len(args) == 0 {
			return fmt.Sprintf("sdCapsule(%s, vec3(0.0, -1.0, 0.0), vec3(0.0, 1.0, 0.0), 0.25)", pv)
		}
		if len(args) == 3 {
			a := g.emitScalarExpr(args[0].Value)
			b := g.emitScalarExpr(args[1].Value)
			r := g.emitScalarExpr(args[2].Value)
			return fmt.Sprintf("sdCapsule(%s, %s, %s, %s)", pv, a, b, r)
		}
		var argStrs []string
		for _, a := range args {
			argStrs = append(argStrs, g.emitScalarExpr(a.Value))
		}
		return fmt.Sprintf("sdCapsule(%s, %s)", pv, strings.Join(argStrs, ", "))
	}

	return fmt.Sprintf("sdSphere(%s, 1.0)", pv) // fallback
}
