# AsciiShader

Real-time 3D ray-marched scenes rendered as ASCII art in the terminal. Includes **Chisel**, a high-level shading language that compiles to GLSL, a GPU-accelerated renderer (OpenGL), and a full TUI built on BubbleTea.

## Quick Reference

```
make build          # Build all binaries (asciishader, chisel, chisel-lsp)
make test           # Run all tests
make lint           # Run vet + staticcheck
make fmt            # Format all Go code (gofmt -w -s)
make fmt-check      # Check formatting without writing
make all            # Build + test + lint (use before committing)
```

Always use the Makefile — do not run `go test`, `go vet`, `gofmt`, or `staticcheck` directly.
Run `make all` before committing to catch formatting, lint, and test failures.

## Project Structure

```
cmd/
  asciishader/       Main TUI application
  chisel/            Chisel CLI compiler (compile, check, fmt)
  chisel-lsp/        Language Server Protocol for Chisel

pkg/
  chisel/            Chisel compiler pipeline (lexer → parser → analyzer → codegen)
  gpu/               OpenGL renderer — framebuffer, SDF→ASCII mapping, ANSI colors
  shader/            GLSL shader compilation and template assembly
  core/              Shared types: RenderConfig, Camera, Vec3, render modes
  scene/             Scene loader — discovers .chisel/.glsl files, watches for changes
  recorder/          Records camera movements and parameters to .asciirec
  clip/              Playback engine and binary .asciirec format (varint + zstd)

tui/
  views/             Gallery, Help, Player views
  editor/            GLSL/Chisel editor with syntax highlighting
  controls/          Interactive sliders and parameter controls
  layout/            Sidebar, panels, resizers, animated transitions
  components/        Reusable UI components (scrollable, sliders, panel animators)
  styles/            Theme and styling constants

shaders/             Example scenes (.chisel and .glsl)
tree-sitter-chisel/  Tree-sitter grammar for editor syntax highlighting
docs/                Architecture and design documentation
```

## Documentation

See `docs/ARCHITECTURE.md` for a detailed overview of the system architecture, rendering pipeline, and compiler design. **Keep this file up to date** when making structural changes.

Key language/compiler docs live alongside the compiler code:
- `pkg/chisel/LANGUAGE.md` — Chisel language reference
- `pkg/chisel/IMPLEMENTATION.md` — Compiler implementation guide
- `pkg/chisel/TASKS.md` — Compiler task breakdown

## Key Concepts

- **Chisel** compiles to GLSL fragment shaders. The pipeline is: Lexer → Parser → Analyzer → Code Generator.
- The **GPU renderer** ray-marches at sub-pixel resolution, then maps cells to ASCII characters by shape matching.
- The **TUI** is BubbleTea-based with four views: Shader (F1), Player (F2), Gallery (F3), Help (F4).
- **Render modes**: Shapes, Dual, Blocks, Half-block, Braille, Density, Slice, Cost heatmap.
- **Recording** captures camera + parameters per frame, delta-encodes with zstd compression.

## Development Workflow

1. Edit code
2. `make fmt` to format
3. `make test` to verify
4. `make lint` to catch issues
5. Or just `make all` to do everything at once

To test only the Chisel compiler: `make test-chisel`
To run fixture-based compiler tests: `make test-fixtures`
To check all shader files compile: `make chisel-check`
