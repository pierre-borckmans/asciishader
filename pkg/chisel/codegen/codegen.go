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
		emittedFn: make(map[string]map[string]bool),
	}

	// First pass: collect top-level assignments (variables & functions),
	// settings, and find the scene expression.
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
		case *ast.SettingStmt:
			g.processSetting(s)
		}
	}

	// If no scene expression, emit default sphere.
	if sceneExpr == nil {
		sceneExpr = &ast.Ident{Name: "sphere"}
	}

	// Function definitions are emitted on-demand during code generation.
	var fnDefs strings.Builder
	g.fnDefs = &fnDefs

	// Third pass: emit sceneSDF body.
	g.pointVar = "p"
	g.tmpCounter = 0
	result := g.emitSDF(sceneExpr)

	// Build the GLSL output.
	var out strings.Builder

	// Emit raymarch #define overrides first.
	g.writeDefines(&out)

	// Emit helper functions (smooth subtract, noise, easing, etc.) if needed.
	g.writeHelpers(&out)

	// Emit settings as comments (camera, lighting, bg, post).
	g.writeSettingComments(&out)

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
		// Multiple colored shapes — re-emit SDF body with bounds skipped
		// so all distance variables are available for color comparison.
		colorGen := &generator{
			scope:      g.scope,
			funcs:      g.funcs,
			helpers:    g.helpers,
			emittedFn:  g.emittedFn,
			fnDefs:     g.fnDefs,
			pointVar:   "p",
			skipBounds: true,
		}
		colorGen.emitSDF(sceneExpr)
		out.WriteString(colorGen.body.String())
		// Use the color entries from the color pass
		colorColors := colorGen.colors

		// Compare distances: return the closest shape's color.
		for i := 0; i < len(colorColors)-1; i++ {
			cmp := colorColors[i].sdfVar
			restCmp := colorColors[i+1].sdfVar
			for j := i + 2; j < len(colorColors); j++ {
				restCmp = fmt.Sprintf("min(%s, %s)", restCmp, colorColors[j].sdfVar)
			}
			fmt.Fprintf(&out, "    if (%s < %s) return %s;\n", cmp, restCmp, colorColors[i].colorExpr)
		}
		fmt.Fprintf(&out, "    return %s;\n", colorColors[len(colorColors)-1].colorExpr)
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
	emittedFn  map[string]map[string]bool  // which user functions have been emitted, keyed by name then return type
	fnDefs     *strings.Builder            // buffer for emitted function definitions
	skipBounds bool                        // when true, .bounds() passes through without if-wrapper (for sceneColor)
	indent     int                        // current indent level (for nested code)

	// loopVars tracks current loop variable substitution values during unrolling.
	loopVars map[string]float64

	// colors tracks the color associated with each SDF variable for sceneColor generation.
	colors []colorEntry

	// defines tracks #define overrides for raymarch settings.
	defines map[string]string

	// settingComments collects comment lines for camera, light, bg, post settings.
	settingComments []string
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
	case *ast.IfExpr:
		cond := g.emitScalarExpr(e.Cond)
		thenExpr := "vec3(1.0)"
		elseExpr := "vec3(1.0)"
		if e.Then != nil && e.Then.Result != nil {
			thenExpr = g.emitScalarExpr(e.Then.Result)
		}
		if e.Else != nil {
			elseExpr = g.emitScalarExpr(e.Else)
		}
		return fmt.Sprintf("(%s ? %s : %s)", cond, thenExpr, elseExpr)
	case *ast.GlslEscape:
		// In scalar/color context, emit as a helper function returning vec3.
		code := strings.ReplaceAll(e.Code, e.Param, "p")
		code = strings.TrimSpace(code)
		// Simple one-liner: return expr; → inline
		if !strings.Contains(code, "\n") && strings.HasPrefix(code, "return ") {
			code = strings.TrimPrefix(code, "return ")
			code = strings.TrimSuffix(code, ";")
			return strings.ReplaceAll(code, "p", g.pointVar)
		}
		// Multi-statement: emit as helper function
		fnName := g.freshVar("glsl_color")
		if g.fnDefs != nil {
			fmt.Fprintf(g.fnDefs, "vec3 %s(vec3 p) {\n", fnName)
			for _, line := range strings.Split(code, "\n") {
				fmt.Fprintf(g.fnDefs, "    %s\n", line)
			}
			fmt.Fprintf(g.fnDefs, "}\n\n")
		}
		return fmt.Sprintf("%s(%s)", fnName, g.pointVar)
	case *ast.Block:
		// Block in scalar context: return the result expression
		if e.Result != nil {
			return g.emitScalarExpr(e.Result)
		}
		return "0.0"
	default:
		return g.emitSDF(expr)
	}
}

// scalarBinaryOp returns the GLSL operator string for a scalar binary op.
func scalarBinaryOp(op ast.BinaryOp) string {
	switch op {
	case ast.Add:
		return "+"
	case ast.Sub, ast.Subtract:
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
		// Map Chisel names to GLSL names
		glslName := e.Name
		if glslName == "atan2" {
			glslName = "atan" // GLSL atan() accepts 1 or 2 args
		}
		var args []string
		for _, a := range e.Args {
			args = append(args, g.emitScalarExpr(a.Value))
		}
		return fmt.Sprintf("%s(%s)", glslName, strings.Join(args, ", "))
	}

	// Check for noise/easing/utility functions.
	if result, ok := g.emitSpecialFuncCall(e); ok {
		return result
	}

	// User-defined function call in scalar/color context: fn_name_v(p, args...)
	if fn, ok := g.funcs[e.Name]; ok {
		var args []string
		args = append(args, g.pointVar)
		for _, a := range e.Args {
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
		// Ensure the vec3 variant of this function is emitted.
		if g.fnDefs != nil {
			g.emitFuncDef(g.fnDefs, e.Name, fn, "vec3")
		}
		return fmt.Sprintf("fn_%s_v(%s)", e.Name, strings.Join(args, ", "))
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

	// 2D shapes used bare without .extrude()/.revolve(): auto-extrude with height 1.
	if is2DShape(name) {
		g.addDiag(diagnostic.Warning, fmt.Sprintf("2D shape '%s' used without .extrude() or .revolve(); auto-extruding with height 1", name), e.NodeSpan())
		p2d := fmt.Sprintf("%s.xy", g.pointVar)
		d2dExpr := shape2DDefault(name, p2d)
		d := g.freshVar("d")
		g.emit("float %s = sdExtrude(%s, %s.z, 0.5);", d, d2dExpr, g.pointVar)
		return d
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
	// 2D shapes with args but without .extrude()/.revolve(): auto-extrude with height 1.
	if is2DShape(e.Name) {
		g.addDiag(diagnostic.Warning, fmt.Sprintf("2D shape '%s' used without .extrude() or .revolve(); auto-extruding with height 1", e.Name), e.NodeSpan())
		p2d := fmt.Sprintf("%s.xy", g.pointVar)
		d2dExpr := g.shape2DCall(e.Name, p2d, e.Args)
		d := g.freshVar("d")
		g.emit("float %s = sdExtrude(%s, %s.z, 0.5);", d, d2dExpr, g.pointVar)
		return d
	}

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
		// Ensure the float variant of this function is emitted.
		if g.fnDefs != nil {
			g.emitFuncDef(g.fnDefs, e.Name, fn, "float")
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

	// Check for noise/easing/utility functions.
	if result, ok := g.emitSpecialFuncCall(e); ok {
		return result
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
	case "swizzle":
		return g.emitSwizzle(e)
	case "bounds":
		return g.emitBounds(e)
	case "orient":
		return g.emitOrient(e)
	case "flip":
		return g.emitFlip(e)
	case "extrude":
		return g.emitExtrude(e)
	case "extrude_to":
		return g.emitExtrudeTo(e)
	case "revolve":
		return g.emitRevolve(e)
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

// emitSwizzle handles .swizzle(x, z, y) — rearranges point components.
// Args are axis identifiers: x, y, z (mapped to component indices).
func (g *generator) emitSwizzle(e *ast.MethodCall) string {
	if len(e.Args) < 3 {
		g.addDiag(diagnostic.Error, ".swizzle() requires 3 arguments (e.g. x, z, y)", e.NodeSpan())
		return g.emitSDF(e.Receiver)
	}

	// Map each arg to a component name
	components := make([]string, 3)
	for i := 0; i < 3; i++ {
		comp := "x"
		if ident, ok := e.Args[i].Value.(*ast.Ident); ok {
			comp = ident.Name
		}
		components[i] = fmt.Sprintf("%s.%s", g.pointVar, comp)
	}

	pNew := g.freshVar("p")
	g.emit("vec3 %s = vec3(%s, %s, %s);", pNew, components[0], components[1], components[2])

	oldPoint := g.pointVar
	g.pointVar = pNew
	result := g.emitSDF(e.Receiver)
	g.pointVar = oldPoint

	return result
}

// emitBounds handles .bounds(center, half_extents) — bounding box culling.
// Skips evaluation of the inner SDF group if the point is far from the box.
// In sceneColor context (skipBounds=true), passes through without the if-wrapper
// so all SDF variables are available for color distance comparisons.
func (g *generator) emitBounds(e *ast.MethodCall) string {
	if len(e.Args) < 2 || g.skipBounds {
		return g.emitSDF(e.Receiver)
	}

	center := g.emitScalarExpr(e.Args[0].Value)
	halfExt := g.emitScalarExpr(e.Args[1].Value)

	// Compute bounding box distance
	bbDist := g.freshVar("bb")
	g.emit("float %s = sdBox(%s - %s, %s);", bbDist, g.pointVar, center, halfExt)

	// Result variable
	d := g.freshVar("d")
	g.emit("float %s;", d)
	g.emit("if (%s < 0.5) {", bbDist)
	g.indent++

	inner := g.emitSDF(e.Receiver)
	g.emit("%s = %s;", d, inner)

	g.indent--
	g.emit("} else {")
	g.indent++
	g.emit("%s = %s;", d, bbDist)
	g.indent--
	g.emit("}")

	return d
}

// emitOrient handles .orient(axis) — aligns the shape's Y axis to the given direction.
func (g *generator) emitOrient(e *ast.MethodCall) string {
	if len(e.Args) < 1 {
		g.addDiag(diagnostic.Error, ".orient() requires 1 argument (axis)", e.NodeSpan())
		return g.emitSDF(e.Receiver)
	}

	axisName := ""
	if ident, ok := e.Args[0].Value.(*ast.Ident); ok {
		axisName = ident.Name
	}

	pNew := g.freshVar("p")
	switch axisName {
	case "x":
		// Z→X: shape's Z axis now points along X
		g.emit("vec3 %s = %s.zyx;", pNew, g.pointVar)
	case "y":
		// Z→Y: shape's Z axis now points along Y
		g.emit("vec3 %s = %s.xzy;", pNew, g.pointVar)
	default:
		// Z→Z: no change (default orientation)
		pNew = g.pointVar
	}

	if pNew != g.pointVar {
		oldPoint := g.pointVar
		g.pointVar = pNew
		result := g.emitSDF(e.Receiver)
		g.pointVar = oldPoint
		return result
	}
	return g.emitSDF(e.Receiver)
}

// emitFlip handles .flip(axis) — negates a point component without distance correction.
// Useful for mirroring/flipping orientation (e.g. flipping Y for capped torus).
func (g *generator) emitFlip(e *ast.MethodCall) string {
	if len(e.Args) < 1 {
		g.addDiag(diagnostic.Error, ".flip() requires 1 argument (axis)", e.NodeSpan())
		return g.emitSDF(e.Receiver)
	}

	pNew := g.freshVar("p")
	axisName := ""
	if ident, ok := e.Args[0].Value.(*ast.Ident); ok {
		axisName = ident.Name
	}

	switch axisName {
	case "x":
		g.emit("vec3 %s = vec3(-%s.x, %s.y, %s.z);", pNew, g.pointVar, g.pointVar, g.pointVar)
	case "y":
		g.emit("vec3 %s = vec3(%s.x, -%s.y, %s.z);", pNew, g.pointVar, g.pointVar, g.pointVar)
	case "z":
		g.emit("vec3 %s = vec3(%s.x, %s.y, -%s.z);", pNew, g.pointVar, g.pointVar, g.pointVar)
	default:
		g.emit("vec3 %s = %s;", pNew, g.pointVar)
	}

	oldPoint := g.pointVar
	g.pointVar = pNew
	result := g.emitSDF(e.Receiver)
	g.pointVar = oldPoint
	return result
}

// ---------------------------------------------------------------------------
// Extrude & Revolve (2D → 3D)
// ---------------------------------------------------------------------------

// emitExtrude handles .extrude(height) on 2D SDF shapes.
// Generates: sdExtrude(sdf2d(p.xy), p.z, h*0.5)
func (g *generator) emitExtrude(e *ast.MethodCall) string {
	if len(e.Args) < 1 {
		g.addDiag(diagnostic.Error, ".extrude() requires 1 argument (height)", e.NodeSpan())
		return g.emitSDF(e.Receiver)
	}

	h := g.emitScalarExpr(e.Args[0].Value)

	// Emit the 2D SDF using pointVar.xy
	p2d := fmt.Sprintf("%s.xy", g.pointVar)
	d2d := g.emit2DSDF(e.Receiver, p2d)

	d := g.freshVar("d")
	g.emit("float %s = sdExtrude(%s, %s.z, (%s)*0.5);", d, d2d, g.pointVar, h)
	return d
}

// emitExtrudeTo handles .extrude_to(other_2d_shape, height) — morph between
// two 2D profiles along the Z axis.
// Generates: mix(sdf2d_A(p.xy), sdf2d_B(p.xy), t) where t = (p.z + h) / (2*h)
func (g *generator) emitExtrudeTo(e *ast.MethodCall) string {
	if len(e.Args) < 2 {
		g.addDiag(diagnostic.Error, ".extrude_to() requires 2 arguments (target_shape, height)", e.NodeSpan())
		return g.emitSDF(e.Receiver)
	}

	targetExpr := e.Args[0].Value
	h := g.emitScalarExpr(e.Args[1].Value)

	p2d := fmt.Sprintf("%s.xy", g.pointVar)

	// Emit both 2D SDFs
	d2dA := g.emit2DSDF(e.Receiver, p2d)
	d2dB := g.emit2DSDF(targetExpr, p2d)

	// Interpolation factor: 0 at bottom (-h/2), 1 at top (+h/2)
	halfH := g.freshVar("hh")
	g.emit("float %s = (%s)*0.5;", halfH, h)
	t := g.freshVar("t")
	g.emit("float %s = clamp((%s.z + %s) / (2.0 * %s), 0.0, 1.0);", t, g.pointVar, halfH, halfH)

	// Mix the two 2D SDFs and extrude
	d2d := g.freshVar("d2d")
	g.emit("float %s = mix(%s, %s, %s);", d2d, d2dA, d2dB, t)

	d := g.freshVar("d")
	g.emit("float %s = sdExtrude(%s, %s.z, %s);", d, d2d, g.pointVar, halfH)
	return d
}

// emitRevolve handles .revolve(offset) on 2D SDF shapes.
// Creates a surface of revolution: evaluates the 2D SDF at vec2(length(p.xz) - offset, p.y)
func (g *generator) emitRevolve(e *ast.MethodCall) string {
	offset := "0.0"
	if len(e.Args) >= 1 {
		offset = g.emitScalarExpr(e.Args[0].Value)
	}

	// Create a 2D point from revolving: vec2(length(p.xz) - offset, p.y)
	p2d := g.freshVar("p2d")
	g.emit("vec2 %s = vec2(length(%s.xz) - %s, %s.y);", p2d, g.pointVar, offset, g.pointVar)

	// Emit the 2D SDF using this revolved point
	d2d := g.emit2DSDF(e.Receiver, p2d)

	return d2d
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
	code := strings.ReplaceAll(e.Code, e.Param, "p")
	code = strings.TrimSpace(code)

	// Simple one-liner: return expr; → inline
	if !strings.Contains(code, "\n") && strings.HasPrefix(code, "return ") {
		expr := strings.TrimPrefix(code, "return ")
		expr = strings.TrimSuffix(expr, ";")
		expr = strings.ReplaceAll(expr, "p", g.pointVar)
		d := g.freshVar("d")
		g.emit("float %s = %s;", d, expr)
		return d
	}

	// Multi-statement: emit as helper function
	fnName := g.freshVar("glsl_sdf")
	if g.fnDefs != nil {
		fmt.Fprintf(g.fnDefs, "float %s(vec3 p) {\n", fnName)
		for _, line := range strings.Split(code, "\n") {
			fmt.Fprintf(g.fnDefs, "    %s\n", line)
		}
		fmt.Fprintf(g.fnDefs, "}\n\n")
	}
	d := g.freshVar("d")
	g.emit("float %s = %s(%s);", d, fnName, g.pointVar)
	return d
}

// ---------------------------------------------------------------------------
// User-defined function emission
// ---------------------------------------------------------------------------

func (g *generator) emitFuncDef(out *strings.Builder, name string, fn *ast.AssignStmt, retType string) {
	if g.emittedFn[name] == nil {
		g.emittedFn[name] = make(map[string]bool)
	}
	if g.emittedFn[name][retType] {
		return
	}
	g.emittedFn[name][retType] = true

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
		fnDefs:    out, // propagate so nested function calls can emit their definitions
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

	var result string
	if retType == "vec3" {
		result = fnGen.emitScalarExpr(fn.Value)
	} else {
		result = fnGen.emitSDF(fn.Value)
	}

	// Merge any helpers/diags from the function emission.
	for k, v := range fnGen.helpers {
		g.helpers[k] = v
	}
	g.diags = append(g.diags, fnGen.diags...)

	// Use suffix for vec3 variant to avoid GLSL name collision
	fnSuffix := name
	if retType == "vec3" {
		fnSuffix = name + "_v"
	}

	fmt.Fprintf(out, "%s fn_%s(%s) {\n", retType, fnSuffix, strings.Join(params, ", "))
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

	// --- Noise helpers (Task 5.2) ---

	// chisel_hash and chisel_noise are emitted together when "noise" is needed.
	if g.helpers["chisel_noise"] {
		out.WriteString(`float chisel_hash(vec3 p) {
    p = fract(p * 0.3183099 + vec3(0.1, 0.1, 0.1));
    p *= 17.0;
    return fract(p.x * p.y * p.z * (p.x + p.y + p.z));
}
float chisel_noise(vec3 p) {
    vec3 i = floor(p);
    vec3 f = fract(p);
    f = f * f * (3.0 - 2.0 * f);
    return mix(mix(mix(chisel_hash(i+vec3(0,0,0)), chisel_hash(i+vec3(1,0,0)), f.x),
                   mix(chisel_hash(i+vec3(0,1,0)), chisel_hash(i+vec3(1,1,0)), f.x), f.y),
               mix(mix(chisel_hash(i+vec3(0,0,1)), chisel_hash(i+vec3(1,0,1)), f.x),
                   mix(chisel_hash(i+vec3(0,1,1)), chisel_hash(i+vec3(1,1,1)), f.x), f.y), f.z);
}
`)
	}

	if g.helpers["chisel_fbm"] {
		out.WriteString(`float chisel_fbm(vec3 p, int octaves) {
    float v = 0.0, a = 0.5;
    for (int i = 0; i < octaves; i++) {
        v += a * chisel_noise(p);
        p = p * 2.0 + vec3(100.0);
        a *= 0.5;
    }
    return v;
}
`)
	}

	if g.helpers["chisel_voronoi"] {
		out.WriteString(`float chisel_voronoi(vec3 p) {
    vec3 i = floor(p);
    vec3 f = fract(p);
    float d = 1.0;
    for (int x = -1; x <= 1; x++)
    for (int y = -1; y <= 1; y++)
    for (int z = -1; z <= 1; z++) {
        vec3 n = vec3(x, y, z);
        vec3 r = n + chisel_hash(i + n) - f;
        d = min(d, dot(r, r));
    }
    return sqrt(d);
}
`)
	}

	// --- Easing helpers (Task 5.3) ---

	if g.helpers["chisel_ease_in"] {
		out.WriteString("float chisel_ease_in(float t) { return t * t; }\n")
	}
	if g.helpers["chisel_ease_out"] {
		out.WriteString("float chisel_ease_out(float t) { return t * (2.0 - t); }\n")
	}
	if g.helpers["chisel_ease_in_out"] {
		out.WriteString("float chisel_ease_in_out(float t) { return t < 0.5 ? 2.0*t*t : -1.0 + (4.0 - 2.0*t)*t; }\n")
	}
	if g.helpers["chisel_ease_cubic_in"] {
		out.WriteString("float chisel_ease_cubic_in(float t) { return t * t * t; }\n")
	}
	if g.helpers["chisel_ease_cubic_out"] {
		out.WriteString("float chisel_ease_cubic_out(float t) { float f = t - 1.0; return f * f * f + 1.0; }\n")
	}
	if g.helpers["chisel_ease_cubic_in_out"] {
		out.WriteString("float chisel_ease_cubic_in_out(float t) { return t < 0.5 ? 4.0*t*t*t : 1.0 - pow(-2.0*t + 2.0, 3.0) / 2.0; }\n")
	}
	if g.helpers["chisel_ease_elastic"] {
		out.WriteString("float chisel_ease_elastic(float t) { return t == 0.0 ? 0.0 : t == 1.0 ? 1.0 : -pow(2.0, 10.0*t - 10.0) * sin((t*10.0 - 10.75) * 2.094395102393195); }\n")
	}
	if g.helpers["chisel_ease_bounce"] {
		out.WriteString(`float chisel_ease_bounce(float t) {
    t = 1.0 - t;
    if (t < 1.0/2.75) return 1.0 - 7.5625*t*t;
    else if (t < 2.0/2.75) { t -= 1.5/2.75; return 1.0 - (7.5625*t*t + 0.75); }
    else if (t < 2.5/2.75) { t -= 2.25/2.75; return 1.0 - (7.5625*t*t + 0.9375); }
    else { t -= 2.625/2.75; return 1.0 - (7.5625*t*t + 0.984375); }
}
`)
	}
	if g.helpers["chisel_ease_back"] {
		out.WriteString("float chisel_ease_back(float t) { float c1 = 1.70158; float c3 = c1 + 1.0; return c3*t*t*t - c1*t*t; }\n")
	}
	if g.helpers["chisel_ease_expo"] {
		out.WriteString("float chisel_ease_expo(float t) { return t == 0.0 ? 0.0 : pow(2.0, 10.0*t - 10.0); }\n")
	}

	// --- Utility helpers (Task 5.3) ---

	if g.helpers["chisel_remap"] {
		out.WriteString("float chisel_remap(float v, float a, float b, float c, float d) { return c + (d - c) * (v - a) / (b - a); }\n")
	}
}

// ---------------------------------------------------------------------------
// Special function call emission (noise, easing, utility) — Tasks 5.2 & 5.3
// ---------------------------------------------------------------------------

// emitSpecialFuncCall checks if a FuncCall is a noise, easing, or utility
// function and if so emits the appropriate GLSL code. Returns the GLSL
// expression string and true, or ("", false) if the name is not recognized.
func (g *generator) emitSpecialFuncCall(e *ast.FuncCall) (string, bool) {
	switch e.Name {
	// --- Noise functions (Task 5.2) ---
	case "noise":
		g.helpers["chisel_noise"] = true
		var args []string
		for _, a := range e.Args {
			args = append(args, g.emitScalarExpr(a.Value))
		}
		return fmt.Sprintf("chisel_noise(%s)", strings.Join(args, ", ")), true

	case "fbm":
		g.helpers["chisel_noise"] = true
		g.helpers["chisel_fbm"] = true
		// First positional arg is the position; named arg "octaves" sets octave count.
		octaves := "6" // default
		var posArg string
		for _, a := range e.Args {
			if a.Name == "octaves" {
				octaves = g.emitScalarExpr(a.Value)
			} else if a.Name == "" && posArg == "" {
				posArg = g.emitScalarExpr(a.Value)
			}
		}
		if posArg == "" {
			posArg = g.pointVar
		}
		return fmt.Sprintf("chisel_fbm(%s, %s)", posArg, octaves), true

	case "voronoi":
		g.helpers["chisel_noise"] = true
		g.helpers["chisel_voronoi"] = true
		var args []string
		for _, a := range e.Args {
			args = append(args, g.emitScalarExpr(a.Value))
		}
		return fmt.Sprintf("chisel_voronoi(%s)", strings.Join(args, ", ")), true

	// --- Easing functions (Task 5.3) ---
	case "ease_in":
		g.helpers["chisel_ease_in"] = true
		return fmt.Sprintf("chisel_ease_in(%s)", g.emitScalarExpr(e.Args[0].Value)), true
	case "ease_out":
		g.helpers["chisel_ease_out"] = true
		return fmt.Sprintf("chisel_ease_out(%s)", g.emitScalarExpr(e.Args[0].Value)), true
	case "ease_in_out":
		g.helpers["chisel_ease_in_out"] = true
		return fmt.Sprintf("chisel_ease_in_out(%s)", g.emitScalarExpr(e.Args[0].Value)), true
	case "ease_cubic_in":
		g.helpers["chisel_ease_cubic_in"] = true
		return fmt.Sprintf("chisel_ease_cubic_in(%s)", g.emitScalarExpr(e.Args[0].Value)), true
	case "ease_cubic_out":
		g.helpers["chisel_ease_cubic_out"] = true
		return fmt.Sprintf("chisel_ease_cubic_out(%s)", g.emitScalarExpr(e.Args[0].Value)), true
	case "ease_cubic_in_out":
		g.helpers["chisel_ease_cubic_in_out"] = true
		return fmt.Sprintf("chisel_ease_cubic_in_out(%s)", g.emitScalarExpr(e.Args[0].Value)), true
	case "ease_elastic":
		g.helpers["chisel_ease_elastic"] = true
		return fmt.Sprintf("chisel_ease_elastic(%s)", g.emitScalarExpr(e.Args[0].Value)), true
	case "ease_bounce":
		g.helpers["chisel_ease_bounce"] = true
		return fmt.Sprintf("chisel_ease_bounce(%s)", g.emitScalarExpr(e.Args[0].Value)), true
	case "ease_back":
		g.helpers["chisel_ease_back"] = true
		return fmt.Sprintf("chisel_ease_back(%s)", g.emitScalarExpr(e.Args[0].Value)), true
	case "ease_expo":
		g.helpers["chisel_ease_expo"] = true
		return fmt.Sprintf("chisel_ease_expo(%s)", g.emitScalarExpr(e.Args[0].Value)), true

	// --- Utility functions (Task 5.3) ---
	case "pulse":
		// pulse(t) → step(0.5, fract(t))
		return fmt.Sprintf("step(0.5, fract(%s))", g.emitScalarExpr(e.Args[0].Value)), true
	case "saw":
		// saw(t) → fract(t)
		return fmt.Sprintf("fract(%s)", g.emitScalarExpr(e.Args[0].Value)), true
	case "tri":
		// tri(t) → abs(fract(t) - 0.5) * 2.0
		arg := g.emitScalarExpr(e.Args[0].Value)
		return fmt.Sprintf("abs(fract(%s) - 0.5) * 2.0", arg), true
	case "remap":
		// remap(v, a, b, c, d) → chisel_remap(v, a, b, c, d)
		g.helpers["chisel_remap"] = true
		var args []string
		for _, a := range e.Args {
			args = append(args, g.emitScalarExpr(a.Value))
		}
		return fmt.Sprintf("chisel_remap(%s)", strings.Join(args, ", ")), true
	case "saturate":
		// saturate(v) → clamp(v, 0.0, 1.0)
		return fmt.Sprintf("clamp(%s, 0.0, 1.0)", g.emitScalarExpr(e.Args[0].Value)), true
	}

	return "", false
}

// ---------------------------------------------------------------------------
// Settings processing — Tasks 6.1, 6.2, 6.3, 6.4
// ---------------------------------------------------------------------------

// processSetting handles a SettingStmt by extracting metadata, defines, and
// comments depending on the setting kind.
func (g *generator) processSetting(s *ast.SettingStmt) {
	switch s.Kind {
	case "raymarch":
		g.processRaymarchSetting(s)
	case "camera":
		g.processCameraSetting(s)
	case "light":
		g.processLightSetting(s)
	case "bg":
		g.processBgSetting(s)
	case "post":
		g.processPostSetting(s)
	case "debug":
		if mode, ok := s.Body.(string); ok {
			g.settingComments = append(g.settingComments, fmt.Sprintf("// chisel:debug %s", mode))
		}
	case "mat":
		// Material definitions are handled elsewhere; skip silently.
	}
}

// processRaymarchSetting emits #define overrides (Task 6.3).
func (g *generator) processRaymarchSetting(s *ast.SettingStmt) {
	body, ok := s.Body.(map[string]interface{})
	if !ok {
		return
	}
	if g.defines == nil {
		g.defines = make(map[string]string)
	}
	for key, val := range body {
		switch key {
		case "steps":
			if numStr := g.settingValueStr(val); numStr != "" {
				g.defines["MAX_STEPS"] = numStr
			}
		case "precision":
			if numStr := g.settingValueStr(val); numStr != "" {
				g.defines["SURF_DIST"] = numStr
			}
		case "max_dist":
			if numStr := g.settingValueStr(val); numStr != "" {
				g.defines["MAX_DIST"] = numStr
			}
		}
	}
}

// processCameraSetting emits camera metadata as comments (Task 6.1).
func (g *generator) processCameraSetting(s *ast.SettingStmt) {
	switch body := s.Body.(type) {
	case map[string]interface{}:
		for key, val := range body {
			if valStr := g.settingValueStr(val); valStr != "" {
				g.settingComments = append(g.settingComments, fmt.Sprintf("// chisel:camera:%s %s", key, valStr))
			}
		}
	default:
		// camera as a single expression — emit comment.
		g.settingComments = append(g.settingComments, "// chisel:camera settings present")
	}
}

// processLightSetting emits light metadata as comments (Task 6.2).
func (g *generator) processLightSetting(s *ast.SettingStmt) {
	switch body := s.Body.(type) {
	case map[string]interface{}:
		for key, val := range body {
			if valStr := g.settingValueStr(val); valStr != "" {
				g.settingComments = append(g.settingComments, fmt.Sprintf("// chisel:light:%s %s", key, valStr))
			} else {
				g.settingComments = append(g.settingComments, fmt.Sprintf("// chisel:light:%s [block]", key))
			}
		}
	default:
		// light [x, y, z] — a vector expression.
		g.settingComments = append(g.settingComments, "// chisel:light direction override")
	}
}

// processBgSetting emits background metadata as comments (Task 6.1).
func (g *generator) processBgSetting(s *ast.SettingStmt) {
	switch s.Body.(type) {
	case map[string]interface{}:
		g.settingComments = append(g.settingComments, "// chisel:bg gradient settings present")
	default:
		g.settingComments = append(g.settingComments, "// chisel:bg color override")
	}
}

// processPostSetting emits post-processing metadata as comments (Task 6.4).
func (g *generator) processPostSetting(s *ast.SettingStmt) {
	switch body := s.Body.(type) {
	case map[string]interface{}:
		for key, val := range body {
			if valStr := g.settingValueStr(val); valStr != "" {
				g.settingComments = append(g.settingComments, fmt.Sprintf("// chisel:post:%s %s", key, valStr))
			} else {
				g.settingComments = append(g.settingComments, fmt.Sprintf("// chisel:post:%s [block]", key))
			}
		}
	default:
		g.settingComments = append(g.settingComments, "// chisel:post settings present")
	}
}

// settingValueStr converts a setting value (which may be an ast.Expr or a
// nested map) to a string representation suitable for comments or defines.
func (g *generator) settingValueStr(val interface{}) string {
	switch v := val.(type) {
	case ast.Expr:
		// Use a temporary scalar emission.
		return g.emitScalarExpr(v)
	case map[string]interface{}:
		return "" // nested block — can't inline
	case string:
		return v
	default:
		return fmt.Sprintf("%v", v)
	}
}

// writeDefines emits #define directives for raymarch settings (Task 6.3).
func (g *generator) writeDefines(out *strings.Builder) {
	if g.defines == nil {
		return
	}
	// Emit in a deterministic order.
	for _, key := range []string{"MAX_STEPS", "SURF_DIST", "MAX_DIST"} {
		if val, ok := g.defines[key]; ok {
			fmt.Fprintf(out, "#define %s %s\n", key, val)
		}
	}
}

// writeSettingComments emits settings metadata as GLSL comments.
func (g *generator) writeSettingComments(out *strings.Builder) {
	for _, comment := range g.settingComments {
		out.WriteString(comment)
		out.WriteByte('\n')
	}
}

// ---------------------------------------------------------------------------
// Built-in shapes
// ---------------------------------------------------------------------------

// builtinShapeNames lists all recognized built-in shape identifiers.
var builtinShapeNames = map[string]bool{
	"sphere":        true,
	"box":           true,
	"cylinder":      true,
	"torus":         true,
	"plane":         true,
	"octahedron":    true,
	"capsule":       true,
	"pyramid":       true,
	"ellipsoid":     true,
	"cone":          true,
	"rounded_box":   true,
	"box_frame":     true,
	"capped_torus":  true,
	"hex_prism":     true,
	"octagon_prism": true,
	"round_cone":    true,
	"tri_prism":     true,
	"capped_cone":   true,
	"solid_angle":   true,
	"rhombus":       true,
	"horseshoe":         true,
	"rounded_cylinder": true,
	"tetrahedron":      true,
	"dodecahedron":     true,
	"icosahedron":      true,
	"slab":             true,
	// 2D shape primitives (require .extrude() or .revolve() to render).
	"circle":   true,
	"rect":     true,
	"hexagon":  true,
	"triangle": true,
}

// isBuiltinShape reports whether name is a built-in shape.
func isBuiltinShape(name string) bool {
	return builtinShapeNames[name]
}

// builtin2DShapeNames lists the 2D shape identifiers that require extrude/revolve.
var builtin2DShapeNames = map[string]bool{
	"circle":   true,
	"rect":     true,
	"hexagon":  true,
	"triangle": true,
}

// is2DShape reports whether name is a 2D shape primitive.
func is2DShape(name string) bool {
	return builtin2DShapeNames[name]
}

// shape2DDefault returns the GLSL 2D SDF call for a bare shape ident (no args).
func shape2DDefault(name, p2d string) string {
	switch name {
	case "circle":
		return fmt.Sprintf("sdCircle2D(%s, 1.0)", p2d)
	case "rect":
		return fmt.Sprintf("sdRect2D(%s, vec2(0.5))", p2d)
	case "hexagon":
		return fmt.Sprintf("sdHexagon2D(%s, 1.0)", p2d)
	case "triangle":
		return fmt.Sprintf("sdEquilateralTriangle2D(%s, 1.0)", p2d)
	}
	return fmt.Sprintf("sdCircle2D(%s, 1.0)", p2d)
}

// shape2DCall returns the GLSL 2D SDF call for a shape function call with args.
func (g *generator) shape2DCall(name, p2d string, args []ast.Arg) string {
	switch name {
	case "circle":
		if len(args) == 0 {
			return fmt.Sprintf("sdCircle2D(%s, 1.0)", p2d)
		}
		r := g.emitScalarExpr(args[0].Value)
		return fmt.Sprintf("sdCircle2D(%s, %s)", p2d, r)

	case "rect":
		if len(args) == 0 {
			return fmt.Sprintf("sdRect2D(%s, vec2(0.5))", p2d)
		}
		if len(args) == 1 {
			s := g.emitScalarExpr(args[0].Value)
			return fmt.Sprintf("sdRect2D(%s, vec2((%s)*0.5))", p2d, s)
		}
		w := g.emitScalarExpr(args[0].Value)
		h := g.emitScalarExpr(args[1].Value)
		return fmt.Sprintf("sdRect2D(%s, vec2((%s)*0.5, (%s)*0.5))", p2d, w, h)

	case "hexagon":
		if len(args) == 0 {
			return fmt.Sprintf("sdHexagon2D(%s, 1.0)", p2d)
		}
		r := g.emitScalarExpr(args[0].Value)
		return fmt.Sprintf("sdHexagon2D(%s, %s)", p2d, r)

	case "triangle":
		if len(args) == 0 {
			return fmt.Sprintf("sdEquilateralTriangle2D(%s, 1.0)", p2d)
		}
		r := g.emitScalarExpr(args[0].Value)
		return fmt.Sprintf("sdEquilateralTriangle2D(%s, %s)", p2d, r)
	}

	return fmt.Sprintf("sdCircle2D(%s, 1.0)", p2d)
}

// emit2DSDF generates the GLSL for a 2D SDF expression, using p2d as the vec2 point.
// It walks the receiver chain to find the 2D shape call and any transforms applied.
func (g *generator) emit2DSDF(expr ast.Expr, p2d string) string {
	switch e := expr.(type) {
	case *ast.Ident:
		if is2DShape(e.Name) {
			return shape2DDefault(e.Name, p2d)
		}
		// Check scope for AST variable.
		if entry, ok := g.scope[e.Name]; ok && entry.kind == entryAST {
			return g.emit2DSDF(entry.node, p2d)
		}
		return g.emitScalarExpr(expr)
	case *ast.FuncCall:
		if is2DShape(e.Name) {
			return g.shape2DCall(e.Name, p2d, e.Args)
		}
		return g.emitScalarExpr(expr)
	case *ast.MethodCall:
		// Handle transforms on 2D shapes: .at(), .scale(), .rot(), .mirror()
		return g.emit2DMethodCall(e, p2d)
	case *ast.BinaryExpr:
		// 2D boolean operations: (circle - rect), (circle | rect), etc.
		left := g.emit2DSDF(e.Left, p2d)
		right := g.emit2DSDF(e.Right, p2d)
		switch e.Op {
		case ast.Union:
			return fmt.Sprintf("min(%s, %s)", left, right)
		case ast.Subtract:
			return fmt.Sprintf("max(%s, -%s)", left, right)
		case ast.Intersect:
			return fmt.Sprintf("max(%s, %s)", left, right)
		case ast.SmoothUnion:
			g.helpers["opSmoothUnion"] = true
			k := "0.25"
			if e.Blend != nil {
				k = formatFloat(*e.Blend)
			}
			return fmt.Sprintf("opSmoothUnion(%s, %s, %s)", left, right, k)
		default:
			// Arithmetic ops
			op := scalarBinaryOp(e.Op)
			return fmt.Sprintf("(%s %s %s)", left, op, right)
		}
	case *ast.UnaryExpr:
		operand := g.emit2DSDF(e.Operand, p2d)
		if e.Op == ast.Neg {
			return fmt.Sprintf("(-%s)", operand)
		}
		return operand
	default:
		return g.emitScalarExpr(expr)
	}
}

// emit2DMethodCall handles method calls on 2D SDF receivers by applying
// transforms in the XY plane before evaluating the 2D SDF.
func (g *generator) emit2DMethodCall(e *ast.MethodCall, p2d string) string {
	switch e.Name {
	case "at":
		// .at(x, y) on a 2D shape translates in the XY plane
		x, y := "0.0", "0.0"
		if len(e.Args) >= 1 {
			x = g.emitScalarExpr(e.Args[0].Value)
		}
		if len(e.Args) >= 2 {
			y = g.emitScalarExpr(e.Args[1].Value)
		}
		pNew := g.freshVar("p2d")
		g.emit("vec2 %s = %s - vec2(%s, %s);", pNew, p2d, x, y)
		return g.emit2DSDF(e.Receiver, pNew)

	case "scale":
		if len(e.Args) >= 1 {
			s := g.emitScalarExpr(e.Args[0].Value)
			pNew := g.freshVar("p2d")
			g.emit("vec2 %s = %s / %s;", pNew, p2d, s)
			inner := g.emit2DSDF(e.Receiver, pNew)
			return fmt.Sprintf("(%s * %s)", inner, s)
		}
		return g.emit2DSDF(e.Receiver, p2d)

	case "rot":
		if len(e.Args) >= 1 {
			deg := g.emitScalarExpr(e.Args[0].Value)
			pNew := g.freshVar("p2d")
			aVar := g.freshVar("a2d")
			g.emit("float %s = radians(%s);", aVar, deg)
			g.emit("vec2 %s = vec2(cos(%s) * %s.x + sin(%s) * %s.y, -sin(%s) * %s.x + cos(%s) * %s.y);",
				pNew, aVar, p2d, aVar, p2d, aVar, p2d, aVar, p2d)
			return g.emit2DSDF(e.Receiver, pNew)
		}
		return g.emit2DSDF(e.Receiver, p2d)

	case "mirror":
		pNew := g.freshVar("p2d")
		g.emit("vec2 %s = %s;", pNew, p2d)
		for _, a := range e.Args {
			if ident, ok := a.Value.(*ast.Ident); ok {
				switch ident.Name {
				case "x":
					g.emit("%s.x = abs(%s.x);", pNew, pNew)
				case "y":
					g.emit("%s.y = abs(%s.y);", pNew, pNew)
				}
			}
		}
		return g.emit2DSDF(e.Receiver, pNew)

	default:
		// For other methods, try to pass through to the 2D shape
		return g.emit2DSDF(e.Receiver, p2d)
	}
}

// shapeDefault returns the GLSL call for a bare shape identifier (no args).
func shapeDefault(name, pv string) string {
	switch name {
	case "sphere":
		return fmt.Sprintf("sdSphere(%s, 1.0)", pv)
	case "box":
		return fmt.Sprintf("sdBox(%s, vec3(0.5))", pv)
	case "cylinder":
		return fmt.Sprintf("sdCylinder(%s, 0.5, 1.0)", pv)
	case "torus":
		return fmt.Sprintf("sdTorus(%s, 1.0, 0.3)", pv)
	case "plane":
		return fmt.Sprintf("sdPlane(%s, vec3(0.0, 1.0, 0.0), 0.0)", pv)
	case "octahedron":
		return fmt.Sprintf("sdOctahedron(%s, 1.0)", pv)
	case "capsule":
		return fmt.Sprintf("sdCapsule(%s, vec3(0.0, -1.0, 0.0), vec3(0.0, 1.0, 0.0), 0.25)", pv)
	case "pyramid":
		return fmt.Sprintf("sdPyramid(%s, 1.0)", pv)
	case "ellipsoid":
		return fmt.Sprintf("sdEllipsoid(%s, vec3(1.0))", pv)
	case "cone":
		return fmt.Sprintf("sdCone(%s, vec2(0.6, 0.8), 0.45)", pv)
	case "rounded_box":
		return fmt.Sprintf("sdRoundBox(%s, vec3(0.5), 0.1)", pv)
	case "box_frame":
		return fmt.Sprintf("sdBoxFrame(%s, vec3(0.5), 0.05)", pv)
	case "capped_torus":
		return fmt.Sprintf("sdCappedTorus(%s, vec2(0.866025, -0.5), 0.25, 0.05)", pv)
	case "hex_prism":
		return fmt.Sprintf("sdHexPrism(%s, vec2(1.0, 0.25))", pv)
	case "octagon_prism":
		return fmt.Sprintf("sdOctogonPrism(%s, 1.0, 0.25)", pv)
	case "round_cone":
		return fmt.Sprintf("sdRoundCone(%s, 0.2, 0.1, 0.3)", pv)
	case "tri_prism":
		return fmt.Sprintf("sdTriPrism(%s, vec2(0.3, 0.25))", pv)
	case "capped_cone":
		return fmt.Sprintf("sdCappedCone(%s, 0.25, 0.5, 0.2)", pv)
	case "solid_angle":
		return fmt.Sprintf("sdSolidAngle(%s, vec2(0.6, 0.8), 0.4)", pv)
	case "rhombus":
		return fmt.Sprintf("sdRhombus(%s, 0.5, 0.5, 0.1, 0.05)", pv)
	case "horseshoe":
		return fmt.Sprintf("sdHorseshoe(%s, vec2(0.866, 0.5), 0.3, 0.3, vec2(0.05, 0.1))", pv)
	case "rounded_cylinder":
		return fmt.Sprintf("sdRoundedCylinder(%s, 0.5, 0.1, 0.5)", pv)
	case "tetrahedron":
		return fmt.Sprintf("sdTetrahedron(%s, 1.0)", pv)
	case "dodecahedron":
		return fmt.Sprintf("sdDodecahedron(%s, 1.0)", pv)
	case "icosahedron":
		return fmt.Sprintf("sdIcosahedron(%s, 1.0)", pv)
	case "slab":
		return fmt.Sprintf("sdSlab(%s, 0.25)", pv)
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
			return fmt.Sprintf("sdBox(%s, vec3(0.5))", pv)
		}
		if len(args) == 1 {
			s := g.emitScalarExpr(args[0].Value)
			return fmt.Sprintf("sdBox(%s, vec3((%s)*0.5))", pv, s)
		}
		w := g.emitScalarExpr(args[0].Value)
		h := g.emitScalarExpr(args[1].Value)
		d := "1.0"
		if len(args) >= 3 {
			d = g.emitScalarExpr(args[2].Value)
		}
		return fmt.Sprintf("sdBox(%s, vec3((%s)*0.5, (%s)*0.5, (%s)*0.5))", pv, w, h, d)

	case "cylinder":
		if len(args) == 0 {
			return fmt.Sprintf("sdCylinder(%s, 0.5, 1.0)", pv)
		}
		if len(args) == 3 {
			// cylinder(a, b, r) — endpoint variant
			a := g.emitScalarExpr(args[0].Value)
			b := g.emitScalarExpr(args[1].Value)
			r := g.emitScalarExpr(args[2].Value)
			return fmt.Sprintf("sdCylinderAB(%s, %s, %s, %s)", pv, a, b, r)
		}
		r := g.emitScalarExpr(args[0].Value)
		h := "1.0"
		if len(args) >= 2 {
			h = fmt.Sprintf("(%s)*0.5", g.emitScalarExpr(args[1].Value))
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

	case "pyramid":
		h := g.scalarArgOr(args, 0, "1.0")
		return fmt.Sprintf("sdPyramid(%s, %s)", pv, h)

	case "ellipsoid":
		if len(args) == 0 {
			return fmt.Sprintf("sdEllipsoid(%s, vec3(1.0))", pv)
		}
		if len(args) == 1 {
			s := g.emitScalarExpr(args[0].Value)
			return fmt.Sprintf("sdEllipsoid(%s, vec3(%s))", pv, s)
		}
		rx := g.emitScalarExpr(args[0].Value)
		ry := g.emitScalarExpr(args[1].Value)
		rz := g.scalarArgOr(args, 2, "1.0")
		return fmt.Sprintf("sdEllipsoid(%s, vec3(%s, %s, %s))", pv, rx, ry, rz)

	case "cone":
		if len(args) == 0 {
			return fmt.Sprintf("sdCone(%s, vec2(0.6, 0.8), 0.45)", pv)
		}
		// cone(angle_vec2, height) or cone(cx, cy, h)
		if len(args) == 2 {
			c := g.emitScalarExpr(args[0].Value)
			h := g.emitScalarExpr(args[1].Value)
			return fmt.Sprintf("sdCone(%s, %s, %s)", pv, c, h)
		}
		cx := g.emitScalarExpr(args[0].Value)
		cy := g.emitScalarExpr(args[1].Value)
		h := g.scalarArgOr(args, 2, "0.45")
		return fmt.Sprintf("sdCone(%s, vec2(%s, %s), %s)", pv, cx, cy, h)

	case "rounded_box":
		if len(args) == 0 {
			return fmt.Sprintf("sdRoundBox(%s, vec3(0.5), 0.1)", pv)
		}
		if len(args) == 1 {
			s := g.emitScalarExpr(args[0].Value)
			return fmt.Sprintf("sdRoundBox(%s, vec3((%s)*0.5), 0.1)", pv, s)
		}
		if len(args) == 2 {
			s := g.emitScalarExpr(args[0].Value)
			r := g.emitScalarExpr(args[1].Value)
			return fmt.Sprintf("sdRoundBox(%s, vec3((%s)*0.5), %s)", pv, s, r)
		}
		w := g.emitScalarExpr(args[0].Value)
		h := g.emitScalarExpr(args[1].Value)
		d := g.emitScalarExpr(args[2].Value)
		r := g.scalarArgOr(args, 3, "0.1")
		return fmt.Sprintf("sdRoundBox(%s, vec3((%s)*0.5, (%s)*0.5, (%s)*0.5), %s)", pv, w, h, d, r)

	case "box_frame":
		if len(args) == 0 {
			return fmt.Sprintf("sdBoxFrame(%s, vec3(0.5), 0.05)", pv)
		}
		if len(args) == 2 {
			s := g.emitScalarExpr(args[0].Value)
			e := g.emitScalarExpr(args[1].Value)
			return fmt.Sprintf("sdBoxFrame(%s, vec3((%s)*0.5), %s)", pv, s, e)
		}
		w := g.emitScalarExpr(args[0].Value)
		h := g.emitScalarExpr(args[1].Value)
		d := g.emitScalarExpr(args[2].Value)
		e := g.scalarArgOr(args, 3, "0.05")
		return fmt.Sprintf("sdBoxFrame(%s, vec3((%s)*0.5, (%s)*0.5, (%s)*0.5), %s)", pv, w, h, d, e)

	case "capped_torus":
		if len(args) == 0 {
			return fmt.Sprintf("sdCappedTorus(%s, vec2(0.866025, -0.5), 0.25, 0.05)", pv)
		}
		sc := g.emitScalarExpr(args[0].Value)
		ra := g.scalarArgOr(args, 1, "0.25")
		rb := g.scalarArgOr(args, 2, "0.05")
		return fmt.Sprintf("sdCappedTorus(%s, %s, %s, %s)", pv, sc, ra, rb)

	case "hex_prism":
		if len(args) == 0 {
			return fmt.Sprintf("sdHexPrism(%s, vec2(1.0, 0.25))", pv)
		}
		r := g.emitScalarExpr(args[0].Value)
		h := "0.25"
		if len(args) >= 2 {
			h = fmt.Sprintf("(%s)*0.5", g.emitScalarExpr(args[1].Value))
		}
		return fmt.Sprintf("sdHexPrism(%s, vec2(%s, %s))", pv, r, h)

	case "octagon_prism":
		if len(args) == 0 {
			return fmt.Sprintf("sdOctogonPrism(%s, 1.0, 0.25)", pv)
		}
		r := g.emitScalarExpr(args[0].Value)
		h := "0.25"
		if len(args) >= 2 {
			h = fmt.Sprintf("(%s)*0.5", g.emitScalarExpr(args[1].Value))
		}
		return fmt.Sprintf("sdOctogonPrism(%s, %s, %s)", pv, r, h)

	case "round_cone":
		if len(args) == 4 {
			// round_cone(a, b, r1, r2) — endpoint variant
			a := g.emitScalarExpr(args[0].Value)
			b := g.emitScalarExpr(args[1].Value)
			r1 := g.emitScalarExpr(args[2].Value)
			r2 := g.emitScalarExpr(args[3].Value)
			return fmt.Sprintf("sdRoundConeAB(%s, %s, %s, %s, %s)", pv, a, b, r1, r2)
		}
		r1 := g.scalarArgOr(args, 0, "0.2")
		r2 := g.scalarArgOr(args, 1, "0.1")
		h := g.scalarArgOr(args, 2, "0.3")
		return fmt.Sprintf("sdRoundCone(%s, %s, %s, %s)", pv, r1, r2, h)

	case "tri_prism":
		if len(args) == 0 {
			return fmt.Sprintf("sdTriPrism(%s, vec2(0.3, 0.25))", pv)
		}
		s := g.emitScalarExpr(args[0].Value)
		h := "0.25"
		if len(args) >= 2 {
			h = fmt.Sprintf("(%s)*0.5", g.emitScalarExpr(args[1].Value))
		}
		return fmt.Sprintf("sdTriPrism(%s, vec2(%s, %s))", pv, s, h)

	case "capped_cone":
		if len(args) == 4 {
			// capped_cone(a, b, ra, rb) — endpoint variant
			a := g.emitScalarExpr(args[0].Value)
			b := g.emitScalarExpr(args[1].Value)
			ra := g.emitScalarExpr(args[2].Value)
			rb := g.emitScalarExpr(args[3].Value)
			return fmt.Sprintf("sdCappedConeAB(%s, %s, %s, %s, %s)", pv, a, b, ra, rb)
		}
		if len(args) == 0 {
			return fmt.Sprintf("sdCappedCone(%s, 0.25, 0.5, 0.2)", pv)
		}
		h := fmt.Sprintf("(%s)*0.5", g.emitScalarExpr(args[0].Value))
		r1 := g.scalarArgOr(args, 1, "0.5")
		r2 := g.scalarArgOr(args, 2, "0.2")
		return fmt.Sprintf("sdCappedCone(%s, %s, %s, %s)", pv, h, r1, r2)

	case "solid_angle":
		c := g.emitScalarExpr(args[0].Value)
		ra := g.scalarArgOr(args, 1, "0.4")
		return fmt.Sprintf("sdSolidAngle(%s, %s, %s)", pv, c, ra)

	case "rhombus":
		la := g.scalarArgOr(args, 0, "0.5")
		lb := g.scalarArgOr(args, 1, "0.5")
		h := g.scalarArgOr(args, 2, "0.1")
		ra := g.scalarArgOr(args, 3, "0.05")
		return fmt.Sprintf("sdRhombus(%s, %s, %s, %s, %s)", pv, la, lb, h, ra)

	case "horseshoe":
		sc := g.emitScalarExpr(args[0].Value)
		r := g.scalarArgOr(args, 1, "0.3")
		le := g.scalarArgOr(args, 2, "0.3")
		w := g.scalarArgOr(args, 3, "vec2(0.05, 0.1)")
		return fmt.Sprintf("sdHorseshoe(%s, %s, %s, %s, %s)", pv, sc, r, le, w)

	case "rounded_cylinder":
		if len(args) == 0 {
			return fmt.Sprintf("sdRoundedCylinder(%s, 0.5, 0.1, 0.5)", pv)
		}
		ra := g.emitScalarExpr(args[0].Value)
		rb := g.scalarArgOr(args, 1, "0.1")
		h := "0.5"
		if len(args) >= 3 {
			h = fmt.Sprintf("(%s)*0.5", g.emitScalarExpr(args[2].Value))
		}
		return fmt.Sprintf("sdRoundedCylinder(%s, %s, %s, %s)", pv, ra, rb, h)

	case "tetrahedron":
		r := g.scalarArgOr(args, 0, "1.0")
		return fmt.Sprintf("sdTetrahedron(%s, %s)", pv, r)

	case "dodecahedron":
		r := g.scalarArgOr(args, 0, "1.0")
		return fmt.Sprintf("sdDodecahedron(%s, %s)", pv, r)

	case "icosahedron":
		r := g.scalarArgOr(args, 0, "1.0")
		return fmt.Sprintf("sdIcosahedron(%s, %s)", pv, r)

	case "slab":
		if len(args) == 0 {
			return fmt.Sprintf("sdSlab(%s, 0.25)", pv)
		}
		th := g.emitScalarExpr(args[0].Value)
		return fmt.Sprintf("sdSlab(%s, (%s)*0.5)", pv, th)
	}

	return fmt.Sprintf("sdSphere(%s, 1.0)", pv) // fallback
}

// scalarArgOr returns the scalar expression for args[idx], or defaultVal if idx is out of range.
func (g *generator) scalarArgOr(args []ast.Arg, idx int, defaultVal string) string {
	if idx < len(args) {
		return g.emitScalarExpr(args[idx].Value)
	}
	return defaultVal
}
