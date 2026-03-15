// Package lang is the single source of truth for the Chisel language definition.
// All editor tooling (tmLanguage, LSP, tree-sitter) and the analyzer derive from this.
//
//go:generate go run ./gen
package lang

// Shape represents a built-in shape primitive.
type Shape struct {
	Name    string
	Is2D    bool
	MinArgs int
	MaxArgs int
	Doc     string
}

// Method represents a chainable transform/material method.
type Method struct {
	Name    string
	Doc     string
	IsColor bool // .red, .blue, etc — bare color shorthand
}

// Func represents a built-in function.
type Func struct {
	Name string
	Doc  string
}

// Constant represents a built-in constant or variable.
type Constant struct {
	Name string
	Doc  string
	Vec  bool // true for vec3 (p, x, y, z), false for float (PI, t, etc.)
}

// Color represents a named color constant.
type Color struct {
	Name string
}

// Alias maps a common name from other tools to the Chisel equivalent.
type Alias struct {
	From string
	To   string
}

// --- 3D Shapes ---

var Shapes3D = []Shape{
	{"sphere", false, 0, 1, "sphere(radius = 1)\nUnit sphere."},
	{"box", false, 0, 3, "box(width = 1, height = 1, depth = 1)\nAxis-aligned box."},
	{"cylinder", false, 0, 3, "cylinder(radius = 0.5, height = 2)\nCylinder along Y."},
	{"torus", false, 0, 2, "torus(major = 1, minor = 0.3)\nTorus in XZ plane."},
	{"capsule", false, 0, 3, "capsule(a, b, radius = 0.25)\nCapsule between two endpoints."},
	{"cone", false, 0, 3, "cone(bottomRadius, topRadius, height)\nCone/frustum."},
	{"plane", false, 0, 0, "plane\nInfinite ground plane at y=0."},
	{"octahedron", false, 0, 1, "octahedron(size = 1)\nRegular octahedron."},
	{"pyramid", false, 0, 1, "pyramid(height = 1)\nSquare-base pyramid."},
	{"ellipsoid", false, 0, 3, "ellipsoid(rx, ry, rz)\nEllipsoid with per-axis radii."},
	{"rounded_box", false, 0, 4, "rounded_box(width, height, depth, edgeRadius)\nBox with rounded edges. 1 arg: uniform size. 2 args: size + radius. 4 args: w, h, d, radius."},
	{"box_frame", false, 0, 4, "box_frame(width, height, depth, thickness)\nWireframe box edges. 2 args: size + thickness. 4 args: w, h, d, thickness."},
	{"capped_torus", false, 0, 3, "capped_torus(angle, majorR, minorR)\nPartial torus arc."},
	{"hex_prism", false, 0, 2, "hex_prism(radius, height)\nHexagonal prism."},
	{"octagon_prism", false, 0, 2, "octagon_prism(radius, height)\nOctagonal prism."},
	{"round_cone", false, 0, 4, "round_cone(r1, r2, height)\nRounded cone."},
	{"tri_prism", false, 0, 2, "tri_prism(radius, height)\nTriangular prism."},
	{"capped_cone", false, 0, 4, "capped_cone(a, b, ra, rb)\nCapped cone between endpoints."},
	{"solid_angle", false, 0, 2, "solid_angle(angle, radius)\nSolid angle sector."},
	{"rhombus", false, 0, 4, "rhombus(la, lb, h, ra)\nRhombus shape."},
	{"horseshoe", false, 1, 4, "horseshoe(angle, r, thickness, width)\nHorseshoe/arc."},
	{"rounded_cylinder", false, 0, 3, "rounded_cylinder(radius, height, rounding)\nCylinder with rounded edges."},
	{"tetrahedron", false, 0, 1, "tetrahedron(size = 1)\nRegular tetrahedron."},
	{"dodecahedron", false, 0, 1, "dodecahedron(size = 1)\nRegular dodecahedron."},
	{"icosahedron", false, 0, 1, "icosahedron(size = 1)\nRegular icosahedron."},
	{"slab", false, 0, 2, "slab(axis, thickness)\nInfinite slab."},
}

// --- 2D Shapes ---

var Shapes2D = []Shape{
	{"circle", true, 0, 1, "circle(radius = 1)\n2D circle. Use .extrude() to render."},
	{"rect", true, 0, 2, "rect(width = 1, height = 1)\n2D rectangle."},
	{"hexagon", true, 0, 1, "hexagon(radius = 1)\n2D hexagon."},
	{"polygon", true, 1, 1, "polygon(points)\n2D polygon from [x,y] points."},
	{"triangle", true, 0, 1, "triangle(size = 1)\n2D equilateral triangle."},
}

// --- Methods ---

var Methods = []Method{
	// Transforms
	{"at", ".at(x, y, z)\nTranslate. Named args: at(x: 2) or at(y: -1).", false},
	{"scale", ".scale(s) or .scale(x, y, z)\nScale uniformly or per-axis.", false},
	{"rot", ".rot(degrees, axis)\nRotate around an axis.", false},
	{"orient", ".orient(axis)\nAlign shape along a direction.", false},
	{"mirror", ".mirror(axes...)\nMirror across axes. O(1) space folding.", false},
	{"rep", ".rep(spacing) or .rep(spacing, count: N)\nRepeat in space. O(1).", false},
	{"array", ".array(count, radius: r)\nCircular array of copies.", false},
	{"flip", ".flip(axis)\nFlip/reflect across an axis.", false},

	// Deformations
	{"morph", ".morph(other, t)\nBlend between two shapes.", false},
	{"shell", ".shell(thickness)\nHollow out the shape.", false},
	{"onion", ".onion(thickness)\nConcentric shells.", false},
	{"displace", ".displace(expr)\nDisplace surface using expression with p.", false},
	{"dilate", ".dilate(amount)\nExpand outward.", false},
	{"erode", ".erode(amount)\nShrink inward.", false},
	{"round", ".round(radius)\nRound edges.", false},
	{"elongate", ".elongate(x, y, z)\nStretch along axes.", false},
	{"twist", ".twist(amount)\nTwist around Y axis.", false},
	{"bend", ".bend(amount)\nBend the shape.", false},
	{"bend_linear", ".bend_linear(amount)\nLinear bend.", false},
	{"swizzle", ".swizzle(axes)\nReorder/swap coordinate axes.", false},
	{"bounds", ".bounds(size)\nClamp SDF to a bounding box.", false},

	// 2D → 3D
	{"extrude", ".extrude(depth)\nExtrude a 2D shape into 3D.", false},
	{"extrude_to", ".extrude_to(target, height)\nTapered extrusion.", false},
	{"revolve", ".revolve(radius)\nRevolve a 2D shape.", false},

	// Materials
	{"color", ".color(r, g, b) or .color(#hex)\nSet shape color.", false},
	{"metallic", ".metallic(value)\nMetallic property (0..1).", false},
	{"roughness", ".roughness(value)\nRoughness property (0..1).", false},
	{"emission", ".emission(r, g, b) or .emission(intensity)\nEmissive glow.", false},
	{"opacity", ".opacity(value)\nTransparency (0..1).", false},
	{"mat", ".mat(material)\nApply a named material definition.", false},

	// Color shorthands
	{"red", ".red\nShorthand: red.", true},
	{"blue", ".blue\nShorthand: blue.", true},
	{"green", ".green\nShorthand: green.", true},
	{"white", ".white\nShorthand: white.", true},
	{"black", ".black\nShorthand: black.", true},
	{"yellow", ".yellow\nShorthand: yellow.", true},
	{"cyan", ".cyan\nShorthand: cyan.", true},
	{"magenta", ".magenta\nShorthand: magenta.", true},
	{"orange", ".orange\nShorthand: orange.", true},
	{"gray", ".gray\nShorthand: gray.", true},
}

// --- Built-in Functions ---

var Functions = []Func{
	// Trig
	{"sin", "sin(x)\nSine."},
	{"cos", "cos(x)\nCosine."},
	{"tan", "tan(x)\nTangent."},
	{"asin", "asin(x)\nArc sine."},
	{"acos", "acos(x)\nArc cosine."},
	{"atan", "atan(x)\nArc tangent."},
	{"atan2", "atan2(y, x)\nTwo-argument arc tangent."},

	// Power / exponential
	{"pow", "pow(x, n)\nRaise x to power n."},
	{"sqrt", "sqrt(x)\nSquare root."},
	{"exp", "exp(x)\nExponential."},
	{"log", "log(x)\nNatural logarithm."},

	// Rounding
	{"floor", "floor(x)\nRound down."},
	{"ceil", "ceil(x)\nRound up."},
	{"round", "round(x)\nRound to nearest."},
	{"fract", "fract(x)\nFractional part."},

	// Comparison
	{"abs", "abs(x)\nAbsolute value."},
	{"sign", "sign(x)\nSign (-1, 0, or 1)."},
	{"min", "min(a, b)\nMinimum."},
	{"max", "max(a, b)\nMaximum."},

	// Interpolation
	{"mix", "mix(a, b, t)\nLinear interpolation."},
	{"smoothstep", "smoothstep(edge0, edge1, x)\nSmooth Hermite interpolation."},
	{"step", "step(edge, x)\n0 if x < edge, else 1."},
	{"clamp", "clamp(x, lo, hi)\nClamp to range."},
	{"saturate", "saturate(x)\nClamp to [0, 1]."},
	{"remap", "remap(x, a, b, c, d)\nRemap from [a,b] to [c,d]."},

	// Vector
	{"length", "length(v)\nVector length."},
	{"normalize", "normalize(v)\nUnit vector."},
	{"dot", "dot(a, b)\nDot product."},
	{"cross", "cross(a, b)\nCross product."},
	{"distance", "distance(a, b)\nDistance between points."},
	{"reflect", "reflect(v, n)\nReflect v around normal n."},
	{"mod", "mod(x, y)\nModulo."},

	// Angle conversion
	{"radians", "radians(degrees)\nConvert degrees to radians."},
	{"degrees", "degrees(radians)\nConvert radians to degrees."},

	// Noise
	{"noise", "noise(p)\nPerlin/simplex noise at point p."},
	{"fbm", "fbm(p, octaves: 6)\nFractal Brownian motion."},
	{"voronoi", "voronoi(p)\nVoronoi cell noise."},

	// Oscillators
	{"pulse", "pulse(t, duty: 0.5)\nSquare wave, 0..1."},
	{"saw", "saw(t)\nSawtooth wave, 0..1."},
	{"tri", "tri(t)\nTriangle wave, 0..1."},

	// Easing
	{"ease_in", "ease_in(x)\nQuadratic ease in."},
	{"ease_out", "ease_out(x)\nQuadratic ease out."},
	{"ease_in_out", "ease_in_out(x)\nQuadratic ease in-out."},
	{"ease_cubic_in", "ease_cubic_in(x)\nCubic ease in."},
	{"ease_cubic_out", "ease_cubic_out(x)\nCubic ease out."},
	{"ease_cubic_in_out", "ease_cubic_in_out(x)\nCubic ease in-out."},
	{"ease_elastic", "ease_elastic(x)\nElastic ease."},
	{"ease_bounce", "ease_bounce(x)\nBounce ease."},
	{"ease_back", "ease_back(x)\nBack ease (overshoot)."},
	{"ease_expo", "ease_expo(x)\nExponential ease."},

	// Color constructors
	{"rgb", "rgb(r, g, b)\nColor from 0..255 RGB."},
	{"hsl", "hsl(h, s, l)\nColor from HSL."},
	{"hsla", "hsla(h, s, l, a)\nColor from HSLA."},
	{"rgba", "rgba(r, g, b, a)\nColor from 0..255 RGBA."},
}

// --- Constants ---

var Constants = []Constant{
	{"PI", "π ≈ 3.14159", false},
	{"TAU", "2π ≈ 6.28318", false},
	{"E", "Euler's number ≈ 2.71828", false},
	{"t", "Time in seconds since start.", false},
	{"p", "Current evaluation point (vec3).", true},
	{"x", "X axis direction [1,0,0].", true},
	{"y", "Y axis direction [0,1,0].", true},
	{"z", "Z axis direction [0,0,1].", true},
}

// --- Named Colors ---

var Colors = []Color{
	{"red"}, {"green"}, {"blue"}, {"white"}, {"black"}, {"yellow"},
	{"cyan"}, {"magenta"}, {"gray"}, {"orange"}, {"purple"}, {"pink"},
}

// --- Keywords ---

var Keywords = []string{"for", "in", "if", "else", "step", "glsl"}

// SettingForm describes how a setting keyword is parsed.
type SettingForm int

const (
	SettingExprOrBlock SettingForm = iota // light expr  OR  light { ... }
	SettingBlockOnly                      // raymarch { ... }
	SettingCamera                         // camera expr -> expr  OR  camera { ... }
	SettingDebug                          // debug <ident>
	SettingMat                            // mat <name> = { ... }
)

// SettingDef describes a settings keyword and its syntactic form.
type SettingDef struct {
	Name string
	Form SettingForm
}

// Settings lists all settings keywords with their syntactic form.
var Settings = []SettingDef{
	{"light", SettingExprOrBlock},
	{"camera", SettingCamera},
	{"bg", SettingExprOrBlock},
	{"raymarch", SettingBlockOnly},
	{"post", SettingBlockOnly},
	{"debug", SettingDebug},
	{"mat", SettingMat},
}

// SettingNames returns just the keyword strings (for highlights, etc.).
func SettingNames() []string {
	out := make([]string, len(Settings))
	for i, s := range Settings {
		out[i] = s.Name
	}
	return out
}

// --- Grammar: Operator Precedence ---

// Assoc represents operator associativity for Pratt parsing.
type Assoc int

const (
	Left Assoc = iota
	Right
)

// Operator represents a binary operator with its precedence level.
type Operator struct {
	Token string // operator string, e.g. "|", "|~"
	Prec  int    // precedence level (higher = tighter binding)
	Assoc Assoc  // associativity
	Blend bool   // true for smooth/chamfer ops that take an optional blend radius
}

// Precedence levels used by the Pratt parser and tree-sitter grammar.
// Higher number = tighter binding.
const (
	PrecRange     = 0 // ..
	PrecUnion     = 1 // | |~ |/
	PrecSubtract  = 2 // - -~ -/
	PrecIntersect = 3 // & &~ &/
	PrecCompare   = 4 // == != < > <= >=
	PrecAddSub    = 5 // +
	PrecMulDiv    = 6 // * / %
	PrecUnary     = 7 // unary - !
	PrecPostfix   = 8 // .method()
)

// Operators lists all binary operators with their precedence.
// The parser's infixPrecedence() and the tree-sitter grammar are
// both derived from this table.
var Operators = []Operator{
	// Range (used in for loops: 0..10)
	{"..", PrecRange, Left, false},
	// Union (CSG)
	{"|", PrecUnion, Left, false},
	{"|~", PrecUnion, Left, true},
	{"|/", PrecUnion, Left, true},
	{"|@", PrecUnion, Left, true},
	{"|!", PrecUnion, Left, true},
	{"|^", PrecUnion, Left, true},
	// Subtract (CSG) — TokMinus is also arithmetic; the analyzer resolves.
	{"-", PrecSubtract, Left, false},
	{"-~", PrecSubtract, Left, true},
	{"-/", PrecSubtract, Left, true},
	// Intersect (CSG)
	{"&", PrecIntersect, Left, false},
	{"&~", PrecIntersect, Left, true},
	{"&/", PrecIntersect, Left, true},
	// Comparison
	{"==", PrecCompare, Left, false},
	{"!=", PrecCompare, Left, false},
	{"<", PrecCompare, Left, false},
	{">", PrecCompare, Left, false},
	{"<=", PrecCompare, Left, false},
	{">=", PrecCompare, Left, false},
	// Additive arithmetic (- is shared with subtract above)
	{"+", PrecAddSub, Left, false},
	// Multiplicative arithmetic
	{"*", PrecMulDiv, Left, false},
	{"/", PrecMulDiv, Left, false},
	{"%", PrecMulDiv, Left, false},
}

// UnaryOperator represents a unary prefix operator.
type UnaryOperator struct {
	Token string
	Prec  int
}

// UnaryOperators lists unary prefix operators.
var UnaryOperators = []UnaryOperator{
	{"-", PrecUnary},
	{"!", PrecUnary},
}

// --- Grammar: Terminal Patterns ---

// Terminals defines regex patterns for lexical tokens, shared between
// the Go lexer, tree-sitter grammar, and TextMate grammar.
var Terminals = struct {
	Number   string // numeric literal
	HexColor string // hex color literal
	Ident    string // identifier
}{
	Number:   `\d+(\.\d+)?([eE][+-]?\d+)?`,
	HexColor: `#[0-9a-fA-F]{3,8}`,
	Ident:    `[a-zA-Z_][a-zA-Z0-9_]*`,
}

// --- Method Aliases (for error suggestions) ---

var MethodAliases = map[string]string{
	"translate": "at",
	"move":      "at",
	"position":  "at",
	"rotate":    "rot",
	"rotation":  "rot",
	"size":      "scale",
	"resize":    "scale",
	"repeat":    "rep",
	"duplicate": "array",
	"hollow":    "shell",
	"offset":    "at",
}

// --- Derived helpers (used by analyzer, LSP, codegen) ---

// ShapeNames returns all shape names (3D + 2D).
func ShapeNames() []string {
	out := make([]string, 0, len(Shapes3D)+len(Shapes2D))
	for _, s := range Shapes3D {
		out = append(out, s.Name)
	}
	for _, s := range Shapes2D {
		out = append(out, s.Name)
	}
	return out
}

// MethodNames returns all method names.
func MethodNames() []string {
	out := make([]string, 0, len(Methods))
	for _, m := range Methods {
		out = append(out, m.Name)
	}
	return out
}

// FuncNames returns all built-in function names.
func FuncNames() []string {
	out := make([]string, 0, len(Functions))
	for _, f := range Functions {
		out = append(out, f.Name)
	}
	return out
}

// ColorNames returns all named color names.
func ColorNames() []string {
	out := make([]string, 0, len(Colors))
	for _, c := range Colors {
		out = append(out, c.Name)
	}
	return out
}

// ShapeDoc returns doc for a shape, or empty string.
func ShapeDoc(name string) string {
	for _, s := range Shapes3D {
		if s.Name == name {
			return s.Doc
		}
	}
	for _, s := range Shapes2D {
		if s.Name == name {
			return s.Doc
		}
	}
	return ""
}

// MethodDoc returns doc for a method, or empty string.
func MethodDoc(name string) string {
	for _, m := range Methods {
		if m.Name == name {
			return m.Doc
		}
	}
	return ""
}

// FuncDoc returns doc for a function, or empty string.
func FuncDoc(name string) string {
	for _, f := range Functions {
		if f.Name == name {
			return f.Doc
		}
	}
	return ""
}
