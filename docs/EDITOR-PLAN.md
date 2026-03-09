# Editor Panel — Plan

Interactive scene editing through the TUI right panel, powered by Chisel's compiler infrastructure (AST, spans, parser).

## Done

### TreeView Component (`tui/components/tree_view.go`)
- Generic collapsible tree with keyboard (arrows, hjkl, enter) and mouse (click, scroll) navigation
- Lazy child expansion via `Children func() []TreeNode`
- Path-based expand/collapse state persistence across tree rebuilds
- Cursor position preserved across rebuilds by path matching
- Scrollbar rendering, viewport scrolling

### Scene Tree Builder (`tui/components/scene_tree.go`)
- Converts Chisel AST into TreeNode hierarchy
- Four sections: Variables, Functions, Settings, Geometry
- Method chain flattening: `sphere(1).at(2,0,0).color(#f00)` → sphere node with `.at()`, `.color()` children
- Handles all settings body types: maps, expressions, blocks, mat special case
- Single-expression settings shown inline (e.g. `bg  #1a1a2e`)
- `SpanFromData()` extracts source spans from tree nodes (supports `NodeData` wrapper)

### Inline Property Editing (Phase 2)
- `NodeData` type carries both node span and editable value span
- `isEditableExpr()` detects editable values: numbers, colors, booleans, vectors
- Editable nodes marked with `Editable: true` and `EditValue` pre-populated from source span
- TreeView edit mode: Enter on editable leaf → inline text input with cursor
- Edit key handling: typing, backspace, delete, cursor movement, Enter/Esc
- `EditResult` struct delivered to caller after confirmed edit
- App splices new value into source at `EditSpan` offsets, recompiles, rebuilds tree

### Scaffold Nodes (Phase 3)
- Scaffold nodes for 5 setting types: bg, light, camera, raymarch, post
- `ScaffoldInfo` carries template text and insertion byte offset
- Missing settings auto-detected from AST; scaffold nodes appended to Settings section
- Settings section always shown when scaffolds exist (even with no real settings)
- Dimmed `+` prefix rendering in TreeView
- Enter/click on scaffold → `ScaffoldResult` delivered → app inserts template and recompiles

### TUI Wiring (`tui/app/`)
- Scene tree renders below controls in the right panel
- Focus cycling: Controls → Tree → Editor → Viewport
- Tree rebuilds on recompile (ctrl+r) and file reload
- Mouse hit-testing with correct screen position accounting for scroll offset

## Phase 1 — Jump to Source

Select a tree node → cursor jumps to the corresponding line in the editor panel.

- Each tree node already carries `token.Span` in its `Data` field
- Wire `OnSelect` callback: extract span, set editor cursor to `span.Start.Line`
- Auto-expand bottom panel if collapsed
- Highlight the target line briefly

Dependencies: none — spans are already stored.

## Phase 2 — Inline Property Editing ✓

Done. See "Done" section above.

Future enhancements:
- Color picker for hex colors
- More slider range heuristics for additional method/primitive types

### Slider Editing (Phase 2b) ✓
- Method args with `SliderRange` get inline slider on Enter or double-click
- Keyboard: left/right adjust value by step, Enter/Esc dismiss
- Mouse: click/drag on slider bar for continuous adjustment
- `sliderRangeForMethod`: opacity (0–1), round/shell (0–1), scale (0.01–5)
- `sliderRangeForPrimitive`: sphere/octahedron/box/torus (0.01–5)
- Slider emits `EditResult` continuously → source splice → recompile → live preview

## Phase 3 — Scaffold Nodes ✓

Done. See "Done" section above.

Future enhancements:
- Scaffold for `mat` (named material template)
- Scaffold for complex lighting (sun, point, fog sub-blocks)
- Custom templates per scene type

## Phase 4 — Rich Diagnostics

Surface compiler errors and warnings directly in the tree and editor.

- After compile, map `diagnostic.Diagnostic` spans to tree nodes
- Render error nodes with red styling, warning nodes with yellow
- In the editor, underline or highlight the error span
- Clicking an error node in the tree jumps to the error location in the editor

Dependencies: Phase 1. Can be developed in parallel with Phase 2.

## Phase 5 — Drag Reorder & Reparent

Drag geometry nodes to reorder CSG children or move shapes between union/intersect/subtract groups.

- Detect drag start on a geometry node
- Show drop targets (before/after siblings, into parent groups)
- On drop, splice source text: remove the expression from its old location, insert at new location
- Handle indentation and separator adjustments

Dependencies: Phase 2 (source splicing).

This is the most complex phase — CSG tree restructuring requires careful span math to avoid overlapping edits.

## Phase 6 — Live Preview Annotations

Overlay information from the tree onto the 3D viewport.

- Hover a tree node → highlight the corresponding shape in the viewport (wireframe or color tint)
- Show axis gizmos for `.at()` translations
- Display bounding boxes for CSG groups

Dependencies: Phase 2, renderer support for per-shape highlight passes.

## Architecture Notes

### Source Text as Ground Truth
All edits go through source text splicing, never direct AST manipulation. This means:
- The formatter can clean up after edits
- The parser re-validates everything
- Undo is just text undo
- File watching still works (external edits merge naturally)

### Span-Based Splicing
```
func splice(source string, span token.Span, replacement string) string {
    return source[:span.Start.Offset] + replacement + source[span.End.Offset:]
}
```
Multiple edits in one pass must be applied back-to-front (highest offset first) to keep earlier offsets valid.

### Tree Rebuild Cycle
```
edit source → parse → analyze → codegen → GPU compile
                ↓
          build tree nodes
                ↓
     SetRoots (preserves expand state + cursor)
```
This cycle already works for recompile and file reload. Inline edits will trigger the same cycle.
