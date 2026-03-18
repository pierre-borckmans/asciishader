package codegen_test

import (
	"strings"
	"testing"

	"asciishader/pkg/chisel/compiler/codegen"
	"asciishader/pkg/chisel/compiler/lexer"
	"asciishader/pkg/chisel/compiler/parser"
)

// compile runs the full pipeline: lex → parse → codegen and returns the GLSL output.
// It fails the test if there are any error-level diagnostics.
func compile(t *testing.T, chisel string) string {
	t.Helper()
	tokens, lexDiags := lexer.Lex("test.chisel", chisel)
	for _, d := range lexDiags {
		t.Logf("lex diag: %s", d.Error())
	}

	prog, parseDiags := parser.Parse(tokens)
	for _, d := range parseDiags {
		t.Logf("parse diag: %s", d.Error())
	}

	glsl, genDiags := codegen.Generate(prog)
	for _, d := range genDiags {
		t.Logf("codegen diag: %s", d.Error())
	}

	return glsl
}

// assertContains checks that glsl contains the expected substring.
func assertContains(t *testing.T, glsl, expected, msg string) {
	t.Helper()
	if !strings.Contains(glsl, expected) {
		t.Errorf("%s: expected GLSL to contain %q\nGot:\n%s", msg, expected, glsl)
	}
}

// assertNotContains checks that glsl does NOT contain the substring.
func assertNotContains(t *testing.T, glsl, unexpected, msg string) {
	t.Helper()
	if strings.Contains(glsl, unexpected) {
		t.Errorf("%s: expected GLSL to NOT contain %q\nGot:\n%s", msg, unexpected, glsl)
	}
}

// ---------------------------------------------------------------------------
// Task 2.1 — Infrastructure
// ---------------------------------------------------------------------------

func TestInfrastructure_EmptyProgram(t *testing.T) {
	glsl := compile(t, "")
	assertContains(t, glsl, "float sceneSDF(vec3 p)", "sceneSDF wrapper")
	assertContains(t, glsl, "return", "sceneSDF return")
	assertContains(t, glsl, "vec3 sceneColor(vec3 p)", "sceneColor wrapper")
	assertContains(t, glsl, "vec3(1.0)", "sceneColor default white")
}

func TestInfrastructure_SphereDefault(t *testing.T) {
	glsl := compile(t, "sphere")
	assertContains(t, glsl, "float sceneSDF(vec3 p)", "sceneSDF wrapper")
	assertContains(t, glsl, "sdSphere(p, 1.0)", "default sphere")
	assertContains(t, glsl, "return", "return statement")
}

// ---------------------------------------------------------------------------
// Task 2.2 — Basic Shapes
// ---------------------------------------------------------------------------

func TestShape_SphereBare(t *testing.T) {
	glsl := compile(t, "sphere")
	assertContains(t, glsl, "sdSphere(p, 1.0)", "bare sphere")
}

func TestShape_SphereWithRadius(t *testing.T) {
	glsl := compile(t, "sphere(2)")
	assertContains(t, glsl, "sdSphere(p, 2.0)", "sphere(2)")
}

func TestShape_BoxBare(t *testing.T) {
	glsl := compile(t, "box")
	assertContains(t, glsl, "sdBox(p, vec3(0.5))", "bare box")
}

func TestShape_BoxDimensions(t *testing.T) {
	glsl := compile(t, "box(2, 1, 3)")
	assertContains(t, glsl, "sdBox(p, vec3((2.0)*0.5, (1.0)*0.5, (3.0)*0.5))", "box(2,1,3)")
}

func TestShape_BoxUniform(t *testing.T) {
	glsl := compile(t, "box(2)")
	assertContains(t, glsl, "sdBox(p, vec3((2.0)*0.5))", "box(2)")
}

func TestShape_CylinderBare(t *testing.T) {
	glsl := compile(t, "cylinder")
	assertContains(t, glsl, "sdCylinder(p, 0.5, 1.0)", "bare cylinder")
}

func TestShape_CylinderArgs(t *testing.T) {
	glsl := compile(t, "cylinder(1, 3)")
	assertContains(t, glsl, "sdCylinder(p, 1.0, (3.0)*0.5)", "cylinder(1,3)")
}

func TestShape_TorusBare(t *testing.T) {
	glsl := compile(t, "torus")
	assertContains(t, glsl, "sdTorus(p, 1.0, 0.3)", "bare torus")
}

func TestShape_TorusArgs(t *testing.T) {
	glsl := compile(t, "torus(2, 0.5)")
	assertContains(t, glsl, "sdTorus(p, 2.0, 0.5)", "torus(2,0.5)")
}

func TestShape_Plane(t *testing.T) {
	glsl := compile(t, "plane")
	assertContains(t, glsl, "sdPlane", "plane")
}

func TestShape_OctahedronBare(t *testing.T) {
	glsl := compile(t, "octahedron")
	assertContains(t, glsl, "sdOctahedron", "octahedron")
}

func TestShape_OctahedronArg(t *testing.T) {
	glsl := compile(t, "octahedron(2)")
	assertContains(t, glsl, "sdOctahedron(p, 2.0)", "octahedron(2)")
}

// ---------------------------------------------------------------------------
// Task 2.3 — Transforms
// ---------------------------------------------------------------------------

func TestTransform_At(t *testing.T) {
	glsl := compile(t, "sphere.at(2, 0, 0)")
	assertContains(t, glsl, "p - vec3(2.0, 0.0, 0.0)", ".at translation")
	assertContains(t, glsl, "sdSphere", "sphere SDF after .at")
}

func TestTransform_ScaleUniform(t *testing.T) {
	glsl := compile(t, "sphere.scale(2)")
	assertContains(t, glsl, "/ 2.0", "scale divide")
	assertContains(t, glsl, "* 2.0", "scale multiply")
}

func TestTransform_RotY(t *testing.T) {
	glsl := compile(t, "sphere.rot(45, y)")
	assertContains(t, glsl, "rotateY", "rotateY")
	assertContains(t, glsl, "radians", "radians conversion")
}

func TestTransform_RotX(t *testing.T) {
	glsl := compile(t, "sphere.rot(90, x)")
	assertContains(t, glsl, "rotateX", "rotateX")
	assertContains(t, glsl, "radians", "radians conversion")
}

func TestTransform_AtThenScale(t *testing.T) {
	glsl := compile(t, "sphere.at(1, 0, 0).scale(2)")
	assertContains(t, glsl, "sdSphere", "sphere present")
	assertContains(t, glsl, "vec3(1.0, 0.0, 0.0)", "translation vector")
	assertContains(t, glsl, "/ 2.0", "scale division")
}

// ---------------------------------------------------------------------------
// Task 2.4 — Boolean Operations
// ---------------------------------------------------------------------------

func TestBoolean_Union(t *testing.T) {
	glsl := compile(t, "sphere | box")
	assertContains(t, glsl, "opUnion", "union op")
	assertContains(t, glsl, "sdSphere", "sphere in union")
	assertContains(t, glsl, "sdBox", "box in union")
}

func TestBoolean_Subtract(t *testing.T) {
	glsl := compile(t, "sphere - box")
	assertContains(t, glsl, "opSubtract", "subtract op")
}

func TestBoolean_Intersect(t *testing.T) {
	glsl := compile(t, "sphere & box")
	assertContains(t, glsl, "opIntersect", "intersect op")
}

func TestBoolean_SmoothUnion(t *testing.T) {
	glsl := compile(t, "sphere |~0.3 box")
	assertContains(t, glsl, "opSmoothUnion", "smooth union op")
	assertContains(t, glsl, "0.3", "blend radius")
}

func TestBoolean_SmoothSubtract(t *testing.T) {
	glsl := compile(t, "sphere -~0.2 box")
	assertContains(t, glsl, "opSmoothSubtract", "smooth subtract op")
	assertContains(t, glsl, "0.2", "blend radius")
	// Should emit helper function.
	assertContains(t, glsl, "float opSmoothSubtract(float d1, float d2, float k)", "helper emitted")
}

func TestBoolean_SmoothIntersect(t *testing.T) {
	glsl := compile(t, "sphere &~0.1 box")
	assertContains(t, glsl, "opSmoothIntersect", "smooth intersect op")
	assertContains(t, glsl, "float opSmoothIntersect(float d1, float d2, float k)", "helper emitted")
}

func TestBoolean_ChamferUnion(t *testing.T) {
	glsl := compile(t, "sphere |/0.3 box")
	assertContains(t, glsl, "opChamferUnion", "chamfer union op")
	assertContains(t, glsl, "float opChamferUnion(float a, float b, float r)", "helper emitted")
}

func TestBoolean_Avoid(t *testing.T) {
	glsl := compile(t, "sphere |^0.3 box")
	assertContains(t, glsl, "opAvoid", "avoid op")
	assertContains(t, glsl, "0.3", "blend radius")
	assertContains(t, glsl, "float opAvoid(float a, float b, float k)", "helper emitted")
}

func TestBoolean_Repel(t *testing.T) {
	glsl := compile(t, "sphere |!0.5 box")
	assertContains(t, glsl, "opRepel", "repel op")
	assertContains(t, glsl, "0.5", "blend radius")
	assertContains(t, glsl, "float opRepel(float a, float b, float k)", "helper emitted")
}

func TestBoolean_Paint(t *testing.T) {
	glsl := compile(t, "sphere |@0.3 box.red")
	assertContains(t, glsl, "opPaint", "paint op")
	assertContains(t, glsl, "0.3", "blend radius")
	assertContains(t, glsl, "float opPaint(float a, float b, float k)", "helper emitted")
}

func TestBoolean_Nested(t *testing.T) {
	glsl := compile(t, "(sphere | box) - cylinder")
	assertContains(t, glsl, "opUnion", "union")
	assertContains(t, glsl, "opSubtract", "subtract")
	assertContains(t, glsl, "sdCylinder", "cylinder")
}

func TestBoolean_ImplicitUnion(t *testing.T) {
	glsl := compile(t, "sphere\nbox")
	assertContains(t, glsl, "opUnion", "implicit union from newlines")
	assertContains(t, glsl, "sdSphere", "sphere")
	assertContains(t, glsl, "sdBox", "box")
}

// ---------------------------------------------------------------------------
// Task 2.5 — Variables & Functions
// ---------------------------------------------------------------------------

func TestVariable_ScalarAssignment(t *testing.T) {
	glsl := compile(t, "r = 2\nsphere(r)")
	assertContains(t, glsl, "sdSphere(p, 2.0)", "scalar variable inlined in shape call")
}

func TestVariable_SDFAssignment(t *testing.T) {
	glsl := compile(t, "base = sphere(2)\nbase")
	assertContains(t, glsl, "sdSphere(p, 2.0)", "SDF variable re-emitted")
}

func TestFunction_Basic(t *testing.T) {
	glsl := compile(t, "f(x) = sphere(x)\nf(2)")
	assertContains(t, glsl, "fn_f", "function name")
	assertContains(t, glsl, "vec3 p", "p parameter in function")
	assertContains(t, glsl, "float x", "x parameter in function")
	assertContains(t, glsl, "sdSphere(p, x)", "shape call with param")
}

func TestFunction_DefaultParam(t *testing.T) {
	glsl := compile(t, "f(x, y = 1) = sphere(x)\nf(2)")
	assertContains(t, glsl, "fn_f", "function name")
	// Call should include default value for y.
	assertContains(t, glsl, "fn_f(p, 2.0, 1.0)", "call with default param")
}

func TestBlock_ImplicitUnion(t *testing.T) {
	glsl := compile(t, "{ sphere\n  box }")
	assertContains(t, glsl, "opUnion", "block implicit union")
	assertContains(t, glsl, "sdSphere", "sphere in block")
	assertContains(t, glsl, "sdBox", "box in block")
}

// ---------------------------------------------------------------------------
// Task 2.6 — For Loops
// ---------------------------------------------------------------------------

func TestFor_BasicUnroll(t *testing.T) {
	glsl := compile(t, "for i in 0..3 { sphere.at(i, 0, 0) }")
	// Should have 3 sphere calls (i=0, 1, 2).
	count := strings.Count(glsl, "sdSphere")
	if count != 3 {
		t.Errorf("expected 3 sdSphere calls, got %d\n%s", count, glsl)
	}
	// Should have opUnion chains.
	assertContains(t, glsl, "opUnion", "for loop union chain")
}

func TestFor_MultiIterator(t *testing.T) {
	glsl := compile(t, "for i in 0..2, j in 0..2 { sphere.at(i, 0, j) }")
	// 2x2 = 4 sphere calls.
	count := strings.Count(glsl, "sdSphere")
	if count != 4 {
		t.Errorf("expected 4 sdSphere calls, got %d\n%s", count, glsl)
	}
}

func TestFor_LoopVariableSubstitution(t *testing.T) {
	glsl := compile(t, "for i in 0..3 { sphere.at(i, 0, 0) }")
	// Each iteration should use a different offset value.
	assertContains(t, glsl, "vec3(0.0, 0.0, 0.0)", "i=0 offset")
	assertContains(t, glsl, "vec3(1.0, 0.0, 0.0)", "i=1 offset")
	assertContains(t, glsl, "vec3(2.0, 0.0, 0.0)", "i=2 offset")
}

// ---------------------------------------------------------------------------
// Task 2.7 — If/Else
// ---------------------------------------------------------------------------

func TestIfElse_Ternary(t *testing.T) {
	glsl := compile(t, "if true { sphere } else { box }")
	assertContains(t, glsl, "?", "ternary operator")
	assertContains(t, glsl, ":", "ternary colon")
	assertContains(t, glsl, "sdSphere", "sphere branch")
	assertContains(t, glsl, "sdBox", "box branch")
}

// ---------------------------------------------------------------------------
// Full pipeline structure tests
// ---------------------------------------------------------------------------

func TestFullPipeline_Structure(t *testing.T) {
	glsl := compile(t, "sphere")
	// Must start with or contain sceneSDF function definition.
	assertContains(t, glsl, "float sceneSDF(vec3 p)", "sceneSDF function")
	assertContains(t, glsl, "return", "return statement")
	assertContains(t, glsl, "vec3 sceneColor(vec3 p)", "sceneColor function")
}

func TestFullPipeline_ComplexScene(t *testing.T) {
	glsl := compile(t, `
sphere(2) - cylinder(0.5, 6)
`)
	assertContains(t, glsl, "sdSphere(p, 2.0)", "sphere(2)")
	assertContains(t, glsl, "sdCylinder", "cylinder")
	assertContains(t, glsl, "opSubtract", "subtract")
}

func TestFullPipeline_FunctionAndCall(t *testing.T) {
	glsl := compile(t, `
pillar(h) = cylinder(0.3, h)
pillar(3)
`)
	assertContains(t, glsl, "fn_pillar", "function definition")
	assertContains(t, glsl, "sdCylinder(p, 0.3, (h)*0.5)", "cylinder in function body")
	assertContains(t, glsl, "fn_pillar(p, 3.0)", "function call")
}

func TestFullPipeline_TransformChain(t *testing.T) {
	glsl := compile(t, "sphere.at(1, 0, 0).rot(45, y)")
	assertContains(t, glsl, "rotateY", "rotation")
	assertContains(t, glsl, "vec3(1.0, 0.0, 0.0)", "translation")
	assertContains(t, glsl, "sdSphere", "sphere")
}

// ---------------------------------------------------------------------------
// Helpers don't pollute output when not used
// ---------------------------------------------------------------------------

func TestHelpers_NotEmittedWhenUnused(t *testing.T) {
	glsl := compile(t, "sphere | box")
	assertNotContains(t, glsl, "opSmoothSubtract", "no smooth subtract helper")
	assertNotContains(t, glsl, "opChamferUnion", "no chamfer union helper")
}

func TestHelpers_EmittedWhenUsed(t *testing.T) {
	glsl := compile(t, "sphere -~0.3 box")
	assertContains(t, glsl, "float opSmoothSubtract(float d1, float d2, float k)", "smooth subtract helper")
}

// ---------------------------------------------------------------------------
// Task 3.1 — Basic Color
// ---------------------------------------------------------------------------

func TestColor_NamedRed(t *testing.T) {
	glsl := compile(t, "sphere.red")
	assertContains(t, glsl, "vec3(1.0, 0.0, 0.0)", "red color in sceneColor")
	assertContains(t, glsl, "sceneColor", "sceneColor function")
}

func TestColor_NamedBlue(t *testing.T) {
	glsl := compile(t, "sphere.blue")
	assertContains(t, glsl, "vec3(0.0, 0.0, 1.0)", "blue color")
}

func TestColor_NamedGreen(t *testing.T) {
	glsl := compile(t, "sphere.green")
	assertContains(t, glsl, "vec3(0.0, 1.0, 0.0)", "green color")
}

func TestColor_NamedWhite(t *testing.T) {
	glsl := compile(t, "sphere.white")
	assertContains(t, glsl, "vec3(1.0, 1.0, 1.0)", "white color")
}

func TestColor_NamedYellow(t *testing.T) {
	glsl := compile(t, "sphere.yellow")
	assertContains(t, glsl, "vec3(1.0, 1.0, 0.0)", "yellow color")
}

func TestColor_NamedOrange(t *testing.T) {
	glsl := compile(t, "sphere.orange")
	assertContains(t, glsl, "vec3(1.0, 0.5, 0.0)", "orange color")
}

func TestColor_RGB(t *testing.T) {
	glsl := compile(t, "sphere.color(0.5, 0.5, 0.5)")
	assertContains(t, glsl, "vec3(0.5, 0.5, 0.5)", "custom RGB color in sceneColor")
}

func TestColor_HexColor(t *testing.T) {
	glsl := compile(t, "sphere.color(#ff0000)")
	assertContains(t, glsl, "vec3(1.0, 0.0, 0.0)", "hex color red")
}

func TestColor_MultiColorUnion(t *testing.T) {
	glsl := compile(t, "sphere.red | box.blue.at(2,0,0)")
	// sceneColor should have if/else comparing distances
	assertContains(t, glsl, "vec3(1.0, 0.0, 0.0)", "red color present")
	assertContains(t, glsl, "vec3(0.0, 0.0, 1.0)", "blue color present")
	// Should have comparison logic in sceneColor
	assertContains(t, glsl, "if (", "distance comparison")
	assertContains(t, glsl, "return vec3(1.0, 0.0, 0.0)", "return red for closer")
	assertContains(t, glsl, "return vec3(0.0, 0.0, 1.0)", "return blue for farther")
}

func TestColor_DefaultWhite(t *testing.T) {
	glsl := compile(t, "sphere")
	assertContains(t, glsl, "return vec3(1.0)", "default white when no color")
}

// ---------------------------------------------------------------------------
// Task 4.1 — Mirror
// ---------------------------------------------------------------------------

func TestMirror_SingleAxis(t *testing.T) {
	glsl := compile(t, "sphere.at(2,0,0).mirror(x)")
	assertContains(t, glsl, "abs(", "abs applied for mirror")
	assertContains(t, glsl, ".x = abs(", "abs on x component")
	assertContains(t, glsl, "sdSphere", "sphere SDF")
}

func TestMirror_TwoAxes(t *testing.T) {
	glsl := compile(t, "sphere.at(1,0,1).mirror(x, z)")
	assertContains(t, glsl, ".x = abs(", "abs on x component")
	assertContains(t, glsl, ".z = abs(", "abs on z component")
}

// ---------------------------------------------------------------------------
// Task 4.2 — Repetition
// ---------------------------------------------------------------------------

func TestRep_Infinite(t *testing.T) {
	glsl := compile(t, "sphere(0.3).rep(2)")
	assertContains(t, glsl, "mod(", "mod-based repetition")
	assertContains(t, glsl, "sdSphere", "sphere inside rep")
}

func TestRep_Clamped(t *testing.T) {
	glsl := compile(t, "sphere(0.3).rep(2, count: 5)")
	assertContains(t, glsl, "clamp(", "clamp for limited repetition")
	assertContains(t, glsl, "round(", "round for grid snapping")
	assertContains(t, glsl, "sdSphere", "sphere inside rep")
}

// ---------------------------------------------------------------------------
// Task 4.3 — Morph, Shell, Onion, Displace, etc.
// ---------------------------------------------------------------------------

func TestMorph_Basic(t *testing.T) {
	glsl := compile(t, "sphere.morph(box, 0.5)")
	assertContains(t, glsl, "mix(", "mix of two SDFs")
	assertContains(t, glsl, "sdSphere", "sphere SDF in morph")
	assertContains(t, glsl, "sdBox", "box SDF in morph")
	assertContains(t, glsl, "0.5", "blend factor")
}

func TestShell_Basic(t *testing.T) {
	glsl := compile(t, "sphere.shell(0.05)")
	assertContains(t, glsl, "abs(", "abs for shell")
	assertContains(t, glsl, "- 0.05", "thickness subtraction")
}

func TestOnion_Basic(t *testing.T) {
	glsl := compile(t, "sphere.onion(0.1)")
	assertContains(t, glsl, "abs(", "abs for onion")
	assertContains(t, glsl, "- 0.1", "thickness subtraction")
}

func TestDisplace_Basic(t *testing.T) {
	glsl := compile(t, "sphere.displace(sin(p.x * 10) * 0.1)")
	assertContains(t, glsl, "sin(", "sin in displacement expression")
	assertContains(t, glsl, "p.x", "p.x reference in displacement")
	assertContains(t, glsl, "sdSphere", "sphere SDF")
}

func TestRound_Basic(t *testing.T) {
	glsl := compile(t, "sphere.round(0.1)")
	assertContains(t, glsl, "- 0.1", "round offset subtraction")
	assertContains(t, glsl, "sdSphere", "sphere SDF")
}

func TestDilate_Basic(t *testing.T) {
	glsl := compile(t, "sphere.dilate(0.1)")
	assertContains(t, glsl, "- 0.1", "dilate offset subtraction")
	assertContains(t, glsl, "sdSphere", "sphere SDF")
}

func TestErode_Basic(t *testing.T) {
	glsl := compile(t, "sphere.erode(0.1)")
	assertContains(t, glsl, "+ 0.1", "erode offset addition")
	assertContains(t, glsl, "sdSphere", "sphere SDF")
}

func TestElongate_Basic(t *testing.T) {
	glsl := compile(t, "sphere.elongate(1, 0, 0)")
	assertContains(t, glsl, "clamp(", "clamp for elongation")
	assertContains(t, glsl, "sdSphere", "sphere SDF")
}

func TestTwist_Basic(t *testing.T) {
	glsl := compile(t, "sphere.twist(0.5)")
	assertContains(t, glsl, "cos(", "cos in twist")
	assertContains(t, glsl, "sin(", "sin in twist")
	assertContains(t, glsl, "sdSphere", "sphere SDF")
}

func TestBend_Basic(t *testing.T) {
	glsl := compile(t, "sphere.bend(0.3)")
	assertContains(t, glsl, "cos(", "cos in bend")
	assertContains(t, glsl, "sin(", "sin in bend")
	assertContains(t, glsl, "sdSphere", "sphere SDF")
}

// ---------------------------------------------------------------------------
// Task 5.1 — Time & Signals
// ---------------------------------------------------------------------------

func TestTime_SinT(t *testing.T) {
	glsl := compile(t, "sphere.at(0, sin(t), 0)")
	assertContains(t, glsl, "sin(uTime)", "sin(t) maps to sin(uTime)")
}

func TestTime_ScaleWithT(t *testing.T) {
	glsl := compile(t, "sphere.scale(1 + sin(t) * 0.2)")
	assertContains(t, glsl, "uTime", "t maps to uTime")
	assertContains(t, glsl, "sin(uTime)", "sin(t) expression")
}

func TestTime_PI(t *testing.T) {
	glsl := compile(t, "sphere.at(PI, 0, 0)")
	assertContains(t, glsl, "PI", "PI constant")
}

func TestTime_TAU(t *testing.T) {
	glsl := compile(t, "sphere.at(TAU, 0, 0)")
	assertContains(t, glsl, "2.0 * PI", "TAU maps to 2.0 * PI")
}

func TestTime_E(t *testing.T) {
	glsl := compile(t, "sphere.at(E, 0, 0)")
	assertContains(t, glsl, "2.71828183", "E constant")
}

// ---------------------------------------------------------------------------
// Task 5.2 — Noise Functions
// ---------------------------------------------------------------------------

func TestNoise_BasicNoise(t *testing.T) {
	glsl := compile(t, "sphere.displace(noise(p * 5) * 0.1)")
	assertContains(t, glsl, "chisel_noise", "noise function call")
	assertContains(t, glsl, "chisel_hash", "hash helper emitted")
	assertContains(t, glsl, "float chisel_noise(vec3 p)", "noise function definition")
}

func TestNoise_FBM(t *testing.T) {
	glsl := compile(t, "sphere.displace(fbm(p, octaves: 4) * 0.1)")
	assertContains(t, glsl, "chisel_fbm", "fbm function call")
	assertContains(t, glsl, "chisel_noise", "noise helper emitted for fbm")
	assertContains(t, glsl, "float chisel_fbm(vec3 p, int octaves)", "fbm function definition")
	assertContains(t, glsl, "4", "octaves value")
}

func TestNoise_FBMDefaultOctaves(t *testing.T) {
	glsl := compile(t, "sphere.displace(fbm(p) * 0.1)")
	assertContains(t, glsl, "chisel_fbm", "fbm function call")
	assertContains(t, glsl, "6", "default 6 octaves")
}

func TestNoise_Voronoi(t *testing.T) {
	glsl := compile(t, "sphere.displace(voronoi(p * 3) * 0.1)")
	assertContains(t, glsl, "chisel_voronoi", "voronoi function call")
	assertContains(t, glsl, "chisel_hash", "hash helper emitted for voronoi")
	assertContains(t, glsl, "float chisel_voronoi(vec3 p)", "voronoi function definition")
}

func TestNoise_NotEmittedWhenUnused(t *testing.T) {
	glsl := compile(t, "sphere")
	assertNotContains(t, glsl, "chisel_noise", "no noise when unused")
	assertNotContains(t, glsl, "chisel_hash", "no hash when unused")
	assertNotContains(t, glsl, "chisel_fbm", "no fbm when unused")
	assertNotContains(t, glsl, "chisel_voronoi", "no voronoi when unused")
}

// ---------------------------------------------------------------------------
// Task 5.3 — Easing Functions
// ---------------------------------------------------------------------------

func TestEasing_EaseIn(t *testing.T) {
	glsl := compile(t, "sphere.at(0, ease_in(fract(t)), 0)")
	assertContains(t, glsl, "chisel_ease_in", "ease_in function call")
	assertContains(t, glsl, "float chisel_ease_in(float t)", "ease_in definition")
}

func TestEasing_EaseOut(t *testing.T) {
	glsl := compile(t, "sphere.at(0, ease_out(fract(t)), 0)")
	assertContains(t, glsl, "chisel_ease_out", "ease_out function call")
	assertContains(t, glsl, "float chisel_ease_out(float t)", "ease_out definition")
}

func TestEasing_EaseInOut(t *testing.T) {
	glsl := compile(t, "sphere.at(0, ease_in_out(fract(t)), 0)")
	assertContains(t, glsl, "chisel_ease_in_out", "ease_in_out function call")
}

func TestEasing_CubicIn(t *testing.T) {
	glsl := compile(t, "sphere.at(0, ease_cubic_in(fract(t)), 0)")
	assertContains(t, glsl, "chisel_ease_cubic_in", "ease_cubic_in function call")
}

func TestEasing_CubicOut(t *testing.T) {
	glsl := compile(t, "sphere.at(0, ease_cubic_out(fract(t)), 0)")
	assertContains(t, glsl, "chisel_ease_cubic_out", "ease_cubic_out function call")
}

func TestEasing_CubicInOut(t *testing.T) {
	glsl := compile(t, "sphere.at(0, ease_cubic_in_out(fract(t)), 0)")
	assertContains(t, glsl, "chisel_ease_cubic_in_out", "ease_cubic_in_out function call")
}

func TestEasing_Elastic(t *testing.T) {
	glsl := compile(t, "sphere.at(0, ease_elastic(fract(t)), 0)")
	assertContains(t, glsl, "chisel_ease_elastic", "ease_elastic function call")
}

func TestEasing_Bounce(t *testing.T) {
	glsl := compile(t, "sphere.at(0, ease_bounce(fract(t)), 0)")
	assertContains(t, glsl, "chisel_ease_bounce", "ease_bounce function call")
}

func TestEasing_Back(t *testing.T) {
	glsl := compile(t, "sphere.at(0, ease_back(fract(t)), 0)")
	assertContains(t, glsl, "chisel_ease_back", "ease_back function call")
}

func TestEasing_Expo(t *testing.T) {
	glsl := compile(t, "sphere.at(0, ease_expo(fract(t)), 0)")
	assertContains(t, glsl, "chisel_ease_expo", "ease_expo function call")
}

func TestEasing_NotEmittedWhenUnused(t *testing.T) {
	glsl := compile(t, "sphere")
	assertNotContains(t, glsl, "chisel_ease", "no easing when unused")
}

// ---------------------------------------------------------------------------
// 2D SDF Pipeline — Extrude & Revolve
// ---------------------------------------------------------------------------

func TestExtrude_Circle(t *testing.T) {
	glsl := compile(t, "circle(2).extrude(3)")
	assertContains(t, glsl, "sdCircle2D", "2D circle SDF")
	assertContains(t, glsl, "sdExtrude", "extrude wrapper")
	assertContains(t, glsl, "2.0", "circle radius")
	assertContains(t, glsl, "(3.0)*0.5", "half-height")
}

func TestExtrude_Rect(t *testing.T) {
	glsl := compile(t, "rect(2, 1).extrude(1)")
	assertContains(t, glsl, "sdRect2D", "2D rect SDF")
	assertContains(t, glsl, "sdExtrude", "extrude wrapper")
	assertContains(t, glsl, "(2.0)*0.5", "half-width")
	assertContains(t, glsl, "(1.0)*0.5", "half-height")
}

func TestExtrude_Triangle(t *testing.T) {
	glsl := compile(t, "triangle(0.5).extrude(2)")
	assertContains(t, glsl, "sdEquilateralTriangle2D", "2D triangle SDF")
	assertContains(t, glsl, "sdExtrude", "extrude wrapper")
	assertContains(t, glsl, "0.5", "triangle radius")
}

func TestExtrude_HexagonBare(t *testing.T) {
	glsl := compile(t, "hexagon.extrude(1)")
	assertContains(t, glsl, "sdHexagon2D", "2D hexagon SDF")
	assertContains(t, glsl, "sdExtrude", "extrude wrapper")
}

func TestRevolve_Circle(t *testing.T) {
	glsl := compile(t, "circle(0.3).revolve(2)")
	assertContains(t, glsl, "sdCircle2D", "2D circle SDF")
	assertContains(t, glsl, "length(", "length for revolve radius")
	assertContains(t, glsl, ".xz", "XZ plane for revolve")
}

func TestRevolve_Hexagon(t *testing.T) {
	glsl := compile(t, "hexagon(1).revolve(3)")
	assertContains(t, glsl, "sdHexagon2D", "2D hexagon SDF")
	assertContains(t, glsl, "length(", "length for revolve radius")
	assertContains(t, glsl, ".xz", "XZ plane for revolve")
}

func TestRevolve_NoOffset(t *testing.T) {
	glsl := compile(t, "circle(1).revolve()")
	assertContains(t, glsl, "sdCircle2D", "2D circle SDF")
	assertContains(t, glsl, "0.0", "default zero offset")
}

func TestEgg_Revolve(t *testing.T) {
	glsl := compile(t, "egg(0.116, 0.034, 0.1, 0.5).revolve(0)")
	assertContains(t, glsl, "sdEgg2D", "2D egg SDF")
	assertContains(t, glsl, "0.116", "he param")
	assertContains(t, glsl, "0.034", "ra param")
	assertContains(t, glsl, "0.5", "bu param")
}

func TestEgg_Extrude(t *testing.T) {
	glsl := compile(t, "egg(0.4, 0.2, 0.3, 0.5).extrude(1)")
	assertContains(t, glsl, "sdEgg2D", "2D egg SDF")
	assertContains(t, glsl, "sdExtrude", "extrude wrapper")
}

func TestEgg_Default(t *testing.T) {
	glsl := compile(t, "egg.revolve(0)")
	assertContains(t, glsl, "sdEgg2D", "2D egg SDF")
	assertContains(t, glsl, "0.5", "default he or bu")
}

func TestExtrude_CircleBareIdent(t *testing.T) {
	glsl := compile(t, "circle.extrude(2)")
	assertContains(t, glsl, "sdCircle2D", "2D circle SDF from bare ident")
	assertContains(t, glsl, "sdExtrude", "extrude wrapper")
	assertContains(t, glsl, "1.0", "default radius 1")
}

func TestExtrude_RectBareIdent(t *testing.T) {
	glsl := compile(t, "rect.extrude(1)")
	assertContains(t, glsl, "sdRect2D", "2D rect SDF from bare ident")
	assertContains(t, glsl, "sdExtrude", "extrude wrapper")
}

func Test2DShape_BareCircleAutoExtrude(t *testing.T) {
	// Using a 2D shape without extrude/revolve should still compile (with warning)
	glsl := compile(t, "circle(2)")
	assertContains(t, glsl, "sdCircle2D", "2D circle SDF")
	assertContains(t, glsl, "sdExtrude", "auto-extrude fallback")
}

func TestExtrude_WithAt(t *testing.T) {
	glsl := compile(t, "circle(2).at(1, 0).extrude(3)")
	assertContains(t, glsl, "sdCircle2D", "2D circle SDF")
	assertContains(t, glsl, "sdExtrude", "extrude wrapper")
	assertContains(t, glsl, "vec2(1.0, 0.0)", "2D translation")
}

func TestExtrude_WithScale(t *testing.T) {
	glsl := compile(t, "circle(1).scale(2).extrude(3)")
	assertContains(t, glsl, "sdCircle2D", "2D circle SDF")
	assertContains(t, glsl, "sdExtrude", "extrude wrapper")
	assertContains(t, glsl, "/ 2.0", "scale division in 2D")
}

// ---------------------------------------------------------------------------
// Task 5.3 — Utility Functions (pulse, saw, tri, remap, saturate)
// ---------------------------------------------------------------------------

func TestUtility_Pulse(t *testing.T) {
	glsl := compile(t, "sphere.at(0, pulse(t), 0)")
	assertContains(t, glsl, "step(0.5, fract(", "pulse maps to step(0.5, fract(...))")
}

func TestUtility_Saw(t *testing.T) {
	glsl := compile(t, "sphere.at(0, saw(t), 0)")
	assertContains(t, glsl, "fract(uTime)", "saw maps to fract(t)")
}

func TestUtility_Tri(t *testing.T) {
	glsl := compile(t, "sphere.at(0, tri(t), 0)")
	assertContains(t, glsl, "abs(fract(", "tri maps to abs(fract(...))")
	assertContains(t, glsl, "* 2.0", "tri scale factor")
}

func TestUtility_Remap(t *testing.T) {
	glsl := compile(t, "sphere.at(0, remap(t, 0, 1, -2, 2), 0)")
	assertContains(t, glsl, "chisel_remap", "remap function call")
	assertContains(t, glsl, "float chisel_remap(", "remap definition emitted")
}

func TestUtility_Saturate(t *testing.T) {
	glsl := compile(t, "sphere.at(0, saturate(t), 0)")
	assertContains(t, glsl, "clamp(uTime, 0.0, 1.0)", "saturate maps to clamp")
}

// ---------------------------------------------------------------------------
// Task 6.1 — Camera & Background
// ---------------------------------------------------------------------------

func TestSetting_CameraBlock(t *testing.T) {
	glsl := compile(t, "camera { pos: [0, 2, 5] }\nsphere")
	assertContains(t, glsl, "sceneSDF", "sceneSDF still emitted")
	assertContains(t, glsl, "sdSphere", "sphere still works with camera setting")
	assertContains(t, glsl, "#define CAMERA_POS", "camera pos define emitted")
}

func TestSetting_CameraShorthand(t *testing.T) {
	glsl := compile(t, "camera [sin(t)*4, 2, cos(t)*4] -> [0, 0, 0]\nsphere")
	assertContains(t, glsl, "#define CAMERA_POS", "camera pos define emitted")
	assertContains(t, glsl, "#define CAMERA_TARGET", "camera target define emitted")
	assertContains(t, glsl, "sin(uTime)", "time expression in camera pos")
}

func TestSetting_BgHex(t *testing.T) {
	glsl := compile(t, "bg #1a1a2e\nsphere")
	assertContains(t, glsl, "sceneSDF", "sceneSDF still emitted")
	assertContains(t, glsl, "sdSphere", "sphere still works with bg setting")
	assertContains(t, glsl, "sceneBg", "sceneBg function emitted")
	assertContains(t, glsl, "HAS_SCENE_BG", "HAS_SCENE_BG define emitted")
}

func TestSetting_BgLinearGradient(t *testing.T) {
	glsl := compile(t, "bg { start: #112, stop: #334, angle: 90 }\nsphere")
	assertContains(t, glsl, "sceneBg", "sceneBg function emitted")
	assertContains(t, glsl, "mix(a, b", "linear gradient uses mix")
	assertContains(t, glsl, "cos(ang)", "uses angle direction")
}

func TestSetting_BgRadialGradient(t *testing.T) {
	glsl := compile(t, "bg { center: #fff, edge: #000 }\nsphere")
	assertContains(t, glsl, "sceneBg", "sceneBg function emitted")
	assertContains(t, glsl, "mix(ctr, edg", "radial gradient uses mix")
	assertContains(t, glsl, "length(uv - 0.5)", "radial uses distance from center")
}

// ---------------------------------------------------------------------------
// Transparency / Opacity
// ---------------------------------------------------------------------------

func TestOpacityMethod(t *testing.T) {
	glsl := compile(t, "sphere.color(#f00).opacity(0.5)")
	assertContains(t, glsl, "sceneOpacity", "sceneOpacity function emitted")
	assertContains(t, glsl, "HAS_TRANSPARENCY", "HAS_TRANSPARENCY define emitted")
	assertContains(t, glsl, "0.5", "opacity value present")
}

func TestOpacityFromHexAlpha(t *testing.T) {
	glsl := compile(t, "sphere.color(#ff000080)")
	assertContains(t, glsl, "sceneOpacity", "sceneOpacity from hex alpha")
	assertContains(t, glsl, "HAS_TRANSPARENCY", "HAS_TRANSPARENCY define emitted")
}

func TestNoTransparencyNoDefine(t *testing.T) {
	glsl := compile(t, "sphere.color(#ff0000)")
	assertNotContains(t, glsl, "HAS_TRANSPARENCY", "no transparency when all opaque")
	assertNotContains(t, glsl, "sceneOpacity", "no sceneOpacity when all opaque")
}

func TestColor4Args(t *testing.T) {
	glsl := compile(t, "sphere.color(1, 0, 0, 0.5)")
	assertContains(t, glsl, "sceneOpacity", "sceneOpacity from 4-arg color")
	assertContains(t, glsl, "0.5", "opacity value present")
}

// ---------------------------------------------------------------------------
// Task 6.2 — Lighting
// ---------------------------------------------------------------------------

func TestSetting_LightVector(t *testing.T) {
	glsl := compile(t, "light [-1, -1, -1]\nsphere")
	assertContains(t, glsl, "sceneSDF", "sceneSDF still emitted")
	assertContains(t, glsl, "sdSphere", "sphere still works with light setting")
	assertContains(t, glsl, "#define LIGHT_DIR", "light direction emitted as #define")
}

func TestSetting_LightBlock(t *testing.T) {
	glsl := compile(t, "light { ambient: 0.2 }\nsphere")
	assertContains(t, glsl, "sceneSDF", "sceneSDF still emitted")
	assertContains(t, glsl, "#define uAmbient 0.2", "ambient emitted as #define override")
}

// ---------------------------------------------------------------------------
// Task 6.3 — Raymarch Settings
// ---------------------------------------------------------------------------

func TestSetting_RaymarchSteps(t *testing.T) {
	glsl := compile(t, "raymarch { steps: 128 }\nsphere")
	assertContains(t, glsl, "#define MAX_STEPS 128", "MAX_STEPS define")
	assertContains(t, glsl, "sdSphere", "sphere still works with raymarch setting")
}

func TestSetting_RaymarchPrecision(t *testing.T) {
	glsl := compile(t, "raymarch { precision: 0.001 }\nsphere")
	assertContains(t, glsl, "#define SURF_DIST 0.001", "SURF_DIST define")
}

func TestSetting_RaymarchMaxDist(t *testing.T) {
	glsl := compile(t, "raymarch { max_dist: 50 }\nsphere")
	assertContains(t, glsl, "#define MAX_DIST 50", "MAX_DIST define")
}

func TestSetting_RaymarchMultiple(t *testing.T) {
	glsl := compile(t, "raymarch { steps: 200, precision: 0.001, max_dist: 100 }\nsphere")
	assertContains(t, glsl, "#define MAX_STEPS 200", "MAX_STEPS define")
	assertContains(t, glsl, "#define SURF_DIST 0.001", "SURF_DIST define")
	assertContains(t, glsl, "#define MAX_DIST 100", "MAX_DIST define")
}

func TestSetting_RaymarchDefinesBeforeSceneSDF(t *testing.T) {
	glsl := compile(t, "raymarch { steps: 128 }\nsphere")
	// #define should come before sceneSDF
	defineIdx := strings.Index(glsl, "#define MAX_STEPS")
	sceneIdx := strings.Index(glsl, "float sceneSDF")
	if defineIdx < 0 || sceneIdx < 0 {
		t.Fatalf("expected both #define and sceneSDF in output\n%s", glsl)
	}
	if defineIdx >= sceneIdx {
		t.Errorf("#define MAX_STEPS should appear before sceneSDF\n%s", glsl)
	}
}

// ---------------------------------------------------------------------------
// Task 6.4 — Post-Processing
// ---------------------------------------------------------------------------

func TestSetting_PostGamma(t *testing.T) {
	glsl := compile(t, "post { gamma: 2.2 }\nsphere")
	assertContains(t, glsl, "sceneSDF", "sceneSDF still emitted")
	assertContains(t, glsl, "sdSphere", "sphere still works with post setting")
	assertContains(t, glsl, "// chisel:post:", "post comment present")
}

func TestSetting_PostDoesNotCrash(t *testing.T) {
	// Multiple post settings should all compile without error.
	glsl := compile(t, "post { gamma: 2.2, vignette: 0.3 }\nsphere")
	assertContains(t, glsl, "sceneSDF", "sceneSDF still emitted")
}

// ---------------------------------------------------------------------------
// Settings don't crash — integration smoke tests
// ---------------------------------------------------------------------------

func TestSetting_AllSettingsCompile(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"light vec", "light [-1, -1, -1]\nsphere"},
		{"camera block", "camera { pos: [0, 2, 5] }\nsphere"},
		{"bg hex", "bg #1a1a2e\nsphere"},
		{"post gamma", "post { gamma: 2.2 }\nsphere"},
		{"raymarch steps", "raymarch { steps: 128 }\nsphere"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			glsl := compile(t, tt.input)
			assertContains(t, glsl, "sceneSDF", "sceneSDF present")
			assertContains(t, glsl, "sdSphere", "sphere present")
		})
	}
}
