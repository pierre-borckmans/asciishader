# SDF Editor Model Format

Reverse-engineered from JSON exports of Inigo Quilez's online SDF modeler. This document progressively builds understanding of the format to inform Chisel language extensions.

---

## Top-Level Structure

```
version            — Format version ("0.1")
info               — Scene metadata (name, size, author)
rendermode         — Render mode ("pbr")
environment        — Background, floor, post-processing
lighting           — Lights array + occlusion settings
cameras            — Camera definitions
snapshots          — Named camera bookmarks
materials          — Material palette (referenced by UUID)
model              — The 3D scene graph
```

---

## Model / Scene Graph

The model is a tree of **groups**, each containing **elements** (leaf SDF nodes) or references to other groups.

```
model
  └─ location       — Root transform [tx, ty, tz, ?, scale, qx, qy, qz, qw]
  └─ groups[]
       ├─ name       — Group name (e.g. "Walls", "Door", "Ducky")
       ├─ uuid       — Unique ID
       └─ elements[] — Array of SDF elements
```

### Group Referencing

The first group is the **root**. Its elements can reference other groups by name. When an element's `prim` is `["Walls"]` or `["Ducky"]`, it's a reference to a group with that name — the group's elements are composed into a sub-scene with the element's transform applied as parent.

---

## Elements

Each element is a leaf SDF node with:

```json
{
  "name": "Wall_Right",
  "uuid": "...",
  "visible": true,
  "locked": false,
  "rm": true,
  "blend": ["add", 0, "medial"],
  "material": "uuid-of-material",
  "symmetry": [false, false, false],
  "location": [tx, ty, tz, ?, scale, qx, qy, qz, qw],
  "rotation": [rx, ry, rz],
  "repeat": ["None"],
  "prim": [...]
}
```

### Fields

| Field | Description |
|-------|-------------|
| `rm` | `true` = this element is a direct SDF primitive. `false` or absent = it's a group reference or non-raymarched element. |
| `blend` | How this element combines with siblings: `["add", radius, "medial"]`, `["sub", ...]`, `["mat", ...]` |
| `material` | UUID reference into the `materials` array |
| `symmetry` | Per-axis mirroring: `[mirrorX, mirrorY, mirrorZ]` |
| `location` | 9-element array: `[tx, ty, tz, ???, scale, qx, qy, qz, qw]` — position, unknown flag, uniform scale, quaternion rotation |
| `rotation` | Euler angles in degrees `[rx, ry, rz]` — **optional**, absent in scene 2. Likely redundant with the quaternion in `location`, kept for UI display. |
| `repeat` | Repetition mode (see below) |
| `prim` | Primitive definition (see below) |

---

## Blend Modes

How an element combines with the scene built so far (its siblings evaluated in order).

### Format

```json
"blend": ["mode", radius]
"blend": ["mode", radius, "interpolation"]
```

The third parameter (e.g. `"medial"`) is optional and selects the blending interpolation curve. When omitted, a default is used. The `radius` controls the smoothness of the blend (0 = sharp).

### All Modes (from UI screenshot)

The editor's Combine panel groups modes into three categories:

**Additive:**
| JSON code | UI name | Meaning | Chisel equivalent |
|-----------|---------|---------|-------------------|
| `"add"` | **Add** | Union (min of SDFs) | `a \| b` or `a \|~r b` |
| `"mat"` | **Color** | Material-only — paints material without changing geometry | **Not in Chisel** |
| `"rep"` | **Repel** | Repels/pushes existing geometry away, inserts this element | **Not in Chisel** |
| `"avo"` | **Avoid** | Smooth union variant that avoids inflation at blend zone | **Not in Chisel** |

**Subtractive:**
| JSON code | UI name | Meaning | Chisel equivalent |
|-----------|---------|---------|-------------------|
| `"sub"` | **Carve** | Subtraction (removes from existing) | `a - b` or `a -~r b` |
| `"int"` | **Intersect** | Intersection (keeps only overlap) | `a & b` or `a &~r b` |

**Blending slider:** Controls the smooth blend radius (0 = sharp, higher = smoother transition). Maps to the second element in the JSON array.

### `"mat"` / Color

Applies a material to a region without affecting geometry. Like "painting" onto existing shapes.

**Examples:** Wall paint bands, floor tile colors, towel stripes, duck eye details, leaf coloring.

Semantics: `if (this_element_sdf < 0) use_this_material; else keep_existing;`

When blend radius > 0, the material transition is smoothly blended at the boundary.

### `"avo"` / Avoid

Smooth union variant that compensates for the "inflation" artifact that standard smooth min creates at blend zones. This is a known technique from IQ's work — the blended surface stays closer to the original shapes instead of bulging outward.

Used in scene 2 to combine petal layers where inflation would distort the shape.

### `"rep"` / Repel

Pushes existing geometry away from this element, creating a smooth gap/clearance around it before inserting. Different from plain `"add"` which just takes the minimum — `"rep"` actively deforms surrounding geometry to make room.

Used in scene 2 for water drops — they push slightly into the surface, creating a natural-looking contact point rather than just sitting on top.

---

## Primitives (`prim`)

### Group References

When `prim` is a single-element array with a name matching a group:
```json
"prim": ["Walls"]
"prim": ["Ducky"]
"prim": ["Door"]
```
This inserts the referenced group's entire sub-tree with the element's transform applied.

### Direct Primitives

When `rm: true`, the element is a direct SDF primitive. Format:

```json
"prim": ["PrimitiveName", [params...], "Operation", [op_params...], [offset...]]
```

### Primitive Types Observed

#### `SoftBox` — Rounded box with per-face rounding
```json
["SoftBox", [r1, r2, r3, r4, r5, r6]]
```
Six independent rounding radii. This is the **universal workhorse primitive** — used for walls, tiles, doors, soap, shampoo, spheres, capsules, and more.

The 6 parameters likely control rounding per face or edge group. When combined with `Extrude`, the shape's final form depends on the relationship between rounding radii and extrude depth:

**Special cases observed:**
- **All 6 radii equal + Extrude all 3 params equal to same value → Sphere**
  ```json
  ["SoftBox", [0.17, 0.17, 0.17, 0.17, 0.17, 0.17], "Extrude", [0.17, 0.17, 0.17], [0,0,0]]
  ```
  This is how the editor represents a sphere — maximum rounding on a box.

- **Two radii large, rest zero → Flat panel** (walls)
- **Mixed radii → Asymmetric rounding** (soap bar, door lock)
- **Small radii + large extrude → Tall box with subtle edge rounding** (tile frames)

This confirms SoftBox + Extrude is a single "super-primitive" that can represent boxes, rounded boxes, capsules, and spheres depending on parameters.

**Not in Chisel.** Chisel's `rounded_box` only has a single uniform radius.

#### `Egg` — Ovoid / egg shape
```json
["Egg", [height, width, offset, eggness]]
```
Asymmetric ellipsoid. The `eggness` parameter (range roughly -1 to 1) controls how pointy one end is vs the other. Used for the duck body, duck beak, mirror frame, mirror handle.

**Not in Chisel.**

#### `Horseshoe` — Arc / horseshoe shape
```json
["Horseshoe", [angle, radius, thickness, width, param5, param6]]
```
An arc (partial torus cross-section). Very versatile:
- **Large angle + thick**: towel draping, towel handle (scene 1)
- **Angle=2 + tiny radii + Revolve**: water droplets / teardrops (scene 2)
- **With Extrude**: leaf/petal shapes (scene 2)

The 6 parameters appear to be: `[angle_radians, major_radius, minor_thickness, tube_width, param5, param6]`. The last two parameters may control additional rounding or tapering.

**Exists in Chisel's lang.go** — needs verification that it works with both Extrude and Revolve.

### Operations Applied to Primitives

After the primitive params, an operation transforms the 2D cross-section into 3D:

#### `Extrude`
```json
"Extrude", [depth, chamfer_top, chamfer_bottom], [offset_x, offset_y, offset_z]
```
Extrudes the 2D primitive profile into 3D. The chamfer parameters bevel the top and bottom edges. The offset shifts the extrusion origin.

**Chisel gap:** `.extrude(depth)` has no chamfer or offset parameters.

#### `Revolve`
```json
"Revolve", [offset, param2], [offset_x, offset_y, offset_z]
```
Revolves the 2D profile around an axis to create a surface of revolution.

The first parameter is the **revolution offset** — how far the profile is shifted from the revolution axis before revolving:
- `[0, 0]` → solid of revolution (profile touches axis) — duck body, mirror handle
- `[0.011, 0]` → hollow/tube revolution (profile offset from axis) — creates a torus-like ring. Used for petal layers in scene 2.

**Chisel has `.revolve(radius)`** but only for 2D shapes, not for 3D primitives like Egg. The offset parameter is the key to creating rings vs solids.

---

## Repeat Modes

```json
"repeat": ["None"]
"repeat": ["XYZ", countX, countY, countZ, spacingX, spacingY, spacingZ]
"repeat": ["ANG", unknown, count, offset]
```

### `"XYZ"` — Grid Repetition

Creates a grid of copies with per-axis count and spacing:
```json
"repeat": ["XYZ", 9, 1, 9, 0.12992, 0.22021, 0.12992]
```
This creates a 9×1×9 grid with specified spacing per axis.

**Chisel gap:** `.rep(spacing, count: N)` exists but doesn't support independent per-axis count and spacing in a single call.

### `"ANG"` — Angular / Circular Repetition

Creates copies arranged in a circle:
```json
"repeat": ["ANG", 0, 6, 0.026]
```
- Index 1: Unknown (always 0 so far — possibly axis or start angle)
- Index 2: Count (6 copies)
- Index 3: Offset/gap between copies (0.026)

Used in scene 2 for arranging petals around a central axis. Like Chisel's `.array(count, radius: r)` but parameterized differently — using angular offset rather than radius.

**Chisel has `.array()`** but the parameterization may differ.

---

## Transform: Location Array

```json
"location": [tx, ty, tz, unknown, scale, qx, qy, qz, qw]
```

- Indices 0-2: Translation (x, y, z)
- Index 3: Unknown (always 0 in observed data — possibly a flags field)
- Index 4: Uniform scale
- Indices 5-8: Quaternion rotation (x, y, z, w)

**Chisel gap:** No quaternion rotation support. `.rot(deg, axis)` only rotates around a single axis. Arbitrary orientations require chaining 2-3 rotations, which doesn't compose the same way.

---

## Materials

Materials are defined in a top-level `materials` array and referenced by UUID from elements.

```json
{
  "name": "floor",
  "uuid": "...",
  "opacity": 1,
  "pbr": {
    "color": [0.02, 0.21, 0.28],
    "roughness": 0.5,
    "metalness": 0,
    "reflect": false
  }
}
```

### PBR Properties

| Property | Type | Description |
|----------|------|-------------|
| `color` | `[r, g, b]` | Linear RGB, 0-1 range |
| `roughness` | `float` | 0 (mirror) to 1 (diffuse) |
| `metalness` | `float` | 0 (dielectric) to 1 (metal) |
| `reflect` | `bool` | Whether to compute reflections |
| `opacity` | `float` | 1 = opaque, 0 = invisible (on the element, not in pbr) |

**Chisel gap:** `.metallic()` and `.roughness()` are declared in lang.go but **not wired into the shader**. They have no effect on rendering.

Materials also have `expressive` and `illustrative` slots for non-PBR render modes — not relevant to Chisel.

---

## Lighting

```json
"lighting": {
  "occlusion": { "enabled": true, "distance": 0.07 },
  "lights": [
    { "type": "directional", "data": { "color": [1,0.9,0.8], "intensity": 2, "direction": [...], "shadowEnable": true, "shadowBlur": 16, "diffBias": 0 }},
    { "type": "dome",        "data": { "color": [1,1,1], "intensity": 0.8 }},
    { "type": "ambient",     "data": { "color": [...], "intensity": 0.5, "diffPower": 1 }},
    { "type": "fresnel",     "data": { "color": [...], "intensity": 1, "diffPower": 1 }}
  ]
}
```

### Light Types

| Type | Description | Chisel equivalent |
|------|-------------|-------------------|
| `directional` | Sun-like parallel light with direction, shadows | `sun { dir, color, intensity }` |
| `dome` | Hemisphere/sky light | **Not in Chisel** |
| `ambient` | Uniform ambient with diffuse power control | `ambient: value` (no diffPower) |
| `fresnel` | Rim/edge lighting | Hardcoded in shader, not configurable |

### Occlusion
Configurable AO distance. Chisel has `ao: value` but no distance control.

### Shadow Controls
Per-light `shadowEnable`, `shadowBlur`, `diffBias`. Chisel has `shadows: true/false` but no blur or bias.

---

## Cameras

```json
{ "type": "perspective", "model": "default", "focal_length": 61, "focal_plane": 0, "aperture": 0, "distortion": 0 }
{ "type": "freeOrthogonal", "size": 2, "focal_plane": 0, "aperture": 0, "distortion": 0 }
{ "type": "iso", "size": 2, ... }
```

| Type | Description | Chisel equivalent |
|------|-------------|-------------------|
| `perspective` | Standard perspective with focal length | `camera { fov: ... }` |
| `freeOrthogonal` | Orthographic projection | Supported in renderer but not in Chisel syntax |
| `iso` | Isometric projection | Supported in renderer but not in Chisel syntax |

---

## Environment

```json
"environment": {
  "background": { "type": "solid", "data": { "color": [245, 204, 95] }},
  "floor": { "enabled": true, "height": 0, "reflection": true },
  "postprocess": { "enabled": false }
}
```

The floor is a built-in ground plane with optional reflection — not a user-placed primitive.

**Chisel gap:** No reflective floor support.

---

## Summary of Gaps for Chisel

### Critical (blocks import of most scenes)
1. **SoftBox** — per-face rounding radii (universal primitive)
2. **Chamfered extrude** — `Extrude [depth, chamfer_top, chamfer_bottom]`
3. **Material-only blend** (`"mat"`) — paint without geometry change
4. **PBR rendering** — metallic/roughness actually affecting shading
5. **Avoid blend** (`"avo"` / Avoid) — inflation-compensated smooth union
6. **Repel blend** (`"rep"` / Repel) — push existing geometry away, insert with clearance

### Important (needed for complex scenes)
7. **Egg primitive** — asymmetric ovoid
8. **Quaternion rotation** — arbitrary orientation
9. **Per-axis grid repetition** — independent count+spacing per axis
10. **Angular repetition** (`"ANG"`) — circular array by count+offset
11. **Revolve offset** — hollow vs solid revolution (ring vs blob)
12. **Dome light** — hemisphere ambient
13. **Extrude offset** — shift extrusion origin

### Nice-to-have
14. **Fresnel light** (configurable)
15. **Shadow blur/bias**
16. **Reflective floor**
17. **AO distance control**
18. **Orthographic/isometric camera** in Chisel syntax

---

## Scenes Analyzed

1. **Bathroom** — Walls, door, floor tiles, water, window, rubber duck, soap, shampoo, mirror, towel. Heavy use of SoftBox + Extrude, groups, material-only blend, XYZ repetition.
2. **Pineapple/succulent** — Layered petals (Egg + Revolve + ANG repeat), water drops (Horseshoe + Revolve), leaf (Horseshoe + Extrude). Introduces `"avo"` and `"rep"` blend modes, angular repetition, revolve offset.

---

*This document will be updated as more scene exports are analyzed.*
