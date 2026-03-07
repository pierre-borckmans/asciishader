# AsciiShader

Real-time 3D scenes rendered as ASCII art in the terminal. Write scenes in **Chisel** — a small language for constructive solid geometry — and watch them ray-march on the GPU, mapped to colored ASCII characters.

```chisel
// Temple with pillars
pillar(h = 6) = {
  cylinder(0.3, h)
  | sphere(0.4).at(0, h / 4, 0)
  | box(1.2, 0.3, 1.2).at(0, -h / 4, 0)
}

base = box(16, 0.6, 10).at(0, -1.65, 0)
roof = box(14, 0.6, 12).at(0, 1.65, 0)

pillars = for i in 0..4 {
  pillar().at(-3 + i * 2, 0, -2)
  | pillar().at(-3 + i * 2, 0, 2)
}

base | pillars | roof
```

## What it does

Chisel source compiles to GLSL. The GPU ray-marches the signed distance field, then the renderer maps the framebuffer to ASCII characters by matching sub-pixel brightness patterns to character shapes. The result is a live, interactive 3D scene in your terminal — with mouse camera control, parameter sliders, and an in-terminal editor.

Render modes include shaped ASCII, unicode block elements, braille dots, and a ray-march cost heatmap for debugging.

## Install

Requires Go 1.25+ and OpenGL 4.1+.

```
git clone https://github.com/pierre-borckmans/asciishader.git
cd asciishader
make build
```

This produces three binaries:
- `./asciishader` — the TUI app
- `./chisel` — CLI compiler (compile, check, format)
- `./chisel-lsp` — language server for editor integration

## Usage

```
# Run the TUI
./asciishader

# Compile a scene to GLSL
./chisel compile shaders/temple.chisel

# Check all scenes for errors
./chisel check shaders/*.chisel

# Format a file in place
./chisel fmt shaders/temple.chisel
```

In the TUI: **F1** shader view, **F3** gallery, mouse to orbit, scroll to zoom, **E** to open the editor, **Ctrl+R** to recompile.

## Chisel language

Chisel is a domain-specific language for describing 3D scenes using signed distance fields. It compiles to GLSL.

```chisel
// Shapes
sphere(0.5)
box(1, 2, 1)
cylinder(0.3, 4)

// Boolean operations
sphere | box                    // union
sphere - cylinder               // subtract
sphere & box                    // intersect
sphere |~0.3 box                // smooth union (blend radius 0.3)

// Transforms and materials
sphere.at(2, 0, 0).rot(45, y).scale(0.5).red
box.mirror(x, z).rep(3)

// Functions, loops, animation
pillar(r, h) = cylinder(r, h) | sphere(r * 1.2).at(0, h/2, 0)

for i in 0..8 {
  pillar(0.2, 3).at(sin(i) * 4, 0, cos(i) * 4)
}

// Time-based animation
sphere.scale(1 + sin(t) * 0.3)

// Raw GLSL when you need it
glsl(p) {
  return length(p) - 1.0;
}
```

Full reference: [pkg/chisel/LANGUAGE.md](pkg/chisel/LANGUAGE.md)

## Editor support

The LSP provides completions, hover docs, go-to-definition, diagnostics, formatting, folding, and semantic highlighting. Generated tree-sitter and TextMate grammars work in Neovim, Helix, VS Code, IntelliJ, and Sublime.

```
# Start the language server
./chisel-lsp
```

Point your editor's LSP config at the `chisel-lsp` binary. For TextMate-based editors, install `pkg/chisel/editors/chisel.tmbundle`.

## Development

```
make all            # build + test + lint
make test-chisel    # test only the compiler
make chisel-check   # check all shaders compile
make generate       # regenerate editor tooling after changing lang.go
make lint           # golangci-lint
```

## Project structure

```
pkg/chisel/
  compiler/          Lexer → Parser → Analyzer → CodeGen
  lang/              Language registry — single source of truth for all tooling
  format/            Code formatter
  lsp/               Language server
  editors/           Generated tree-sitter, TextMate, VS Code config

pkg/gpu/             OpenGL renderer, ASCII mapping
pkg/shader/          GLSL template assembly
pkg/core/            Shared types (camera, render config)
tui/                 BubbleTea terminal UI
shaders/             25 example scenes
```

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the full system design.

## License

MIT
