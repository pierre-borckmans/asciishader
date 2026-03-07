# Grammar Design: Single Source of Truth

## Decision

All editor tooling (tree-sitter grammar, TextMate grammar, LSP, syntax highlighting queries) is **generated** from a single source of truth: `pkg/chisel/lang/lang.go`.

The Go compiler's hand-written parser remains the canonical implementation. We do not use a formal grammar specification (PEG, ANTLR, etc.) to generate the parser.

## What `lang.go` defines

| Data | Used by |
|------|---------|
| `Shapes3D`, `Shapes2D` | Highlights, completions, analyzer |
| `Methods` | Highlights, completions, analyzer |
| `Functions` | Highlights, completions, analyzer |
| `Constants`, `Colors` | Highlights, completions |
| `Keywords`, `Settings` | Highlights, completions, parser, grammar |
| `Operators` (precedence table) | Grammar, parser |
| `UnaryOperators` | Grammar, parser |
| `Terminals` (regex patterns) | Grammar, tmLanguage, lexer |

## What gets generated

Running `go generate ./pkg/chisel/lang/` produces:

| File | Description |
|------|-------------|
| `editors/tree-sitter-chisel/grammar.js` | Full tree-sitter grammar |
| `editors/tree-sitter-chisel/queries/highlights.scm` | Syntax highlighting queries |
| `editors/tree-sitter-chisel/queries/folds.scm` | Code folding rules |
| `editors/tree-sitter-chisel/queries/indents.scm` | Auto-indentation rules |
| `editors/chisel.tmbundle/Syntaxes/chisel.tmLanguage.json` | TextMate grammar |

## Adding a new language feature

**New shape, function, method, or constant**: Add it to `lang.go`, run `go generate`. All editors pick it up automatically.

**New operator**: Add it to `lang.Operators` with its precedence. Update the parser's `infixPrecedence()` to match. Run `go generate`. The tree-sitter grammar, tmLanguage, and highlights all update.

**New keyword or settings block**: Add to `lang.Keywords` or `lang.Settings` (with its `SettingForm`). Update the parser. Run `go generate`.

**New syntax construct** (e.g., a `match` expression): Update the parser. Update the structural template in `gen/main.go` to emit the new grammar rule. The template code is explicit and readable.

## Alternatives considered

### Formal grammar (PEG/EBNF) as the single source

Write a canonical grammar file and generate everything from it, including the Go parser.

**Rejected because:**
- Parser generators produce poor error messages. Chisel's hand-written parser has rich diagnostics with "did you mean?" suggestions, fuzzy matching, and context-aware recovery. These are critical for a DSL aimed at creative users, not compiler engineers.
- Build-time complexity: parser generators add a code generation step and a dependency on a specific tool.
- Go's ecosystem convention: go/parser, go/scanner, and most Go language tools use hand-written parsers.
- The grammar is small (~20 rules). The overhead of a formal specification exceeds the duplication risk.

### Ungrammar (rust-analyzer approach)

Define the tree shape in a `.ungram` file, generate AST types and syntax kinds, but hand-write parsing logic.

**Rejected because:**
- Ungrammar defines the tree shape, not the parsing rules. We'd still duplicate operator precedence and statement structure.
- The Chisel AST is small and stable (~15 node types). Code-generating AST types adds complexity without proportional benefit.
- Worth reconsidering if the language grows significantly in complexity.

### Tree-sitter as the compiler's parser

Use tree-sitter's C parser (via CGo or WASM) as the single parser for both compilation and editing.

**Rejected because:**
- Tree-sitter produces a CST, not an AST. A conversion layer would be needed.
- Tree-sitter's error recovery is designed for editors (insert ERROR nodes, keep parsing). Compiler diagnostics need specific error messages, not just "syntax error here."
- CGo or WASM binding adds deployment complexity to the CLI tool.
- Tree-sitter and compilers want fundamentally different things from the same grammar: editors want tolerance, compilers want precision.

## Why this approach works

The key insight: **what changes** in a DSL grammar falls into two categories:

1. **Vocabulary** (shapes, functions, methods, colors, constants) — changes frequently as the language grows. Fully data-driven in `lang.go`.

2. **Syntax structure** (how blocks, assignments, control flow, and operator precedence work) — changes rarely. Defined in the parser and mirrored in the generator template.

Category 1 is where drift bugs live. Category 2 is stable enough that template duplication is acceptable. The operator precedence table bridges both: it's data-driven in `lang.go` and consumed by both the parser (via constants) and the generator (via the `Operators` slice).
