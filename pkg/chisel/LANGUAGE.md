# Chisel Language Reference

**Sculpt 3D worlds with code.**

Chisel compiles to GLSL. Everything you can do in a fragment shader, you can do in Chisel — but in fewer lines.

---

## 1. Hello World

```chisel
sphere
```

That's a complete program. It renders a unit sphere at the origin with default lighting.

---

## 2. Comments

```chisel
// Single line comment

/* Multi-line
   comment */

sphere // inline comment
```

---

## 3. Shapes

All shapes have smart defaults. Parentheses optional when using defaults.

### 3D Primitives

```chisel
sphere              // radius 1
sphere(2)           // radius 2

box                 // 1×1×1 cube
box(2, 1, 3)        // width, height, depth

cylinder            // radius 0.5, height 2
cylinder(1, 3)      // radius, height

torus               // major 1, minor 0.3
torus(2, 0.5)       // major, minor

capsule             // default endpoints, radius 0.25
capsule([0,-1,0], [0,1,0], 0.5)

cone(1, 0.5, 3)     // bottom radius, top radius, height

plane               // infinite ground plane at y=0

octahedron          // size 1
pyramid             // height 1
ellipsoid(2, 1, 3)  // radii per axis

rounded_box(2, 0.1)         // size, edge radius
wireframe_box(2, 0.05)      // size, thickness
rounded_cylinder(1, 3, 0.1) // radius, height, rounding
```

### 2D Primitives

2D shapes exist in the XY plane. They must be extruded or revolved to render.

```chisel
circle              // radius 1
circle(2)           // radius 2

rect                // 1×1 square
rect(2, 1)          // width, height

hexagon             // radius 1
polygon([[0,0], [1,0], [0.5,1]])

// 2D → 3D
circle(2).extrude(3)                    // cylinder
rect(2, 1).extrude(1)                   // box
circle(2).extrude_to(circle(0.5), 3)    // tapered
hexagon.revolve(3)                      // donut from hexagon
```

---

## 4. Transforms

Method chaining. Order matters (applied left to right).

```chisel
sphere.at(2, 0, 0)             // translate
sphere.scale(2)                // uniform scale
sphere.scale(2, 1, 0.5)       // non-uniform
sphere.rot(45, y)              // rotate 45° around Y

// Short aliases
sphere.at(x: 2)               // translate only X
sphere.at(y: -1)              // translate only Y

// Axis constants: x, y, z
cylinder.orient(x)            // align cylinder along X axis
cylinder.orient([1, 1, 0])    // align along arbitrary direction
```

### Advanced Transforms

```chisel
box.twist(90)                  // twist around Y axis
box.bend(0.3)                  // bend
sphere.elongate(1, 0, 0)      // stretch along X
sphere.round(0.1)             // round edges
sphere.shell(0.05)            // hollow out (wall thickness)
sphere.onion(0.1)             // concentric shells
sphere.dilate(0.1)            // expand outward
sphere.erode(0.1)             // shrink inward
```

### Mirror & Symmetry

Mirror folds space — O(1) cost, like `.rep()`.

```chisel
sphere.at(2, 0, 0).mirror(x)          // two spheres at x=±2
sphere.at(2, 1, 0).mirror(x, y)       // four copies
box.at(1, 0, 1).mirror(x, z)          // four copies in XZ

// Mirror with offset (fold axis at x=1 instead of x=0)
sphere.at(3, 0, 0).mirror(x, origin: 1)
```

### Morph

Blend between two shapes by interpolating their distance fields:

```chisel
sphere.morph(box, 0.5)                 // halfway between sphere and box
sphere.morph(box, sin(t) * 0.5 + 0.5) // animated morph
sphere.morph(octahedron, 0.3)          // mostly sphere, hint of octahedron
```

### Displacement

Distort a surface using a scalar expression. `p` is the evaluation point:

```chisel
sphere.displace(noise(p * 5) * 0.1)           // noisy surface
sphere.displace(sin(p.x * 10) * 0.05)         // ripples
plane.displace(fbm(p.xz * 0.5) * 2)           // terrain
```

> **Note:** `+` is **not** used for displacement or union. Use `|` for union and `.displace()` for surface perturbation. `+` is reserved for arithmetic.

### Repetition (Space Folding)

`.rep()` and `.array()` fold space — the GPU evaluates one shape and gets repetition for free. **Use these for identical copies.**

```chisel
sphere(0.3).rep(2)             // infinite repeat, spacing 2
sphere(0.3).rep(2, 2, 2)      // repeat in all axes
sphere(0.3).rep(x: 2)         // repeat only along X
sphere(0.3).rep(2, count: 5)  // limited repetitions

// Circular array
pillar.array(8, radius: 4)    // 8 copies in a circle
pillar.array(8, radius: 4, axis: y)
```

> **`.rep()` vs `for`**: `.rep()` is O(1) on the GPU — one SDF evaluation regardless of count. `for` loops unroll into N separate shapes, costing O(N). Use `.rep()`/`.array()` for identical copies, `for` when each copy needs to be different.

---

## 5. Boolean Operations

```chisel
// Sharp (default)
a | b                  // union
a - b                  // subtract
a & b                  // intersect

// Smooth — ~ after operator, then blend radius
a |~0.3 b              // smooth union
a -~0.2 b              // smooth subtract
a &~0.1 b              // smooth intersect

// Chamfer — / after operator
a |/0.3 b              // chamfer union
a -/0.2 b              // chamfer subtract
```

### Precedence (tightest → loosest)

```
&    intersect
-    subtract
|    union
```

```chisel
sphere | box - cylinder
// parses as: sphere | (box - cylinder)

// Override with parens
(sphere | box) - cylinder
```

---

## 6. Variables & Functions

```chisel
// Variables — just assignment
r = 1.5
base = sphere(r)

// Functions — assignment with parameters
pillar(h) = cylinder(0.3, h) | sphere(0.4).at(0, h/2, 0)

// Default parameters
pillar(h = 3, r = 0.3) = cylinder(r, h) | sphere(r * 1.3).at(0, h/2, 0)

// Multi-line with block (implicit union)
pillar(h) = {
  cylinder(0.3, h)
  sphere(0.4).at(0, h/2, 0)
  sphere(0.4).at(0, -h/2, 0)
}

// Usage
pillar(3) | pillar(5).at(4, 0, 0)
```

---

## 7. Blocks & Implicit Union

Multiple shape expressions in a block are implicitly unioned.

```chisel
// These are equivalent:
{
  sphere
  box.at(2, 0, 0)
}

sphere | box.at(2, 0, 0)
```

If a block contains assignments, the last expression is the result:

```chisel
{
  base = sphere(2)
  holes = cylinder(0.5, 6).orient(x) | cylinder(0.5, 6).orient(z)
  base - holes
}
```

Top-level program follows the same rule.

---

## 8. Control Flow

### For Loops

Loops are expressions. They return the union of all iterations.

```chisel
// Circle of spheres
for i in 0..8 {
  sphere(0.3).at(cos(i * 45) * 2, 0, sin(i * 45) * 2)
}

// Grid
for x in -3..3, z in -3..3 {
  sphere(0.2).at(x, 0, z)
}

// With step
for i in 0..1 step 0.1 {
  sphere(0.1).at(i * 5, sin(i * PI * 2), 0)
}
```

### Conditionals

```chisel
// Conditional expression
if r > 1 { sphere(r) } else { box(r) }

// In functions
shape(kind, s) = if kind == 0 { sphere(s) } else { box(s) }
```

---

## 9. Signals & Animation

### Time

`t` is always available — seconds since start.

```chisel
sphere.at(0, sin(t), 0)                   // bobbing
sphere.scale(1 + sin(t) * 0.2)           // breathing
sphere.rot(t * 30, y)                     // spinning
```

### Oscillators

Built-in periodic functions (all take time and return -1..1 or 0..1):

```chisel
sin(t)                  // sine wave, -1..1
cos(t)                  // cosine wave, -1..1
pulse(t, duty: 0.5)    // square wave, 0..1
saw(t)                  // sawtooth, 0..1
tri(t)                  // triangle wave, 0..1
```

### Easing Functions

Transform a 0..1 value with an easing curve:

```chisel
ease_in(x)              // quadratic ease in
ease_out(x)             // quadratic ease out
ease_in_out(x)          // quadratic ease in-out

ease_cubic_in(x)
ease_cubic_out(x)
ease_cubic_in_out(x)

ease_elastic(x)
ease_bounce(x)
ease_back(x)
ease_expo(x)
```

### Mapping & Clamping

```chisel
remap(v, 0, 1, -2, 2)   // remap range
clamp(v, 0, 1)           // clamp to range
saturate(v)              // clamp to 0..1
smoothstep(0, 1, v)      // smooth interpolation
mix(a, b, v)             // linear interpolation
step(edge, v)            // hard threshold
```

### Springs

```chisel
spring(target, stiffness: 0.1, damping: 0.8)
```

### Keyframes

```chisel
kf = keyframes(t) {
  0:    0
  1:    3   ease: ease_out
  1.2:  0   ease: ease_in
  2:    0
}

sphere.at(y: kf)
```

---

## 10. Vectors & Swizzling

Vector literals and GLSL-style component access:

```chisel
v = [1, 2, 3]           // vec3
v.x                     // 1
v.xy                    // [1, 2]
v.xz                    // [1, 3]
v.yx                    // [2, 1]
v.xxx                   // [1, 1, 1]

// Works on p (evaluation point)
p.xz                    // horizontal plane position
p.y                     // height
length(p.xz)            // distance from Y axis
```

---

## 11. Noise & Procedural

All noise functions work in 2D or 3D, return -1..1 or 0..1.

```chisel
// Basic noise
noise(p)                     // Perlin/simplex noise at point p
noise(p * 5)                 // higher frequency
noise(p * 5 + t)             // animated noise

// Fractal noise
fbm(p, octaves: 6)           // fractal Brownian motion
fbm(p, octaves: 4, gain: 0.5, lacunarity: 2.0)

// Voronoi
voronoi(p)                   // returns distance to nearest cell
voronoi(p).cell              // cell ID
voronoi(p).edge              // distance to cell edge

// Domain warping
warp(p, noise(p) * 0.5)     // warp space with noise
```

### Using Noise in Shapes

The special variable `p` refers to the current evaluation point:

```chisel
// Noisy sphere — displaces the surface
sphere.displace(noise(p * 5) * 0.1)

// Terrain
plane.displace(fbm(p.xz * 0.5) * 2)

// Animated displacement
sphere.displace(sin(p.x * 8 + t * 3) * sin(p.z * 8) * 0.05)
```

---

## 12. Materials & Color

### Basic Color

```chisel
sphere.color(1, 0, 0)                    // RGB 0..1
sphere.color(#ff0000)                     // hex
sphere.color(#f00)                        // hex short
sphere.color(rgb(255, 0, 0))             // RGB 0..255
sphere.color(hsl(0, 100, 50))            // HSL

// Named colors
sphere.red
sphere.blue
sphere.white
sphere.orange
```

### Material Properties

```chisel
sphere
  .color(0.8, 0.6, 0.2)
  .metallic(0.9)
  .roughness(0.1)
  .emission(1, 0.5, 0)       // emissive color
  .emission(2.0)              // emissive intensity (white)
  .opacity(0.5)               // transparency
```

### Material Definitions

```chisel
mat gold = {
  color: [1, 0.843, 0]
  metallic: 1
  roughness: 0.3
}

mat glass = {
  color: [0.95, 0.95, 1]
  roughness: 0
  opacity: 0.1
  ior: 1.5
}

sphere.mat(gold) | box.mat(glass).at(2, 0, 0)
```

### Procedural Color

Use `p` (evaluation point) for position-dependent materials:

```chisel
// Checkerboard
sphere.color(
  mix([1,1,1], [0,0,0], step(0, sin(p.x * 10) * sin(p.z * 10)))
)

// Noise-driven color
sphere.color(
  mix(blue, orange, noise(p * 3) * 0.5 + 0.5)
)

// Height-based gradient
terrain.color(
  mix(green, white, smoothstep(0, 2, p.y))
)
```

### Per-Shape Color in Booleans

Each shape carries its own material. The closest shape determines the material at any point:

```chisel
sphere.red | box.blue.at(2, 0, 0)
// sphere area renders red, box area renders blue
// smooth blends interpolate colors
```

---

## 13. Lighting

Top-level `light` block. Multiple lights supported.

```chisel
// Simple — just a direction
light [−1, −1, −1]

// Full control
light {
  sun {
    dir: [-1, -1, -1]
    color: #fff5e0
    intensity: 0.8
    shadows: true
  }

  point {
    pos: [2, 3, 1]
    color: orange
    intensity: 10
    radius: 5
  }

  ambient: 0.1
  ao: 0.3                    // ambient occlusion intensity
  fog: { color: #1a1a2e, near: 10, far: 50 }
}
```

### Animated Lighting

```chisel
light {
  sun {
    dir: [sin(t), -1, cos(t)]
    color: mix(#fff5e0, #ff6430, sin(t) * 0.5 + 0.5)
  }

  point {
    pos: [sin(t * 2) * 3, 2, cos(t * 2) * 3]
    color: hsl(t * 50, 100, 60)
    intensity: 5
  }
}
```

---

## 14. Camera & Scene Settings

```chisel
camera {
  pos: [0, 2, 5]
  target: [0, 0, 0]
  fov: 60
}

// Or one-liner
camera [0, 2, 5] -> [0, 0, 0]
```

### Background

```chisel
// Solid
bg #1a1a2e

// Gradient
bg {
  linear {
    dir: [0, 1]
    stops: { 0: black, 0.5: #1a1a2e, 1: #0a0a1e }
  }
}

// Radial gradient
bg {
  radial {
    center: [0.5, 0.3]
    radius: 1.5
    stops: { 0: #2a1a3e, 1: black }
  }
}
```

### Raymarching

```chisel
raymarch {
  steps: 128          // max iterations (default 64)
  precision: 0.001    // hit threshold (default 0.005)
  max_dist: 50        // max ray distance (default 50)
}
```

---

## 15. Post-Processing

Screen-space effects applied after rendering. Order matters.

```chisel
post {
  // Color grading
  gamma: 2.2
  contrast: 1.1
  saturation: 1.2
  brightness: 0.05
  tint: #ffe0c0          // warm tint

  // Tone mapping
  tonemap: aces           // none, reinhard, aces, filmic

  // Effects
  vignette: 0.3           // darken edges
  bloom: { intensity: 0.5, threshold: 0.8 }
  grain: 0.02             // film grain
  chromatic: 0.003        // chromatic aberration

  // Screen distortion
  barrel: 0.1             // barrel distortion
  scanlines: { intensity: 0.1, count: 300 }
}
```

### Animated Post-Processing

```chisel
post {
  saturation: 1 + sin(t) * 0.3
  chromatic: 0.001 + pulse(t * 2) * 0.01
}
```

---

## 16. GLSL Escape Hatch

For when Chisel's built-ins aren't enough — inline raw GLSL:

```chisel
// Custom SDF as a GLSL block
my_shape = glsl(p) {
  float d = length(p) - 1.0;
  d += sin(p.x * 10.0) * sin(p.y * 10.0) * sin(p.z * 10.0) * 0.03;
  return d;
}

// Use like any other shape
my_shape.rot(t * 10, y).red

// Custom color function
sphere.color(glsl(p) {
  return vec3(sin(p.x * 5.0) * 0.5 + 0.5, 0.3, 0.7);
})
```

The `glsl` block receives `p` (vec3 evaluation point) and must return a `float` (for SDFs) or `vec3` (for colors). All Chisel uniforms (`uTime` as `t`, etc.) are available inside.

---

## 17. Debug Modes

Visualize the internals of your scene:

```chisel
debug normals              // surface normals as RGB
debug steps                // raymarching step count (heat map)
debug distance             // distance field visualization
debug ao                   // ambient occlusion only
debug uv                   // UV coordinates
debug depth                // depth buffer
```

Debug mode replaces the normal shading pipeline. Only one can be active.

---

## 18. Scope & Parameters

### Scope

Variables are block-scoped. Inner blocks can read outer variables and shadow them:

```chisel
r = 2
{
  r = 1               // shadows outer r
  sphere(r)            // uses r = 1
}
box(r)                 // uses r = 2
```

### Parameter Conventions

Positional arguments first, named arguments for optional/clarity:

```chisel
// Positional: most common args
sphere(2)
cylinder(1, 3)
capsule([0,-1,0], [0,1,0], 0.5)

// Named: for clarity or optional args
sphere(radius: 2)
cylinder(radius: 1, height: 3)
fbm(p, octaves: 6, gain: 0.5)

// Mixed: positional then named
rep(2, count: 5)
array(8, radius: 4, axis: y)
```

---

## 19. Built-in Math

```chisel
// Trig
sin(x)  cos(x)  tan(x)
asin(x) acos(x) atan(x) atan2(y, x)

// Power
pow(x, n)  sqrt(x)  exp(x)  log(x)

// Rounding
floor(x)  ceil(x)  round(x)  fract(x)

// Comparison
min(a, b)  max(a, b)  abs(x)  sign(x)

// Interpolation
mix(a, b, t)  smoothstep(a, b, t)  step(edge, x)
clamp(x, lo, hi)  saturate(x)  remap(x, a, b, c, d)

// Vector
length(v)  normalize(v)  dot(a, b)  cross(a, b)
distance(a, b)  reflect(v, n)

// Constants
PI  TAU  E
```

---

## 20. Complete Examples

### Carved Sphere

```chisel
sphere(2) - cylinder(0.5, 6).orient(x) - cylinder(0.5, 6).orient(z)
```

### Temple

```chisel
pillar(h = 3) = {
  cylinder(0.3, h)
  sphere(0.4).at(0, h/2, 0)
  box(0.6, 0.15, 0.6).at(0, -h/2, 0)
}

floor = box(8, 0.3, 5).at(0, -1.65, 0).color(0.8, 0.8, 0.75)
pillars = for i in 0..4 {
  pillar().at(-3 + i * 2, 0, -2) | pillar().at(-3 + i * 2, 0, 2)
}
roof = box(7, 0.3, 6).at(0, 1.65, 0).color(0.9, 0.85, 0.8)

floor | pillars.color(0.9, 0.88, 0.82) | roof
```

### Lava Lamp

```chisel
raymarch { steps: 128 }

blob(cx, cy, cz, phase) =
  sphere(0.6 + sin(t * 0.7 + phase) * 0.2)
    .at(cx + sin(t * 0.3 + phase) * 0.5, cy + sin(t * 0.5 + phase) * 1.5, cz)

blobs = {
  blob(0, 0, 0, 0)
  blob(0.5, 1, 0, 1)
  blob(-0.3, -1, 0, 2.5)
  blob(0.2, 0.5, 0, 4)
}

glass = cylinder(1, 4).shell(0.05).opacity(0.1)

blobs.color(mix(#ff4400, #ffaa00, p.y * 0.3 + 0.5)) |~0.5 glass

light {
  sun { dir: [-1, -1, -1], intensity: 0.6 }
  point { pos: [0, 0, 2], color: #ff6600, intensity: 3 }
  ambient: 0.15
}

bg #0a0808
```

### Animated Gear

```chisel
gear(teeth = 12, r = 2, thickness = 0.3) = {
  body = cylinder(r, thickness) - cylinder(r * 0.3, thickness * 2)
  tooth = box(r * 0.3, thickness, r * 0.15).at(r, 0, 0)
  body | for i in 0..teeth {
    tooth.rot(i * 360 / teeth, y)
  }
}

gear(12).rot(t * 30, y).metallic(0.9).roughness(0.3).color(#888)
| gear(8, 1.2).rot(-t * 45 + 15, y).at(3.1, 0, 0).color(#aaa).metallic(0.9)
```

### Alien Landscape

```chisel
raymarch { steps: 200, precision: 0.001 }

terrain = plane.displace(
  fbm(p.xz * 0.3 + t * 0.05, octaves: 5) * 3
  - smoothstep(5, 0, length(p.xz)) * 2
)

spire(h, r) = cone(r, 0.01, h).round(0.05)

spires = for i in 0..20 {
  px = noise([i, 0]) * 10
  pz = noise([0, i]) * 10
  h = 2 + noise([i, i]) * 3
  spire(h, 0.3).at(px, 0, pz)
}

terrain.color(mix(#2a1a0a, #4a3a1a, saturate(p.y * 0.5)))
| spires.color(mix(#3a0a2a, #8a2a4a, p.y * 0.2))

light {
  sun { dir: [-0.5, -1, -0.3], color: #ff8844, intensity: 0.7 }
  ambient: rgb(20, 10, 30)
  fog: { color: #1a0a2a, near: 5, far: 30 }
}

bg { radial { center: [0.5, 0.2], radius: 2, stops: { 0: #2a1040, 1: #050010 } } }

post {
  tonemap: aces
  bloom: { intensity: 0.3, threshold: 0.7 }
  vignette: 0.4
}
```

### Interactive Morph

```chisel
// Smooth transition between shapes over time
phase = fract(t * 0.25)

a = sphere
b = box.rot(45, y)
c = octahedron(1.2)

shape = if phase < 0.33 {
  a |~mix(0, 0.5, phase * 3) b
} else if phase < 0.66 {
  b |~mix(0, 0.5, (phase - 0.33) * 3) c
} else {
  c |~mix(0, 0.5, (phase - 0.66) * 3) a
}

shape
  .rot(t * 15, y)
  .rot(sin(t * 0.7) * 20, x)
  .color(hsl(t * 20, 80, 60))
  .metallic(0.7)
  .roughness(0.2)

post { bloom: { intensity: 0.4, threshold: 0.6 } }
```

---

## 21. Syntax Summary

```
PROGRAM     = (SETTING | ASSIGN | EXPR)*

SETTING     = light BLOCK | camera BLOCK | bg EXPR | raymarch BLOCK
            | post BLOCK | debug NAME | mat NAME = BLOCK

ASSIGN      = NAME PARAMS? = EXPR
PARAMS      = ( NAME (= EXPR)? (, NAME (= EXPR)?)* )

EXPR        = BOOL_EXPR
BOOL_EXPR   = UNARY ((BOOL_OP UNARY)*)
BOOL_OP     = |  |~NUM  |/NUM  -  -~NUM  -/NUM  &  &~NUM  &/NUM

UNARY       = ATOM (. METHOD | . SWIZZLE)*
METHOD      = NAME ( ARGS? )
SWIZZLE     = [xyzrgb]{1,4}

ATOM        = NAME | NUM | VEC | HEX | STRING
            | NAME ( ARGS )           // function call
            | ( EXPR )                // grouped expression
            | BLOCK                   // block expression
            | FOR | IF                // control flow
            | glsl ( NAME ) GLSL_BLOCK  // escape hatch

BLOCK       = { (ASSIGN | EXPR)* }
VEC         = [ EXPR (, EXPR)* ]
FOR         = for NAME in RANGE (, NAME in RANGE)* BLOCK
IF          = if EXPR BLOCK (else (IF | BLOCK))?
RANGE       = EXPR .. EXPR (step EXPR)?

ARGS        = ARG (, ARG)*
ARG         = (NAME :)? EXPR          // positional or named
```

---

## 22. Design Principles

1. **A program is a shape.** No ceremony, no boilerplate.
2. **Defaults are beautiful.** `sphere` renders something interesting with zero configuration.
3. **Progressive disclosure.** Simple → variables → functions → animation → materials → post.
4. **Operators are visual.** `|` union, `-` subtract, `&` intersect, `~` smooth, `/` chamfer.
5. **Everything is an expression.** Blocks, loops, conditionals all return shapes.
6. **Position is implicit.** `p` is always available where it makes sense (materials, noise, displacement).
7. **Time is `t`.** One character for the most common signal.
8. **Chainable everything.** `.at().scale().rot().color().metallic()` reads top-to-bottom.
9. **No ambiguity.** `+` is arithmetic only. `|` is union. `.displace()` is displacement. No overloading.
10. **Space is free.** `.mirror()`, `.rep()`, `.array()` fold space — O(1) cost for infinite copies.
11. **Escape when needed.** `glsl(p) { ... }` for anything Chisel can't express yet.
