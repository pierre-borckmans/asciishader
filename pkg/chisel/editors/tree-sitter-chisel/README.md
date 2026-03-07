# tree-sitter-chisel

Tree-sitter grammar for the [Chisel](../../LANGUAGE.md) SDF language.

**This grammar is generated** from `pkg/chisel/lang/lang.go`. Do not edit `grammar.js` or the query files directly — run `go generate ./pkg/chisel/lang/` instead.

## Features

- Full syntax highlighting for all Chisel constructs
- Code folding for blocks, loops, conditionals, and GLSL escapes
- Auto-indentation

## Building & Testing

```bash
# Generate the parser from grammar.js
tree-sitter generate

# Parse a file and print the syntax tree
tree-sitter parse path/to/file.chisel

# Run all valid test files
for f in ../../testdata/valid/*.chisel; do
  tree-sitter parse "$f" 2>&1 | grep -q ERROR && echo "FAIL: $f"
done
```

Requires the `tree-sitter-cli`:
```bash
brew install tree-sitter-cli
```

## Editor Setup

### Neovim

Add to your tree-sitter config:

```lua
require('nvim-treesitter.parsers').get_parser_configs().chisel = {
  install_info = {
    url = "path/to/tree-sitter-chisel",
    files = {"src/parser.c"},
  },
  filetype = "chisel",
}
```

### Helix

Copy `queries/` to `~/.config/helix/runtime/queries/chisel/`

### VS Code

Use the tree-sitter VS Code extension with this grammar, or use the TextMate grammar in `chisel.tmbundle/`.
