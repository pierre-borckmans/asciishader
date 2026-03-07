# AsciiShader

Real-time 3D ray-marched scenes rendered as ASCII art in the terminal. Includes **Chisel**, a high-level shading language that compiles to GLSL, a GPU-accelerated renderer (OpenGL), and a full TUI built on BubbleTea.

## Quick Reference

```
make build          # Build all binaries (asciishader, chisel, chisel-lsp)
make test           # Run all tests
make lint           # Run golangci-lint
make fmt            # Format all Go code (gofmt -w -s)
make fmt-check      # Check formatting without writing
make generate       # Regenerate editor tooling from lang.go
make all            # Build + test + lint (use before committing)
```

Always use the Makefile — do not run `go test`, `go vet`, `gofmt`, or `golangci-lint` directly.
Run `make all` before committing to catch formatting, lint, and test failures.

## Project Structure

```
cmd/asciishader/           Main TUI application

pkg/
  chisel/                  Chisel language — public API (Compile)
    compiler/
      token/               Token types, Position, Span
      lexer/               Hand-written tokenizer
      ast/                 AST node types, Walk, Print
      parser/              Recursive descent + Pratt
      analyzer/            Type checker, scope resolution
      codegen/             AST → GLSL
      diagnostic/          Error types, terminal renderer
    lang/                  Language registry (single source of truth)
    lang/gen/              Code generator for editor tooling
    format/                Source code formatter
    lsp/                   Language Server Protocol server
    editors/               Generated: tree-sitter, TextMate, VS Code config
    cmd/chisel/            CLI (compile, check, fmt)
    cmd/chisel-lsp/        LSP entry point
  gpu/                     OpenGL renderer — framebuffer, SDF→ASCII, ANSI colors
  shader/                  GLSL shader compilation and template assembly
  core/                    Shared types: RenderConfig, Camera, Vec3, render modes
  scene/                   Scene loader — discovers .chisel/.glsl, watches for changes
  recorder/                Records camera + parameters to .asciirec
  clip/                    Playback engine and binary .asciirec format

tui/
  views/                   Gallery, Help, Player views
  editor/                  GLSL/Chisel editor with syntax highlighting
  controls/                Interactive sliders and parameter controls
  layout/                  Sidebar, panels, resizers, animated transitions
  components/              Reusable UI components
  styles/                  Theme and styling constants

shaders/                   25 example scenes (.chisel)
docs/                      Architecture documentation
```

## Documentation

See `docs/ARCHITECTURE.md` for the system architecture, rendering pipeline, and compiler design. **Keep it up to date** when making structural changes.

Chisel language docs:
- `pkg/chisel/LANGUAGE.md` — Language reference
- `pkg/chisel/IMPLEMENTATION.md` — Compiler implementation guide
- `pkg/chisel/GRAMMAR-DESIGN.md` — Grammar single-source-of-truth design

## Key Concepts

- **Chisel** compiles to GLSL fragment shaders. Pipeline: Lexer → Parser → Analyzer → CodeGen.
- The **GPU renderer** ray-marches at sub-pixel resolution, then maps cells to ASCII characters by shape matching.
- The **TUI** is BubbleTea-based with four views: Shader (F1), Player (F2), Gallery (F3), Help (F4).
- **Language registry** (`lang/lang.go`) is the single source of truth — `go generate` produces all editor grammars.
- **Render modes**: Shapes, Blocks, Braille, Slice, Cost heatmap.

## Development Workflow

1. Edit code
2. `make all` (builds, tests, lints)
3. To test only Chisel: `make test-chisel`
4. To check all shaders compile: `make chisel-check`
5. After changing `lang.go`: `make generate`
