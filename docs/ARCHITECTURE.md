# Architecture

> **This document must be kept up to date.** When making structural changes — adding packages, changing the rendering pipeline, modifying the compiler stages, or altering the TUI layout — update the relevant sections here.

## System Overview

AsciiShader is three systems wired together:

1. **Chisel Compiler** — transforms `.chisel` source into GLSL fragment shaders
2. **GPU Renderer** — ray-marches the GLSL shader on the GPU, maps output to ASCII cells
3. **TUI Application** — displays the ASCII output interactively with camera controls, editor, gallery, and recording

```
┌─────────────────┐     ┌──────────────┐     ┌──────────────┐     ┌──────────┐
│  .chisel source │────▶│    Chisel     │────▶│  GLSL frag   │────▶│   GPU    │
│  (or raw .glsl) │     │   Compiler   │     │   shader     │     │ Renderer │
└─────────────────┘     └──────────────┘     └──────────────┘     └────┬─────┘
                                                                       │
                                                            ASCII cell grid
                                                                       │
                                                                  ┌────▼─────┐
                                                                  │   TUI    │
                                                                  │ (Bubble  │
                                                                  │   Tea)   │
                                                                  └──────────┘
```

## Chisel Compiler (`pkg/chisel/`)

A four-stage pipeline under `compiler/`, each stage independently testable:

### Stage 1: Lexer (`compiler/lexer/`)
- Hand-written tokenizer producing 50+ token types
- Handles significant newlines (Go-style automatic semicolons adapted for shape unions)
- Newline suppression after continuation tokens (`|`, `&`, `,`, etc.)
- Newline suppression before `.` to support method chaining across lines

### Stage 2: Parser (`compiler/parser/`)
- Recursive descent with Pratt operator precedence for mixed SDF boolean / arithmetic expressions
- 8 precedence levels: union < subtract < intersect < comparison < add/sub < mul/div < unary < postfix
- Error recovery: on unexpected token, skip to next newline/`}`/`)`, record diagnostic, continue
- Parses settings blocks (camera, light, bg, raymarch, post, debug) with `key: value` syntax
- GLSL escape blocks captured as raw strings with brace-nesting tracking

### Stage 3: Analyzer (`compiler/analyzer/`)
- Type system: float, vec2, vec3, bool, sdf2d, sdf3d, material, color, signal
- Name resolution with scoped symbol table (builtins → top-level → block → loop)
- Validates SDF operations (both sides same type), catches 2D shapes without `.extrude()`
- Fuzzy matching for "did you mean?" suggestions on undefined identifiers

### Stage 4: Code Generator (`compiler/codegen/`)
- Emits GLSL with `sceneSDF(vec3 p)` and `sceneColor(vec3 p)` entry points
- Shapes → GLSL SDF function calls with correct argument mapping
- Transforms modify the point variable (`vec3 pN = transform(p)`)
- For loops unrolled at compile time (GLSL can't dynamically union SDFs)
- Noise/easing/helper functions emitted only when used (dead code elimination)

### Supporting Compiler Packages
- `compiler/ast/` — AST node definitions, `Walk()` traversal, `Print()` debug output
- `compiler/token/` — Token types and position tracking
- `compiler/diagnostic/` — Error/warning structs with spans; Rust/Elm-style rendered output

### Language Registry (`lang/`)
- Single source of truth for all Chisel vocabulary: shapes, methods, functions, constants, colors
- Operator precedence table and terminal patterns shared with the parser
- `go generate ./pkg/chisel/lang/` produces all editor tooling files
- See `GRAMMAR-DESIGN.md` for the rationale

### Formatter (`format/`)
- Canonical code formatter with comment preservation
- Width-aware line breaking for method chains and argument lists (100-char target)
- Precedence-aware parenthesization to avoid semantic changes

### LSP (`lsp/`)
- JSON-RPC 2.0 over stdin/stdout, no external framework
- Diagnostics (errors/warnings on keystroke)
- Completions (shapes, methods, functions, variables in scope)
- Hover documentation
- Go-to-definition (user definitions + virtual builtins document)
- Folding ranges
- Document formatting
- Semantic tokens (shapes, functions, variables, parameters, methods, constants colored by role)

### Editor Tooling (`editors/`)
All generated from `lang/lang.go` via `go generate`:
- Tree-sitter grammar (`grammar.js`) + highlight/fold/indent queries
- TextMate grammar (`chisel.tmLanguage.json`) + preferences
- VS Code language configuration (`language-configuration.json`)

## GPU Renderer (`pkg/gpu/`)

Renders ray-marched SDF scenes to a grid of ASCII cells:

1. **Setup**: Creates an offscreen OpenGL framebuffer at sub-pixel resolution (2-3x terminal cell size)
2. **Render**: Executes the GLSL fragment shader via a full-screen quad. The shader ray-marches from the camera through each pixel, evaluating the SDF, computing lighting (diffuse, specular, AO, shadows), and outputting color.
3. **Readback**: Reads the framebuffer pixels back to CPU
4. **ASCII Mapping**: For each terminal cell, samples the sub-pixel block and matches it against a shape lookup table of ASCII characters. Picks the character whose shape best matches the brightness pattern. Assigns ANSI foreground/background colors.

### Render Modes (defined in `pkg/core/`)
| Mode | Description |
|------|-------------|
| Shapes | Match sub-pixel brightness to ASCII character shapes |
| Dual | Two-character cells for higher horizontal resolution |
| Blocks | Unicode block elements |
| Half-block | Half-block characters for 2x vertical resolution |
| Braille | Braille dot patterns for highest effective resolution |
| Density | Simple brightness-to-character density mapping |
| Slice | 2D cross-section of the SDF distance field |
| Cost | Heatmap of ray-march step count (for performance debugging) |

### Uniforms
The renderer passes uniforms to the shader: `uTime`, `uResolution`, camera position/target, render parameters (contrast, spread, ambient, specular, shadow, AO strengths).

## TUI Application (`cmd/asciishader/`, `tui/`)

Built on Charm's BubbleTea framework (event-driven Elm architecture):

### Views
- **Shader View (F1)** — Main viewport with live rendering, mouse camera control, right-side parameter sliders, bottom editor panel
- **Player View (F2)** — Playback of recorded `.asciirec` clips
- **Gallery View (F3)** — Browse and select from available scenes
- **Help View (F4)** — Keybindings reference

### Layout System (`tui/layout/`)
- Sidebar with animated open/close transitions
- Resizable panels (editor, controls)
- Focus management across zones: Viewport, Controls, Editor

### Editor (`tui/editor/`)
- Syntax-highlighted GLSL/Chisel editing in the terminal
- Ctrl+R to compile and hot-reload the shader
- Compilation errors shown in status bar

### Controls (`tui/controls/`)
- Interactive sliders for render parameters
- Keys 1-6 and Shift+1-6 to adjust individual parameters
- R to reset all parameters to defaults

## Scene Management (`pkg/scene/`)

- Discovers `.chisel` and `.glsl` files in the `shaders/` directory
- File watching for hot-reload on external edits
- Chisel files are compiled to GLSL on load; raw GLSL files used directly

## Recording & Playback

### Recorder (`pkg/recorder/`)
- Captures camera state and render parameters each frame
- Region selection UI with preset sizes
- Bakes/re-renders at higher quality after recording stops

### Clip Format (`pkg/clip/`)
- Binary `.asciirec` format
- Delta encoding: stores XOR of colors and varint-encoded deltas between frames
- zstd compression for final output
- Playback engine with pause, loop, seek

## Build System

All build, test, lint, and format operations go through the `Makefile`. See `CLAUDE.md` for the command reference.

### Binaries
| Target | Binary | Description |
|--------|--------|-------------|
| `build-app` | `asciishader` | Interactive TUI application |
| `build-chisel` | `chisel` | CLI compiler (compile, check, fmt) |
| `build-lsp` | `chisel-lsp` | Language server for editor integration |
