# Chisel Compiler — Implementation Plan

## Architecture Overview

```
                  ┌─────────────┐
  .chisel file →  │    Lexer    │ → tokens
                  └──────┬──────┘
                         ↓
                  ┌─────────────┐
                  │   Parser    │ → AST
                  └──────┬──────┘
                         ↓
                  ┌─────────────┐
                  │  Analyzer   │ → typed AST + errors
                  └──────┬──────┘
                         ↓
                  ┌─────────────┐
                  │  Code Gen   │ → GLSL
                  └─────────────┘
```

Each stage is a clean Go package, each produces a well-defined output, each can be tested independently.

---

## 1. Grammar Definition

### Approach: Hand-Written Recursive Descent

**Why not parser generators (yacc, ANTLR, PEG)?**

- Hand-written parsers produce **far better error messages** — we control every diagnostic
- Easier to implement **error recovery** (skip to next statement, keep parsing)
- No build-time dependency or generated code to maintain
- Go's standard library ecosystem is built this way (go/parser, go/scanner)
- Full control over AST shape — no adapter layer between generated tree and our types
- Easier to support **incremental/partial parsing** for editor integration

**Why not tree-sitter for the parser itself?**

Tree-sitter is a separate concern — it's for editor syntax highlighting, not for compilation. We'll generate a tree-sitter grammar FROM our language spec (see §6), not use it as our parser.

### Grammar Formalism

We document the grammar in EBNF for reference and testing, but the source of truth is the hand-written parser. The grammar is simple enough (no ambiguity except operator precedence, which Pratt parsing handles cleanly).

---

## 2. Lexer

### Token Types

```go
package chisel

type TokenKind int

const (
    // Literals
    TokInt          // 42
    TokFloat        // 3.14, 1e-5
    TokHexColor     // #ff0000, #f00
    TokString       // "hello"

    // Identifiers & Keywords
    TokIdent        // myVar, sphere, x, y, z
    TokFor          // for
    TokIn           // in
    TokIf           // if
    TokElse         // else
    TokStep         // step
    TokLight        // light
    TokCamera       // camera
    TokBg           // bg
    TokRaymarch     // raymarch
    TokPost         // post
    TokMat          // mat
    TokDebug        // debug
    TokGlsl         // glsl
    TokTrue         // true
    TokFalse        // false

    // Operators — boolean (SDF)
    TokPipe         // |
    TokPipeSmooth   // |~   (followed by number)
    TokPipeChamfer  // |/   (followed by number)
    TokMinus        // -
    TokMinusSmooth  // -~
    TokMinusChamfer // -/
    TokAmp          // &
    TokAmpSmooth    // &~
    TokAmpChamfer   // &/

    // Operators — arithmetic
    TokPlus         // +
    TokStar         // *
    TokSlash        // /
    TokPercent      // %

    // Comparison
    TokEq           // ==
    TokNeq          // !=
    TokLt           // <
    TokGt           // >
    TokLte          // <=
    TokGte          // >=

    // Punctuation
    TokLParen       // (
    TokRParen       // )
    TokLBrack       // [
    TokRBrack       // ]
    TokLBrace       // {
    TokRBrace       // }
    TokDot          // .
    TokComma        // ,
    TokColon        // :
    TokAssign       // =
    TokArrow        // ->
    TokDotDot       // ..

    // Special
    TokNewline      // significant for implicit union
    TokComment      // // ... or /* ... */
    TokEOF
)
```

### Token Structure

```go
type Token struct {
    Kind   TokenKind
    Value  string     // raw text
    Pos    Position   // source location
    Len    int        // byte length in source
}

type Position struct {
    File   string
    Line   int   // 1-based
    Col    int   // 1-based (byte offset in line)
    Offset int   // byte offset from start of file
}
```

### Newline Significance

Newlines matter for implicit union. The lexer inserts "logical newlines" after tokens that could end a shape expression (identifiers, `)`, `]`, `}`). This is the same approach Go uses for automatic semicolons.

```
sphere          \n    ← logical newline (could end expression)
  .at(2, 0, 0) \n    ← logical newline
box             \n    ← logical newline → triggers implicit union
```

Lines that end with an operator (`|`, `-`, `&`, `+`, `,`) suppress the newline — the expression continues.

---

## 3. Parser

### Pratt Parsing for Expressions

Pratt parsing (top-down operator precedence) is the cleanest way to handle the mixed arithmetic + boolean operator precedence:

```
Precedence (lowest → highest):
  1. |  |~  |/        union
  2. -  -~  -/        subtract
  3. &  &~  &/        intersect
  4. + -              arithmetic add/sub (- is context-dependent)
  5. * / %            arithmetic mul/div
  6. unary - !        negation
  7. .method()        method chain
  8. atoms            literals, calls, parens, blocks
```

### AST Node Types

```go
// Every node carries its source span for error reporting
type Node interface {
    Span() Span
}

type Span struct {
    Start Position
    End   Position
}

// Top-level program
type Program struct {
    Statements []Statement
}

// Statements
type Statement interface { Node }

type AssignStmt struct {
    Name   string
    Params []Param       // nil for variables, non-nil for functions
    Value  Expr
}

type SettingStmt struct {
    Kind  string          // "light", "camera", "bg", "raymarch", "post", "debug", "mat"
    Body  interface{}     // parsed into specific setting structs
}

// Expressions
type Expr interface { Node }

type NumberLit struct { Value float64 }
type BoolLit   struct { Value bool }
type VecLit    struct { Elems []Expr }
type HexColor  struct { R, G, B, A float64 }
type Ident     struct { Name string }

type BinaryExpr struct {
    Left     Expr
    Op       BinaryOp   // Union, SmoothUnion, Subtract, Add, Mul, etc.
    Right    Expr
    Blend    *float64   // smooth/chamfer radius (nil for sharp)
}

type UnaryExpr struct {
    Op      UnaryOp
    Operand Expr
}

type MethodCall struct {
    Receiver Expr
    Name     string
    Args     []Arg
}

type Swizzle struct {
    Receiver   Expr
    Components string   // "xz", "xy", "xxx", etc.
}

type FuncCall struct {
    Name string
    Args []Arg
}

type Arg struct {
    Name  string  // "" for positional
    Value Expr
}

type Block struct {
    Stmts []Statement
    Expr  Expr          // final expression (result)
}

type ForExpr struct {
    Iterators []Iterator
    Body      *Block
}

type Iterator struct {
    Name  string
    Start Expr
    End   Expr
    Step  Expr          // nil = default
}

type IfExpr struct {
    Cond Expr
    Then *Block
    Else Expr           // nil, *Block, or *IfExpr (else if)
}

type GlslEscape struct {
    Param string        // "p"
    Code  string        // raw GLSL source
}
```

### Error Recovery

The parser never panics. On error it:

1. Records a diagnostic with source span
2. Skips tokens until it finds a synchronization point (newline, `}`, `)`)
3. Continues parsing — one error shouldn't prevent finding others

```go
type Diagnostic struct {
    Severity  Severity     // Error, Warning, Hint
    Message   string       // human-readable
    Span      Span         // source location
    Help      string       // suggestion (optional)
    Labels    []Label      // additional annotated spans
}

type Label struct {
    Span    Span
    Message string
}
```

---

## 4. Error Messages

### Philosophy

Error messages are a **user interface**. They should:
1. Point to exactly where the problem is
2. Explain what went wrong in plain language
3. Suggest how to fix it
4. Show context (surrounding code)

Inspired by Elm, Rust, and Zig compiler errors.

### Error Format

```
error: unexpected token '+'
  ┌─ scene.chisel:3:8
  │
3 │ sphere + box
  │        ^ did you mean '|' for union?
  │
  = help: '+' is arithmetic only. Use '|' for combining shapes.
```

```
error: unknown shape 'sphree'
  ┌─ scene.chisel:1:1
  │
1 │ sphree
  │ ^^^^^^ unknown identifier
  │
  = help: did you mean 'sphere'?
```

```
error: 2D shape cannot be rendered directly
  ┌─ scene.chisel:5:3
  │
5 │   circle(2)
  │   ^^^^^^^^^ 'circle' returns a 2D shape
  │
  = help: use .extrude(height) to convert to 3D:
  =        circle(2).extrude(1)
```

### Fuzzy Matching

For unknown identifiers, compute edit distance against:
- All built-in shape names
- All built-in method names
- All variables in scope
- All function names in scope

Suggest the closest match if distance ≤ 2:

```go
func suggest(name string, candidates []string) string {
    best := ""
    bestDist := 3
    for _, c := range candidates {
        d := levenshtein(name, c)
        if d < bestDist {
            bestDist = d
            best = c
        }
    }
    return best
}
```

### Common Mistake Detection

Detect patterns that look like common errors and give targeted messages:

| User wrote | Likely meant | Message |
|---|---|---|
| `sphere + box` | `sphere \| box` | "'+' is arithmetic. Use '\|' for union." |
| `sphere.move(1,0,0)` | `sphere.at(1,0,0)` | "unknown method 'move'. Did you mean 'at'?" |
| `sphere.translate(1,0,0)` | `sphere.at(1,0,0)` | "unknown method 'translate'. Did you mean 'at'?" |
| `circle` (in scene) | `circle.extrude(1)` | "2D shape cannot be rendered. Use .extrude()." |
| `fn foo()` | `foo() =` | "use 'name() = expr' to define functions." |
| `let x = 1` | `x = 1` | "no 'let' needed. Just write 'x = 1'." |

---

## 5. Semantic Analysis

After parsing, the analyzer walks the AST and:

### Type Checking

Types in Chisel:
- `float` — scalar
- `vec2` — 2D vector
- `vec3` — 3D vector
- `sdf2d` — 2D distance field
- `sdf3d` — 3D distance field
- `material` — material definition
- `color` — RGB color value
- `signal` — time-varying value (compiles to a GLSL expression)

Type inference flows bottom-up:
- `sphere` → `sdf3d`
- `circle` → `sdf2d`
- `1.5` → `float`
- `[1, 2, 3]` → `vec3`
- `sdf3d | sdf3d` → `sdf3d`
- `sdf2d.extrude(float)` → `sdf3d`

### Validation Rules

- Scene root must be `sdf3d`
- Boolean ops require matching types (`sdf3d | sdf3d`, not `sdf3d | float`)
- `.extrude()` only on `sdf2d`
- `.at()`, `.rot()`, `.scale()` work on both `sdf2d` and `sdf3d`
- Arithmetic ops on scalars and vectors (with broadcasting)
- Function arity checks against definitions
- Variable defined before use
- No recursive function calls (would be infinite loop in GLSL)

### Name Resolution

Build a scope chain:
1. Built-ins (shapes, math functions, constants, `t`, `p`)
2. Top-level assignments
3. Function parameters
4. Block-local assignments
5. For-loop iterator variables

---

## 6. Tree-Sitter Grammar

### Why Tree-Sitter

Tree-sitter provides:
- **Syntax highlighting** in any editor (VS Code, Neovim, Helix, Zed)
- **Incremental parsing** — re-parse only changed regions
- **Structural selection** — select/move by AST node
- **Code folding** — collapse blocks
- **Bracket matching**

### Generation Strategy

We write the tree-sitter grammar (`grammar.js`) by hand, matching our parser's behavior. The grammar is declarative and maps directly to our EBNF:

```javascript
// tree-sitter-chisel/grammar.js
module.exports = grammar({
  name: 'chisel',

  extras: $ => [/\s/, $.comment],

  rules: {
    program: $ => repeat(choice(
      $.setting,
      $.assignment,
      $.expression
    )),

    comment: $ => choice(
      seq('//', /[^\n]*/),
      seq('/*', /[^*]*\*+([^/*][^*]*\*+)*/, '/')
    ),

    assignment: $ => seq(
      $.identifier,
      optional($.params),
      '=',
      $.expression
    ),

    expression: $ => choice(
      $.binary_expr,
      $.unary_expr,
      $.method_chain,
      $.atom
    ),

    binary_expr: $ => choice(
      prec.left(1, seq($.expression, '|', $.expression)),
      prec.left(1, seq($.expression, /\|~[\d.]+/, $.expression)),
      prec.left(2, seq($.expression, '-', $.expression)),
      prec.left(3, seq($.expression, '&', $.expression)),
      prec.left(4, seq($.expression, '+', $.expression)),
      prec.left(5, seq($.expression, '*', $.expression)),
    ),

    method_chain: $ => seq(
      $.atom,
      repeat1(seq('.', $.method_call))
    ),

    // ... etc
  }
});
```

### Repo Structure

```
tree-sitter-chisel/
  grammar.js          ← grammar definition
  src/
    parser.c          ← generated
    scanner.c         ← custom scanner (for GLSL blocks, newlines)
  queries/
    highlights.scm    ← syntax highlighting queries
    folds.scm         ← code folding
    indents.scm       ← auto-indent
  package.json
```

### Highlight Queries

```scheme
;; queries/highlights.scm
(comment) @comment
(number) @number
(hex_color) @constant
(string) @string

;; Keywords
["for" "in" "if" "else" "step"] @keyword
["light" "camera" "bg" "raymarch" "post" "mat" "debug" "glsl"] @keyword

;; Built-in shapes
((identifier) @function.builtin
  (#match? @function.builtin
    "^(sphere|box|cylinder|torus|capsule|cone|plane|octahedron|pyramid|ellipsoid|circle|rect|hexagon|polygon)$"))

;; Methods
(method_call name: (identifier) @method)

;; Operators
["|" "-" "&"] @operator
["|~" "-~" "&~" "|/" "-/"] @operator

;; Named colors
((identifier) @constant.builtin
  (#match? @constant.builtin
    "^(red|green|blue|white|black|yellow|cyan|magenta|gray|orange|purple|pink)$"))

;; Variables
(assignment name: (identifier) @variable)
(identifier) @variable
```

---

## 7. LSP (Language Server Protocol)

### Architecture

The LSP server is a separate binary that wraps the compiler stages:

```
Editor ←→ LSP Server ←→ Lexer/Parser/Analyzer
              ↕
         File Watcher
```

### Capabilities

**Phase 1 (from parser):**
- Diagnostics (errors/warnings) on save or keystroke
- Syntax highlighting (via tree-sitter or semantic tokens)
- Document symbols (list of variables, functions)
- Code folding
- Bracket matching

**Phase 2 (from analyzer):**
- Go to definition (variable/function)
- Hover info (type, documentation, default values)
- Autocomplete (shapes, methods, variables in scope, named params)
- Signature help (parameter hints for functions/methods)
- Rename symbol

**Phase 3 (advanced):**
- Live preview (compile + send GLSL to renderer)
- Color picker integration (for hex colors and RGB values)
- Inline parameter hints (show default values)
- Code actions ("Extract to variable", "Convert to function")

### Implementation

Use the `gopls`-style approach: the LSP server is a Go binary that imports the compiler packages:

```go
package main

import (
    "asciishader/pkg/chisel/lexer"
    "asciishader/pkg/chisel/parser"
    "asciishader/pkg/chisel/analyzer"
    "github.com/tliron/glsp"        // Go LSP framework
)

func (s *Server) TextDocumentDidChange(ctx context.Context, params *protocol.DidChangeTextDocumentParams) error {
    source := params.ContentChanges[0].Text
    tokens := lexer.Lex(source)
    ast, parseErrors := parser.Parse(tokens)
    typeErrors := analyzer.Check(ast)

    diagnostics := toDiagnostics(append(parseErrors, typeErrors...))
    s.publishDiagnostics(params.TextDocument.URI, diagnostics)
    return nil
}
```

---

## 8. Code Generation

### GLSL Output Structure

A Chisel program compiles to two GLSL functions that plug into the existing shader pipeline:

```glsl
// Generated from: sphere.red | box.blue.at(2, 0, 0)

float sceneSDF(vec3 p) {
    float d0 = sdSphere(p, 1.0);
    float d1 = sdBox(p - vec3(2.0, 0.0, 0.0), vec3(1.0));
    return opUnion(d0, d1);
}

vec3 sceneColor(vec3 p) {
    float d0 = sdSphere(p, 1.0);
    float d1 = sdBox(p - vec3(2.0, 0.0, 0.0), vec3(1.0));
    if (d0 < d1) return vec3(1.0, 0.0, 0.0);  // red
    return vec3(0.0, 0.0, 1.0);                 // blue
}
```

### Compilation Strategy

Walk the typed AST depth-first, emitting GLSL:

1. **Shapes** → call to GLSL SDF primitive function
2. **Transforms** → modify `p` before passing to inner SDF
   - `.at(x,y,z)` → `p - vec3(x,y,z)`
   - `.scale(s)` → `sdf(p/s) * s`
   - `.rot(deg, axis)` → `sdf(rotateAxis(p, deg))`
3. **Booleans** → `opUnion`, `opSmoothUnion`, `opSubtract`, etc.
4. **Variables** → GLSL local variables or inlined expressions
5. **Functions** → GLSL functions (chisel functions are pure, no state)
6. **For loops** → unrolled at compile time (GLSL has no dynamic loops over SDFs)
7. **Materials** → tracked alongside SDF for `sceneColor` generation

### Transform Stacking

Transforms compose by wrapping the evaluation point:

```chisel
sphere.at(2, 0, 0).scale(0.5).rot(45, y)
```

Compiles to (inside out):

```glsl
sdSphere(rotateY(((p - vec3(2,0,0)) / 0.5), radians(45.0)), 1.0) * 0.5
```

### Material Tracking

The color function needs to know which sub-SDF "won" at each point. For simple unions, compare distances. For smooth blends, interpolate colors:

```glsl
vec3 sceneColor(vec3 p) {
    float d0 = /* sdf for shape A */;
    float d1 = /* sdf for shape B */;
    vec3 c0 = /* color for A */;
    vec3 c1 = /* color for B */;

    // Sharp union: closest wins
    if (d0 < d1) return c0;
    return c1;

    // Smooth union: blend colors
    float h = clamp(0.5 + 0.5*(d1-d0)/k, 0.0, 1.0);
    return mix(c1, c0, h);
}
```

---

## 9. Formatter

### Goals

- Canonical formatting (one true style, like `gofmt`)
- Preserves comments
- Idempotent (formatting formatted code = no change)

### Rules

1. **Indent**: 2 spaces
2. **One shape per line** in blocks (when > 1 shape)
3. **Method chains**: align `.` under receiver if multi-line
4. **Operators**: spaces around `|`, `-`, `&`; no space in `|~0.3`
5. **Settings blocks**: one property per line, aligned colons
6. **Trailing newline**: always

### Example

Input (messy):
```
sphere.at(2,0,0).red|box.blue.at(  -2 ,0,0 )|cylinder(  0.5,3).orient(x)
```

Formatted:
```chisel
sphere.at(2, 0, 0).red
| box.blue.at(-2, 0, 0)
| cylinder(0.5, 3).orient(x)
```

---

## 10. Package Structure

```
pkg/chisel/
  LANGUAGE.md           ← language reference
  IMPLEMENTATION.md     ← this document

  token/
    token.go            ← TokenKind, Token, Position types

  lexer/
    lexer.go            ← Lex(source) → []Token
    lexer_test.go

  ast/
    ast.go              ← all AST node types

  parser/
    parser.go           ← Parse(tokens) → (AST, []Diagnostic)
    parser_test.go
    pratt.go            ← Pratt expression parser

  analyzer/
    analyzer.go         ← Check(AST) → []Diagnostic
    types.go            ← type system
    scope.go            ← scope chain / name resolution
    analyzer_test.go

  codegen/
    codegen.go          ← Generate(AST) → string (GLSL)
    codegen_test.go
    glsl.go             ← GLSL primitives and helpers

  format/
    format.go           ← Format(AST) → string
    format_test.go

  diagnostic/
    diagnostic.go       ← Diagnostic, Label, Severity types
    render.go           ← render diagnostics to terminal (with colors, underlines)

  chisel.go             ← public API: Compile(source) → (glsl, []Diagnostic)
```

---

## 11. Testing Strategy

### Unit Tests Per Stage

```go
// Lexer: source → tokens
func TestLexSimple(t *testing.T) {
    tokens := lexer.Lex("sphere | box.at(2, 0, 0)")
    expect := []TokenKind{TokIdent, TokPipe, TokIdent, TokDot, TokIdent, TokLParen, ...}
    ...
}

// Parser: tokens → AST
func TestParseUnion(t *testing.T) {
    ast := parse("sphere | box")
    assert(ast, BinaryExpr{Op: Union, Left: Ident{"sphere"}, Right: Ident{"box"}})
}

// Codegen: AST → GLSL
func TestCodegenSphere(t *testing.T) {
    glsl := compile("sphere")
    assertContains(glsl, "sdSphere(p, 1.0)")
}
```

### Golden File Tests

For each `.chisel` example, store expected `.glsl` output. CI diffs against golden files:

```
testdata/
  sphere.chisel           → sphere.glsl
  union.chisel            → union.glsl
  smooth_blend.chisel     → smooth_blend.glsl
  temple.chisel           → temple.glsl
  ...
```

### Error Message Tests

Golden file tests for error output too — errors are a UI, they shouldn't regress:

```
testdata/errors/
  typo.chisel             → typo.expected
  type_mismatch.chisel    → type_mismatch.expected
  missing_paren.chisel    → missing_paren.expected
```

### Fuzz Testing

Fuzz the lexer and parser with random input — they should never panic:

```go
func FuzzParse(f *testing.F) {
    f.Add("sphere | box")
    f.Add("sphere.at(2, 0, 0).red")
    f.Fuzz(func(t *testing.T, input string) {
        // Must not panic
        _, _ = parser.Parse(lexer.Lex(input))
    })
}
```

### Integration Tests

Compile Chisel → GLSL → load into GPU renderer → verify it renders without errors:

```go
func TestCompileAndRender(t *testing.T) {
    glsl, errs := chisel.Compile("sphere.red | box.blue.at(2, 0, 0)")
    assert(len(errs) == 0)
    err := gpu.CompileUserCode(glsl)
    assert(err == nil)
}
```

---

## 12. Build Phases

### Phase 1: Core Language
Lexer + parser + codegen for: shapes, transforms, booleans, variables.
One-line Chisel programs produce working GLSL.

### Phase 2: Full Expressions
Functions, blocks, for loops, conditionals, implicit union.
Multi-line programs with composition.

### Phase 3: Materials & Settings
Color, materials, lighting, camera, background, raymarching settings.
Complete scenes with full visual control.

### Phase 4: Advanced Features
Noise, displacement, morph, mirror, GLSL escape hatch.
Animation with `t`, easing, keyframes.

### Phase 5: Tooling
Formatter, error messages with fuzzy matching, diagnostic renderer.
Tree-sitter grammar, LSP server.

### Phase 6: Post-Processing & Debug
Post-processing pipeline, debug visualization modes.
