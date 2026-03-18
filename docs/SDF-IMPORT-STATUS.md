# SDF Editor Import — Feature Status

Tracking what Chisel supports vs what's needed to import scenes from IQ's SDF editor.
Based on analysis of 7 scenes: turntable, bear, cash, frank, magneteye, truck, gameboy.

---

## Primitives

| Primitive | Scenes | Chisel Status | Notes |
|-----------|--------|---------------|-------|
| **SoftBox** | **7/7** | **Missing** | Universal. 6 independent rounding radii + Extrude = super-primitive (box, rounded box, capsule, sphere). Chisel has `rounded_box` with single radius. |
| **Curves** | 4/7 | Missing | SVG path-based 2D curves referenced by ID. Used for logos, decorative shapes, antenna. Would need path import or `glsl()` escape. |
| **Text** | 3/7 | Missing | Text characters as SDF. Needs glyph/font system. |
| **Triangle** | 3/7 | **Exists** | `triangle` 2D primitive in Chisel. |
| **Horseshoe** | 2/7 | **Exists** | `horseshoe` in Chisel. Codegen fixed to pass correct vec2 params. |
| **Star** | 2/7 | Missing | Star 2D primitive. Not in Chisel. |
| **Egg** | 0/7* | **Added** | Added in this session. Used in berry scene (not in the 7 analyzed). |

## Operations

| Operation | Scenes | Chisel Status | Notes |
|-----------|--------|---------------|-------|
| **Extrude** | **7/7** | **Partial** | `.extrude(depth)` works. Missing: chamfer params (7/7 scenes use it), offset param (6/7 scenes). |
| **Revolve** | 1/7 | **Exists** | `.revolve(offset)` works. |

## Extrude Gaps (Critical — used in every scene)

| Feature | Scenes | Status |
|---------|--------|--------|
| Basic extrude `[depth, 0, 0]` | 7/7 | **Works** |
| Chamfer `[depth, chamfer_top, chamfer_bottom]` | **7/7** | **Missing** |
| Offset `[depth, ct, cb], [ox, oy, oz]` | 6/7 | **Missing** |

## Blend Modes

| Mode | JSON | UI Name | Scenes | Chisel Status |
|------|------|---------|--------|---------------|
| Union | `"add"` | Add | **7/7** | `\|` and `\|~r` |
| Subtract | `"sub"` | Carve | 5/7 | `-` and `-~r` |
| Material | `"mat"` | Color | 5/7 | `\|@r` **Added** |
| Repel | `"rep"` | Repel | 1/7 | `\|!r` **Added** |
| Avoid | `"avo"` | Avoid | 0/7* | `\|^r` **Added** |
| Intersect | `"int"` | Intersect | 0/7 | `&` and `&~r` |

*Avoid used in berry scene, not in the 7 analyzed.

## Repeat Modes

| Mode | Scenes | Chisel Status | Notes |
|------|--------|---------------|-------|
| None | 7/7 | N/A | |
| **XYZ** | 3/7 | **Partial** | `.rep(spacing, count: N)` exists but no independent per-axis count+spacing. |
| **ANG** | 2/7 | **Partial** | `.array(count, radius: r, axis: a)` exists. Axis support added this session. Parameterization differs from editor. |

## Transforms

| Feature | Scenes | Chisel Status |
|---------|--------|---------------|
| Translation | 7/7 | `.at(x, y, z)` |
| Quaternion rotation | 7/7 | `.quat(x, y, z, w)` **Added** |
| Uniform scale | 7/7 | `.scale(s)` (location[4]) |
| Euler rotation | 7/7 | `.rot(deg, axis)` (redundant with quat) |

## Animation

| Type | Scenes | Chisel Status | Notes |
|------|--------|---------------|-------|
| spin | 2/7 | Approximate | Can use `.rot(t * speed, axis)` |
| wiggle | 3/7 | Approximate | Can use `sin(t * speed) * amplitude` with `.at()` |
| wobble | 2/7 | Approximate | Can use `sin(t * speed) * amplitude` with `.rot()` |
| (none) | 4/7 | N/A | |

Chisel has `t` for time and trig functions. Named animation types aren't built-in but can be composed manually.

## Render Modes

| Mode | Scenes | Chisel Status |
|------|--------|---------------|
| pbr | 4/7 | **Exists** (but metallic/roughness not wired to shader) |
| expressive | 3/7 | Missing | Outline + flat shading style |

## Camera Types

| Type | Scenes | Chisel Status |
|------|--------|---------------|
| perspective | 3/7 | `camera { fov: ... }` |
| freeOrthogonal | 3/7 | Supported in renderer, not in Chisel syntax |
| iso | 1/7 | Supported in renderer, not in Chisel syntax |

## Materials

| Feature | Scenes | Chisel Status |
|---------|--------|---------------|
| Color (RGB) | 7/7 | `.color(r, g, b)` |
| Opacity < 1 | 1/7 | `.opacity(v)` |
| Roughness | 7/7 | `.roughness(v)` declared, **not wired to shader** |
| Metalness | 7/7 | `.metallic(v)` declared, **not wired to shader** |

## Scene Complexity

| Scene | SDF Elements | Total Elements | Groups |
|-------|-------------|----------------|--------|
| gameboy | 303 | 480 | 10 |
| truck | 320 | 342 | 23 |
| turntable | 154 | 167 | 14 |
| cash | 63 | 80 | 18 |
| frank | 39 | 46 | 8 |
| bear | 26 | 37 | 12 |
| magneteye | 25 | 30 | 6 |

---

## Priority Ranking

### P0 — Blocks most scenes
1. **SoftBox primitive** (7/7) — The universal primitive. Every scene uses it exclusively.
2. **Chamfered extrude** (7/7) — Every scene uses non-zero chamfer on extrude.
3. **Extrude offset** (6/7) — Most scenes shift the extrusion origin.

### P1 — Blocks many scenes
4. **Curves/Path primitive** (4/7) — SVG path shapes. Needed for logos, decorative elements.
5. **Text primitive** (3/7) — Text rendering as SDF.
6. **Star primitive** (2/7) — Simple 2D star shape.
7. **PBR material rendering** (4/7) — Roughness/metalness currently ignored by shader.

### P2 — Blocks some scenes
8. **XYZ repeat with per-axis counts** (3/7) — Independent count+spacing per axis.
9. **Expressive render mode** (3/7) — Outline + flat shading.

### P3 — Nice to have
10. **Named animations** (spin/wiggle/wobble) (3/7) — Can be manually composed with `t`.
11. **Orthographic/isometric camera** in Chisel syntax (4/7).
12. **Dome/fresnel lights** — Standard in all scenes but approximated by current shader.

---

## What Chisel Already Supports

- Basic shapes: sphere, box, cylinder, torus, capsule, cone, plane, octahedron, pyramid, ellipsoid, horseshoe, triangle, egg
- 2D shapes: circle, rect, hexagon, triangle, egg + extrude/revolve
- Boolean ops: union, subtract, intersect (sharp, smooth, chamfer)
- New blend ops: paint (`|@`), repel (`|!`), avoid (`|^`)
- Transforms: translate, rotate (axis+angle), quaternion, scale, mirror, orient
- Repetition: linear rep, circular array (with axis support)
- Deformations: twist, bend, shell, onion, morph, displace, dilate, erode, round, elongate
- Materials: color (RGB, hex, HSL, named), opacity
- Lighting: sun, point, ambient
- Animation: `t` variable, trig functions, easing functions, noise
- GLSL escape hatch for custom SDFs

## What's Missing (Summary)

**Primitives:** SoftBox (per-face rounding), Curves (SVG paths), Text, Star
**Operations:** Chamfered extrude, extrude offset
**Rendering:** PBR (metallic/roughness in shader), expressive mode (outlines)
**Repeat:** XYZ with per-axis count+spacing
**Scene:** Orthographic/iso camera syntax, dome/fresnel light types
