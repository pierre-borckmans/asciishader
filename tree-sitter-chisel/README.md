# tree-sitter-chisel

Tree-sitter grammar for the [Chisel](../pkg/chisel/LANGUAGE.md) SDF language.

## Features

- Full syntax highlighting for all Chisel constructs
- Code folding for blocks, loops, conditionals, and GLSL escapes
- Auto-indentation

## Usage

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

Use the tree-sitter VS Code extension with this grammar.
