# Chisel Compiler — Task Breakdown

Each task is independently testable. Dependencies listed explicitly.
Tasks within the same phase can often be parallelized.

---

## Phase 1: Foundation

### 1.1 Token Types
**Deps:** none
**Files:** `token/token.go`
**Do:**
- Define `TokenKind` enum (all token types from IMPLEMENTATION.md §2)
- Define `Token` struct with `Kind`, `Value`, `Pos`, `Len`
- Define `Position` struct with `File`, `Line`, `Col`, `Offset`
- Define `Span` struct with `Start`, `End Position`
**Test:** Verify constants are distinct, Token/Position constructors work.

### 1.2 Diagnostic Types
**Deps:** 1.1
**Files:** `diagnostic/diagnostic.go`
**Do:**
- Define `Severity` enum (Error, Warning, Hint)
- Define `Diagnostic` struct with Severity, Message, Span, Help, Labels
- Define `Label` struct with Span, Message
- `func (d Diagnostic) Error() string` — implements error interface
**Test:** Construct diagnostics, verify string output.

### 1.3 Diagnostic Renderer
**Deps:** 1.2
**Files:** `diagnostic/render.go`
**Do:**
- `func Render(source string, diag Diagnostic) string` — renders Rust/Elm-style error with:
  - Source line with line number
  - Underline carets pointing to span
  - Error message
  - Help text (if present)
  - Additional labeled spans
- Support ANSI colors (with flag to disable)
**Test:** Golden file tests — input source + diagnostic → expected rendered output.

### 1.4 Lexer — Core
**Deps:** 1.1, 1.2
**Files:** `lexer/lexer.go`
**Do:**
- `func Lex(filename, source string) ([]Token, []Diagnostic)`
- Tokenize: whitespace, single-line comments `//`, multi-line comments `/* */`
- Numbers: int `42`, float `3.14`, scientific `1e-5`
- Identifiers and keywords (lookup table)
- Single-char punctuation: `( ) [ ] { } . , : =`
- Multi-char: `..` `->` `==` `!=` `<=` `>=`
- Hex colors: `#ff0000`, `#f00`, `#ff000080`
- EOF token
**Test:**
- `""` → `[EOF]`
- `"42"` → `[Int(42), EOF]`
- `"3.14"` → `[Float(3.14), EOF]`
- `"sphere"` → `[Ident("sphere"), EOF]`
- `"for"` → `[For, EOF]`
- `"#ff0000"` → `[HexColor("#ff0000"), EOF]`
- `"// comment\nsphere"` → `[Comment, Newline, Ident, EOF]`
- `"/* block */sphere"` → `[Comment, Ident, EOF]`
- Errors: unterminated `/*`, invalid `#xyz`

### 1.5 Lexer — Operators
**Deps:** 1.4
**Files:** `lexer/lexer.go` (extend)
**Do:**
- Arithmetic: `+` `-` `*` `/` `%`
- Boolean SDF: `|` `&`
- Smooth: `|~` `-~` `&~` (two-char tokens)
- Chamfer: `|/` `-/` `&/` (two-char tokens — disambiguate `/` from division)
- Comparison: `<` `>` `==` `!=` `<=` `>=`
- Distinguish `-` (arithmetic) from `-` (subtract) — both are `TokMinus`, parser handles context
**Test:**
- `"a | b"` → `[Ident, Pipe, Ident]`
- `"a |~0.3 b"` → `[Ident, PipeSmooth, Float, Ident]`
- `"a |/ 0.3 b"` → `[Ident, PipeChamfer, Float, Ident]`
- `"a - b"` → `[Ident, Minus, Ident]`
- `"a -~0.2 b"` → `[Ident, MinusSmooth, Float, Ident]`
- `"a * b + c"` → `[Ident, Star, Ident, Plus, Ident]`

### 1.6 Lexer — Newline Insertion
**Deps:** 1.4, 1.5
**Files:** `lexer/lexer.go` (extend)
**Do:**
- After tokens that can end an expression (`Ident`, `)`, `]`, `}`, `Int`, `Float`, `HexColor`, `True`, `False`), insert `TokNewline` if a real newline follows
- Suppress newline after continuation tokens (`|`, `&`, `-`, `+`, `*`, `/`, `%`, `,`, `=`, `(`, `[`, `{`, `|~`, `|/`, `-~`, `-/`, `&~`, `&/`)
- Collapse multiple consecutive newlines into one
**Test:**
- `"sphere\nbox"` → `[Ident, Newline, Ident, EOF]` (two shapes → implicit union)
- `"sphere |\nbox"` → `[Ident, Pipe, Ident, EOF]` (continuation)
- `"sphere\n  .at(1,0,0)"` → `[Ident, Newline, Dot, Ident, ...]` — wait, this breaks method chaining. Need rule: suppress newline before `.`
- `"sphere\n\n\nbox"` → `[Ident, Newline, Ident, EOF]` (collapsed)

### 1.7 AST Types
**Deps:** 1.1
**Files:** `ast/ast.go`
**Do:**
- Define all AST node types from IMPLEMENTATION.md §3
- Every node embeds `Span`
- `Program`, `AssignStmt`, `SettingStmt`
- `NumberLit`, `BoolLit`, `VecLit`, `HexColor`, `Ident`, `StringLit`
- `BinaryExpr` (with `BinaryOp` enum), `UnaryExpr`
- `MethodCall`, `Swizzle`, `FuncCall`, `Arg`
- `Block`, `ForExpr`, `Iterator`, `IfExpr`, `GlslEscape`
- `BinaryOp` enum: Union, SmoothUnion, ChamferUnion, Subtract, SmoothSubtract, ChamferSubtract, Intersect, SmoothIntersect, ChamferIntersect, Add, Sub, Mul, Div, Mod, Eq, Neq, Lt, Gt, Lte, Gte
- `UnaryOp` enum: Neg, Not
- `func Walk(node Node, fn func(Node) bool)` — AST traversal helper
**Test:** Construct AST nodes programmatically, verify Walk visits all nodes.

### 1.8 Parser — Atoms
**Deps:** 1.4, 1.5, 1.7
**Files:** `parser/parser.go`
**Do:**
- `func Parse(tokens []Token) (*ast.Program, []Diagnostic)`
- Parser struct with position, peek, advance, expect helpers
- Parse atoms: number literals, boolean literals, identifiers, hex colors
- Parse vector literals: `[1, 2, 3]`
- Parse parenthesized expressions: `(expr)`
- Error recovery: on unexpected token, skip to next newline/`}`/`)`, record diagnostic
**Test:**
- `"42"` → `NumberLit{42}`
- `"true"` → `BoolLit{true}`
- `"sphere"` → `Ident{"sphere"}`
- `"#ff0000"` → `HexColor{1,0,0,1}`
- `"[1, 2, 3]"` → `VecLit{NumberLit{1}, NumberLit{2}, NumberLit{3}}`
- `"(sphere)"` → `Ident{"sphere"}` (parens stripped)
- `"[1, , 3]"` → error + partial VecLit

### 1.9 Parser — Function Calls & Method Chains
**Deps:** 1.8
**Files:** `parser/parser.go` (extend)
**Do:**
- Parse function calls: `sphere(2)`, `cylinder(1, 3)`, `fbm(p, octaves: 6)`
- Parse named arguments: `name: expr`
- Parse method chains: `sphere.at(2, 0, 0).red.scale(2)`
- Parse bare methods (no parens): `.red`, `.blue` — treated as `.red()` (no-arg method)
- Parse swizzles: `.xyz`, `.xz` — distinguished from methods by checking if all chars are in `xyzrgb`
**Test:**
- `"sphere(2)"` → `FuncCall{"sphere", [NumberLit{2}]}`
- `"sphere(radius: 2)"` → `FuncCall{"sphere", [Arg{"radius", NumberLit{2}}]}`
- `"sphere.at(1, 0, 0)"` → `MethodCall{Ident{"sphere"}, "at", [1,0,0]}`
- `"sphere.red"` → `MethodCall{Ident{"sphere"}, "red", []}`
- `"sphere.at(1,0,0).scale(2).red"` → nested MethodCall chain
- `"p.xz"` → `Swizzle{Ident{"p"}, "xz"}`
- `"v.xyz"` → `Swizzle{...}` vs `"v.scale"` → `MethodCall{...}`

### 1.10 Parser — Pratt Expression Parser
**Deps:** 1.9
**Files:** `parser/pratt.go`
**Do:**
- Implement Pratt parsing for binary operators with precedence:
  - 1: `|` `|~` `|/` (union)
  - 2: `-` `-~` `-/` (subtract — as SDF op, not arithmetic)
  - 3: `&` `&~` `&/` (intersect)
  - 4: `+` `-` (arithmetic — `-` is context-dependent)
  - 5: `*` `/` `%` (arithmetic)
  - 6: `==` `!=` `<` `>` `<=` `>=` (comparison)
- Unary prefix: `-expr`, `!expr`
- Smooth/chamfer ops consume the blend radius as part of the operator: `|~0.3` → BinaryExpr with Blend=0.3
- Handle ambiguity: `-` is SDF subtract when between two shapes, arithmetic minus when in math context. Strategy: parse as highest-precedence match, analyzer resolves type-based.
**Test:**
- `"a | b"` → `BinaryExpr{Union, a, b}`
- `"a | b - c"` → `BinaryExpr{Union, a, BinaryExpr{Subtract, b, c}}`
- `"(a | b) - c"` → `BinaryExpr{Subtract, BinaryExpr{Union, a, b}, c}`
- `"a |~0.3 b"` → `BinaryExpr{SmoothUnion, a, b, Blend: 0.3}`
- `"a * 2 + b"` → `BinaryExpr{Add, BinaryExpr{Mul, a, 2}, b}`
- `"-a"` → `UnaryExpr{Neg, a}`
- `"a | b | c"` → left-associative

### 1.11 Parser — Statements & Assignments
**Deps:** 1.10
**Files:** `parser/parser.go` (extend)
**Do:**
- Parse variable assignment: `name = expr`
- Parse function definition: `name(params) = expr`
- Parse parameters with defaults: `name(a, b = 1, c = 2)`
- Parse newline-separated statements as program body
- Implicit union: consecutive shape expressions separated by newlines become union
- Parse blocks: `{ stmt1; stmt2; expr }` — last expression is result
**Test:**
- `"r = 1.5"` → `AssignStmt{"r", nil, NumberLit{1.5}}`
- `"f(x) = sphere(x)"` → `AssignStmt{"f", [Param{"x"}], FuncCall{...}}`
- `"f(x, y = 1) = ..."` → params with defaults
- `"sphere\nbox"` → `Program` with implicit union of sphere and box
- `"{ sphere\n  box }"` → `Block` with implicit union
- `"a = sphere\na"` → assignment then reference

### 1.12 Parser — Control Flow
**Deps:** 1.11
**Files:** `parser/parser.go` (extend)
**Do:**
- Parse `for i in 0..8 { body }`
- Parse multi-iterator: `for x in -3..3, z in -3..3 { body }`
- Parse `step`: `for i in 0..1 step 0.1 { body }`
- Parse `if cond { body }` and `if cond { body } else { body }`
- Parse `else if` chains
- For and if are expressions (return a value)
**Test:**
- `"for i in 0..8 { sphere.at(i, 0, 0) }"` → `ForExpr{...}`
- `"for x in 0..3, y in 0..3 { ... }"` → two iterators
- `"for i in 0..1 step 0.1 { ... }"` → with step
- `"if x > 1 { sphere } else { box }"` → `IfExpr{...}`
- `"if a { x } else if b { y } else { z }"` → chained

### 1.13 Parser — Settings Blocks
**Deps:** 1.11
**Files:** `parser/settings.go`
**Do:**
- Parse `light { ... }` with nested sun/point/spot blocks and properties
- Parse `camera { pos: ..., target: ..., fov: ... }`
- Parse `camera [0,2,5] -> [0,0,0]` (one-liner)
- Parse `bg expr` (solid) and `bg { linear { ... } }` / `bg { radial { ... } }`
- Parse `raymarch { steps: 128, precision: 0.001 }`
- Parse `post { gamma: 2.2, bloom: { intensity: 0.5, threshold: 0.8 } }`
- Parse `debug normals`
- Parse `mat name = { color: ..., metallic: ... }`
- Settings use `key: value` syntax (colon, not equals)
**Test:**
- `"light [-1, -1, -1]"` → simple directional
- `"camera { pos: [0,2,5], target: [0,0,0] }"` → camera settings
- `"bg #1a1a2e"` → solid background
- `"raymarch { steps: 128 }"` → raymarch settings
- `"debug normals"` → debug mode
- `"mat gold = { color: [1, 0.843, 0], metallic: 1 }"` → material def

### 1.14 Parser — GLSL Escape
**Deps:** 1.8
**Files:** `parser/parser.go` (extend)
**Do:**
- Parse `glsl(p) { ... raw GLSL ... }` blocks
- The GLSL body is NOT parsed as Chisel — it's captured as a raw string
- Track brace nesting to find the closing `}`
- Return `GlslEscape` node with param name and raw code
**Test:**
- `"glsl(p) { return length(p) - 1.0; }"` → `GlslEscape{"p", "return length(p) - 1.0;"}`
- Nested braces: `"glsl(p) { if (p.x > 0.0) { return 1.0; } return 0.0; }"` — correct nesting

---

## Phase 2: Code Generation (Basic)

### 2.1 Codegen — Infrastructure
**Deps:** 1.7
**Files:** `codegen/codegen.go`
**Do:**
- `func Generate(prog *ast.Program) (string, []Diagnostic)`
- Code generator struct with string builder, indent level, temp variable counter
- Helper: `emitSDF(expr) string` — returns GLSL variable name holding the SDF distance
- Helper: `emitColor(expr) string` — returns GLSL variable name holding the color
- Generate wrapper: `float sceneSDF(vec3 p) { ... }` and `vec3 sceneColor(vec3 p) { ... }`
**Test:**
- Empty program → valid GLSL with default sphere

### 2.2 Codegen — Basic Shapes
**Deps:** 2.1
**Files:** `codegen/shapes.go`
**Do:**
- `sphere` → `sdSphere(p, 1.0)`
- `sphere(r)` → `sdSphere(p, r)`
- `box` → `sdBox(p, vec3(1.0))`
- `box(w,h,d)` → `sdBox(p, vec3(w,h,d))`
- `cylinder` → `sdCylinder(p, 0.5, 2.0)` (our GLSL primitive)
- `cylinder(r, h)` → `sdCylinder(p, r, h)`
- `torus` → `sdTorus(p, 1.0, 0.3)`
- `torus(R, r)` → `sdTorus(p, R, r)`
- `plane` → `sdPlane(p, vec3(0,1,0), 0.0)`
- `octahedron` → `sdOctahedron(p, 1.0)`
- `capsule([a],[b],r)` → `sdCapsule(p, a, b, r)`
- Resolve shape names → GLSL function calls with correct argument mapping
**Test:** For each shape, compile and verify GLSL output contains correct function call. Golden file tests.

### 2.3 Codegen — Transforms
**Deps:** 2.2
**Files:** `codegen/transforms.go`
**Do:**
- `.at(x, y, z)` → transform `p` by subtracting offset: `sdf(p - vec3(x,y,z))`
- `.scale(s)` → `sdf(p / s) * s` (uniform)
- `.scale(x, y, z)` → `sdf(p / vec3(x,y,z)) * min(x, min(y, z))` (non-uniform approx)
- `.rot(deg, axis)` → rotate p by -deg around axis before evaluating
- `.orient(axis)` → compute rotation to align Y with target axis
- Each transform wraps the inner SDF by modifying the point variable
- Generate a new local `vec3 pN = transform(p)` for each transform
**Test:**
- `"sphere.at(2, 0, 0)"` → GLSL contains `p - vec3(2.0, 0.0, 0.0)`
- `"sphere.scale(2)"` → GLSL contains `/ 2.0` and `* 2.0`
- `"sphere.rot(45, y)"` → GLSL contains rotation matrix/function
- `"sphere.at(1,0,0).scale(2)"` → transforms compose correctly (translate then scale)

### 2.4 Codegen — Boolean Operations
**Deps:** 2.2
**Files:** `codegen/booleans.go`
**Do:**
- `a | b` → `opUnion(dA, dB)`
- `a - b` → `opSubtract(dA, dB)` (which is `max(dA, -dB)`)
- `a & b` → `opIntersect(dA, dB)` (which is `max(dA, dB)`)
- `a |~0.3 b` → `opSmoothUnion(dA, dB, 0.3)`
- `a -~0.3 b` → `opSmoothSubtract(dA, dB, 0.3)`
- `a &~0.3 b` → `opSmoothIntersect(dA, dB, 0.3)`
- `a |/0.3 b` → `opChamferUnion(dA, dB, 0.3)` (need GLSL impl)
- `a -/0.3 b` → `opChamferSubtract(dA, dB, 0.3)`
- Emit GLSL helper functions for smooth/chamfer ops if used
**Test:**
- `"sphere | box"` → `opUnion(d0, d1)`
- `"sphere - box"` → `opSubtract(d0, d1)`
- `"sphere |~0.3 box"` → `opSmoothUnion(d0, d1, 0.3)`
- `"(sphere | box) - cylinder"` → nested correctly

### 2.5 Codegen — Variables & Functions
**Deps:** 2.4
**Files:** `codegen/codegen.go` (extend)
**Do:**
- Variable assignments → GLSL local variables (floats) or inlined (SDFs)
- SDF variables are inlined as function calls (GLSL can't store SDF as a variable)
- Chisel functions → GLSL functions: `fn_name(vec3 p, float arg1, ...)` returning float
- Handle default parameter values
- Handle blocks with implicit union of shapes
**Test:**
- `"r = 1.5\nsphere(r)"` → `float r = 1.5; ... sdSphere(p, r)`
- `"f(x) = sphere(x)\nf(2)"` → GLSL function `float fn_f(vec3 p, float x) { return sdSphere(p, x); }` called as `fn_f(p, 2.0)`
- `"{ sphere\n  box }"` → `opUnion(sdSphere(p, 1.0), sdBox(p, vec3(1.0)))`

### 2.6 Codegen — For Loops
**Deps:** 2.5
**Files:** `codegen/codegen.go` (extend)
**Do:**
- Unroll `for` loops at compile time (GLSL can't dynamically union SDFs)
- `for i in 0..4 { sphere.at(i, 0, 0) }` → 4 sphere SDFs unioned
- Multi-iterator: `for x in 0..3, z in 0..3 { ... }` → 9 iterations
- Step support: `for i in 0..1 step 0.25 { ... }` → 4 iterations
- Emit `opUnion(opUnion(opUnion(d0, d1), d2), d3)` or chain
- Limit max iterations (e.g. 256) and error if exceeded
**Test:**
- `"for i in 0..3 { sphere.at(i, 0, 0) }"` → 3 spheres unioned with offsets 0, 1, 2
- `"for i in 0..2, j in 0..2 { sphere.at(i, 0, j) }"` → 4 spheres

### 2.7 Codegen — If/Else
**Deps:** 2.5
**Files:** `codegen/codegen.go` (extend)
**Do:**
- `if cond { a } else { b }` for SDF: `cond ? dA : dB`
- `if cond { a } else { b }` for scalar: same ternary
- Nested `else if` chains
- Condition must be a boolean/scalar expression
**Test:**
- `"if t > 1 { sphere } else { box }"` → ternary in GLSL

---

## Phase 3: Materials & Color

### 3.1 Codegen — Basic Color
**Deps:** 2.4
**Files:** `codegen/material.go`
**Do:**
- `.color(r, g, b)` → track color per SDF node
- `.color(#hex)` → parse hex to RGB, track
- Named colors (`.red`, `.blue`, etc.) → map to RGB vectors
- `sceneColor()` function: for each union, compare distances, return closest shape's color
- For smooth unions: interpolate colors using the blend factor
- Default color: white `vec3(1.0)` when no color specified
**Test:**
- `"sphere.red"` → `sceneColor` returns `vec3(1,0,0)`
- `"sphere.red | box.blue"` → color switches based on closest SDF
- `"sphere.color(0.5, 0.5, 0.5)"` → `vec3(0.5)`
- `"sphere.color(#ff8800)"` → correct RGB conversion

### 3.2 Codegen — Material Properties
**Deps:** 3.1
**Files:** `codegen/material.go` (extend)
**Do:**
- `.metallic(v)`, `.roughness(v)`, `.emission(...)`, `.opacity(v)` → track per shape
- Generate material struct passing in sceneColor or separate material functions
- `mat name = { ... }` definitions → reusable material constants
- `.mat(name)` → apply named material
**Test:**
- `"sphere.metallic(0.9).roughness(0.1)"` → material properties in output
- `"mat gold = { color: [1,0.843,0], metallic: 1 }\nsphere.mat(gold)"` → uses gold material

### 3.3 Codegen — Procedural Color
**Deps:** 3.1
**Files:** `codegen/material.go` (extend)
**Do:**
- `.color(expr)` where expr uses `p` → position-dependent color
- The color expression is emitted inline in `sceneColor()` with `p` as the evaluation point
- Support: `mix(a, b, noise(p))`, `smoothstep(...)`, math on `p.y`, etc.
**Test:**
- `"sphere.color(p.y * 0.5 + 0.5, 0.3, 0.1)"` → uses `p.y` in sceneColor
- `"sphere.color(mix(blue, red, p.y))"` → mix with position

---

## Phase 4: Advanced Transforms

### 4.1 Codegen — Mirror
**Deps:** 2.3
**Files:** `codegen/transforms.go` (extend)
**Do:**
- `.mirror(x)` → `p.x = abs(p.x)` before SDF evaluation
- `.mirror(x, z)` → `p.x = abs(p.x); p.z = abs(p.z)`
- `.mirror(x, origin: 1)` → `p.x = abs(p.x - 1.0) + 1.0`
**Test:**
- `"sphere.at(2,0,0).mirror(x)"` → GLSL uses `abs(p.x)`
- `"sphere.at(1,0,1).mirror(x, z)"` → both axes folded

### 4.2 Codegen — Repetition
**Deps:** 2.3
**Files:** `codegen/transforms.go` (extend)
**Do:**
- `.rep(spacing)` → `p = mod(p + 0.5*s, s) - 0.5*s` (infinite repeat)
- `.rep(sx, sy, sz)` → per-axis spacing
- `.rep(x: 2)` → repeat only along X
- `.rep(spacing, count: n)` → clamp repetition: `p = p - s * clamp(round(p/s), -n, n)`
- `.array(n, radius: r)` → circular array using angle repetition
**Test:**
- `"sphere(0.3).rep(2)"` → mod-based repetition in GLSL
- `"sphere(0.3).rep(2, count: 5)"` → clamped repetition

### 4.3 Codegen — Morph, Shell, Onion, Displace
**Deps:** 2.2
**Files:** `codegen/transforms.go` (extend)
**Do:**
- `.morph(other, t)` → `mix(sdfA(p), sdfB(p), t)`
- `.shell(thickness)` → `abs(sdf(p)) - thickness`
- `.onion(thickness)` → same as shell but commonly layered
- `.displace(expr)` → `sdf(p) + expr` where expr can use `p`
- `.dilate(r)` → `sdf(p) - r`
- `.erode(r)` → `sdf(p) + r`
- `.round(r)` → `sdf(p) - r`
- `.elongate(x,y,z)` → elongation transform on p
- `.twist(strength)` → twist transform on p
- `.bend(strength)` → bend transform on p
**Test:**
- `"sphere.morph(box, 0.5)"` → `mix(sdSphere(...), sdBox(...), 0.5)`
- `"sphere.shell(0.05)"` → `abs(...) - 0.05`
- `"sphere.displace(sin(p.x * 10) * 0.1)"` → `... + sin(...) * 0.1`

---

## Phase 5: Signals & Noise

### 5.1 Codegen — Time & Built-in Signals
**Deps:** 2.1
**Files:** `codegen/signals.go`
**Do:**
- `t` → `uTime` uniform
- `sin(t)`, `cos(t)` etc. → direct GLSL math
- `pulse(t, duty)` → GLSL step function implementation
- `saw(t)` → `fract(t)`
- `tri(t)` → `abs(fract(t) - 0.5) * 2.0`
- Ensure `uTime` uniform is declared in output
**Test:**
- `"sphere.at(0, sin(t), 0)"` → `sin(uTime)` in GLSL

### 5.2 Codegen — Noise Functions
**Deps:** 2.1
**Files:** `codegen/noise.go`
**Do:**
- `noise(p)` → emit simplex/value noise GLSL function, call it
- `fbm(p, octaves: n)` → emit FBM GLSL function with n octave loops
- `voronoi(p)` → emit Voronoi GLSL function
- Only emit the GLSL noise functions if they're actually used (dead code elimination)
**Test:**
- `"sphere.displace(noise(p * 5) * 0.1)"` → noise function emitted and called
- `"plane.displace(fbm(p.xz, octaves: 4))"` → fbm function emitted

### 5.3 Codegen — Easing Functions
**Deps:** 2.1
**Files:** `codegen/easing.go`
**Do:**
- Emit GLSL implementations for all easing functions when used
- `ease_in(x)` → `x * x`
- `ease_out(x)` → `1.0 - (1.0 - x) * (1.0 - x)`
- `ease_in_out(x)` → standard smoothstep-like
- `ease_cubic_*`, `ease_elastic`, `ease_bounce`, `ease_back`, `ease_expo`
- `remap(v, a, b, c, d)` → linear remap GLSL
**Test:** Each easing function compiles to valid GLSL.

---

## Phase 6: Settings & Scene

### 6.1 Codegen — Camera & Background
**Deps:** 2.1, 1.13
**Files:** `codegen/scene.go`
**Do:**
- `camera { pos: ..., target: ..., fov: ... }` → emit uniform overrides or inject into main()
- `bg #color` → modify the background color in the fragment shader's miss branch
- `bg { linear { ... } }` → gradient computation based on screen UV
- `bg { radial { ... } }` → radial gradient
**Test:**
- `"camera { pos: [0,2,5] }\nsphere"` → camera position in output
- `"bg #1a1a2e\nsphere"` → background color in miss branch

### 6.2 Codegen — Lighting
**Deps:** 2.1, 1.13
**Files:** `codegen/lighting.go`
**Do:**
- `light [-1,-1,-1]` → set directional light uniform
- `light { sun { ... } }` → directional light with properties
- `light { point { ... } }` → point light evaluation in shade function
- Multiple lights → loop/sum in shading
- `ambient`, `ao`, `fog` properties → modify shading pipeline
**Test:**
- `"light [-1,-1,-1]\nsphere"` → light direction in output
- `"light { ambient: 0.2 }\nsphere"` → ambient value

### 6.3 Codegen — Raymarch Settings
**Deps:** 2.1, 1.13
**Files:** `codegen/scene.go` (extend)
**Do:**
- `raymarch { steps: N }` → override MAX_STEPS constant
- `precision` → override SURF_DIST
- `max_dist` → override MAX_DIST
**Test:**
- `"raymarch { steps: 128 }\nsphere"` → `MAX_STEPS = 128` in output

### 6.4 Codegen — Post-Processing
**Deps:** 2.1, 1.13
**Files:** `codegen/post.go`
**Do:**
- Post-processing applies after `shade()` in the main function
- `gamma` → `pow(col, vec3(1.0/gamma))`
- `contrast` → `(col - 0.5) * contrast + 0.5`
- `saturation` → desaturate/saturate via luminance mix
- `vignette` → darken based on distance from screen center
- `bloom` → threshold + add (simplified single-pass)
- `grain` → random noise based on UV + time
- `chromatic` → offset R and B channels by UV
- `tonemap: aces` → ACES filmic tone mapping function
**Test:**
- `"post { gamma: 2.2 }\nsphere"` → gamma correction in output
- `"post { vignette: 0.3 }\nsphere"` → vignette calculation

---

## Phase 7: Semantic Analysis

### 7.1 Type System
**Deps:** 1.7
**Files:** `analyzer/types.go`
**Do:**
- Define types: `Float`, `Vec2`, `Vec3`, `Bool`, `SDF2D`, `SDF3D`, `Material`, `Color`, `Void`
- Type inference rules for each AST node
- Type compatibility checks for operators
- Method return types (`.at()` on SDF3D → SDF3D, `.extrude()` on SDF2D → SDF3D)
**Test:** Type inference for all basic expressions.

### 7.2 Name Resolution & Scope
**Deps:** 7.1
**Files:** `analyzer/scope.go`
**Do:**
- Build scope chain: builtins → top-level → block → for-loop
- Resolve all identifiers to their definitions
- Report "undefined variable" with fuzzy suggestions
- Report "variable defined but unused" as warning
- Check function arity (too many/few arguments)
**Test:**
- Undefined variable → error with suggestion
- Out-of-scope variable → error
- Shadowing → no error
- Wrong arity → error with expected count

### 7.3 Validation
**Deps:** 7.2
**Files:** `analyzer/analyzer.go`
**Do:**
- Scene root must be SDF3D (not SDF2D, not float)
- Boolean ops: both sides must be same SDF type
- 2D shape in scene without `.extrude()` → error with help
- `.extrude()` on 3D shape → error
- Recursive function → error
- For loop iteration count > 256 → error
- Common mistakes detection (§4 of IMPLEMENTATION.md)
**Test:** Golden file error tests for each validation rule.

---

## Phase 8: Tooling

### 8.1 Formatter
**Deps:** 1.7
**Files:** `format/format.go`
**Do:**
- `func Format(prog *ast.Program) string`
- Canonical formatting rules (IMPLEMENTATION.md §9)
- Preserves comments (comments attached to nearest AST node)
- Idempotent
**Test:** Format → format again → same output. Golden file tests.

### 8.2 Public API
**Deps:** all above
**Files:** `chisel.go`
**Do:**
- `func Compile(source string) (glsl string, diagnostics []Diagnostic)`
- `func Parse(source string) (*ast.Program, []Diagnostic)`
- `func Format(source string) (string, error)`
- `func Check(source string) []Diagnostic`
- Wire together lexer → parser → analyzer → codegen
**Test:** End-to-end: `.chisel` source → valid GLSL that compiles on GPU.

### 8.3 Integration with asciishader
**Deps:** 8.2
**Files:** `cmd/asciishader/main.go`, `tui/editor/editor.go`
**Do:**
- Detect `.chisel` files in editor (by extension or content)
- On Ctrl+R: compile Chisel → GLSL → GPU compile
- Show Chisel diagnostics in editor status bar
- Support both `.chisel` and `.glsl` files
**Test:** Load a `.chisel` file, compile, render — no crash, correct output.

---

## Dependency Graph (what can parallelize)

```
1.1 ─→ 1.2 ─→ 1.3
 │       │
 ├→ 1.4 ─→ 1.5 ─→ 1.6
 │                   │
 ├→ 1.7 ─→ 1.8 ─→ 1.9 ─→ 1.10 ─→ 1.11 ─→ 1.12
 │                                    │       │
 │                                    ├→ 1.13 ─→ 1.14
 │                                    │
 │                          2.1 ─→ 2.2 ─→ 2.3
 │                           │       │     │
 │                           │    2.4 ─→ 2.5 ─→ 2.6 ─→ 2.7
 │                           │               │
 │                           │           3.1 ─→ 3.2 ─→ 3.3
 │                           │
 │                        4.1, 4.2, 4.3 (parallel after 2.3)
 │
 │                        5.1, 5.2, 5.3 (parallel after 2.1)
 │
 │                        6.1, 6.2, 6.3, 6.4 (parallel after 1.13 + 2.1)
 │
 │                        7.1 ─→ 7.2 ─→ 7.3 (after 1.7)
 │
 │                        8.1 (after 1.7)
 │                        8.2 (after all above)
 │                        8.3 (after 8.2)
```

**Max parallelism opportunities:**
- After 1.1: tasks 1.2, 1.4, 1.7 can start in parallel
- After 2.2: tasks 2.3, 2.4, 4.1, 4.2, 4.3 can start in parallel
- After 2.1: tasks 5.1, 5.2, 5.3 can start in parallel
- After 1.13 + 2.1: tasks 6.1, 6.2, 6.3, 6.4 can start in parallel
- Phase 7 (analyzer) is independent of codegen and can start after 1.7
- Phase 8.1 (formatter) is independent of codegen
