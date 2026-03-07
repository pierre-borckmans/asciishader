// Generator for Chisel editor tooling files.
// Reads the lang package (single source of truth) and produces:
//   - editors/tree-sitter-chisel/grammar.js
//   - editors/tree-sitter-chisel/queries/highlights.scm
//   - editors/tree-sitter-chisel/queries/folds.scm
//   - editors/tree-sitter-chisel/queries/indents.scm
//   - editors/chisel.tmbundle/Syntaxes/chisel.tmLanguage.json
//
// Run: go generate ./pkg/chisel/lang/
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"asciishader/pkg/chisel/lang"
)

func main() {
	root := projectRoot()
	editorsDir := filepath.Join(root, "pkg", "chisel", "editors")

	generators := []struct {
		name string
		path string
		fn   func(string) error
	}{
		{"grammar.js", filepath.Join(editorsDir, "tree-sitter-chisel", "grammar.js"), generateGrammarJS},
		{"highlights.scm", filepath.Join(editorsDir, "tree-sitter-chisel", "queries", "highlights.scm"), generateHighlights},
		{"folds.scm", filepath.Join(editorsDir, "tree-sitter-chisel", "queries", "folds.scm"), generateFolds},
		{"indents.scm", filepath.Join(editorsDir, "tree-sitter-chisel", "queries", "indents.scm"), generateIndents},
		{"tmLanguage", filepath.Join(editorsDir, "chisel.tmbundle", "Syntaxes", "chisel.tmLanguage.json"), generateTMLanguage},
		{"tmPreferences", filepath.Join(editorsDir, "chisel.tmbundle", "Preferences", "chisel.tmPreferences"), generateTMPreferences},
		{"language-configuration", filepath.Join(editorsDir, "chisel.tmbundle", "language-configuration.json"), generateLanguageConfiguration},
	}

	for _, g := range generators {
		if err := g.fn(g.path); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", g.name, err)
			os.Exit(1)
		}
		fmt.Printf("generated %s\n", g.path)
	}
}

func projectRoot() string {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	_, file, _, _ := runtime.Caller(0)
	dir = filepath.Dir(file)
	for i := 0; i < 5; i++ {
		dir = filepath.Dir(dir)
	}
	return dir
}

func joinNames(items []string) string {
	return strings.Join(items, "|")
}

// ---------------------------------------------------------------------------
// grammar.js
// ---------------------------------------------------------------------------

func generateGrammarJS(path string) error {
	var b strings.Builder
	w := func(s string) { b.WriteString(s); b.WriteByte('\n') }

	w("/// <reference types=\"tree-sitter-cli/dsl\" />")
	w("// @ts-check")
	w("//")
	w("// Generated from pkg/chisel/lang/lang.go")
	w("// Do not edit — run: go generate ./pkg/chisel/lang/")
	w("")
	w("module.exports = grammar({")
	w("  name: 'chisel',")
	w("")
	w("  extras: $ => [/\\s/, $.comment],")
	w("")
	w("  word: $ => $.identifier,")
	w("")
	w("  conflicts: $ => [")
	w("    [$.settings_block, $.block],")
	w("    [$.settings_entry, $.primary_expression],")
	w("    [$.param, $.primary_expression],")
	w("  ],")
	w("")
	w("  rules: {")

	// ── program ──────────────────────────────────────────────
	w("    program: $ => repeat(choice(")
	w("      $.setting,")
	w("      $.assignment,")
	w("      $.expression,")
	w("    )),")
	w("")

	// ── comments ─────────────────────────────────────────────
	w("    // ── Comments ──────────────────────────────────────────────")
	w("    comment: $ => choice(")
	w("      seq('//', /[^\\n]*/),")
	w("      seq('/*', /[^*]*\\*+([^/*][^*]*\\*+)*/, '/'),")
	w("    ),")
	w("")

	// ── settings ─────────────────────────────────────────────
	w("    // ── Settings blocks ──────────────────────────────────────")
	w("    setting: $ => choice(")
	// Generate settings from lang.Settings (form is encoded in SettingDef)
	for _, s := range lang.Settings {
		switch s.Form {
		case lang.SettingCamera:
			w("      seq('" + s.Name + "', choice($.camera_shorthand, $.settings_block)),")
		case lang.SettingDebug:
			w("      seq('" + s.Name + "', $.identifier),")
		case lang.SettingMat:
			w("      seq('" + s.Name + "', $.identifier, '=', $.settings_block),")
		case lang.SettingBlockOnly:
			w("      seq('" + s.Name + "', $.settings_block),")
		case lang.SettingExprOrBlock:
			w("      seq('" + s.Name + "', choice($.expression, $.settings_block)),")
		}
	}
	w("    ),")
	w("")
	w("    camera_shorthand: $ => seq($.expression, '->', $.expression),")
	w("")
	w("    settings_block: $ => seq('{', commaSep($.settings_entry), '}'),")
	w("")
	w("    settings_entry: $ => choice(")
	w("      seq($.identifier, ':', $.expression),")
	w("      seq($.identifier, $.settings_block),")
	w("    ),")
	w("")

	// ── assignments ──────────────────────────────────────────
	w("    // ── Assignments ──────────────────────────────────────────")
	w("    assignment: $ => seq(")
	w("      $.identifier,")
	w("      optional($.params),")
	w("      '=',")
	w("      $.expression,")
	w("    ),")
	w("")
	w("    params: $ => seq('(', commaSep($.param), ')'),")
	w("")
	w("    param: $ => seq($.identifier, optional(seq('=', $.expression))),")
	w("")

	// ── expressions ──────────────────────────────────────────
	w("    // ── Expressions (with precedence) ────────────────────────")
	w("    expression: $ => choice(")
	w("      $.binary_expression,")
	w("      $.unary_expression,")
	w("      $.method_chain,")
	w("      $.primary_expression,")
	w("    ),")
	w("")

	// ── binary_expression (generated from operator table) ────
	w("    binary_expression: $ => choice(")
	for _, op := range lang.Operators {
		assoc := "prec.left"
		if op.Assoc == lang.Right {
			assoc = "prec.right"
		}
		tok := jsString(op.Token)
		if op.Blend {
			// Smooth/chamfer operators: op [optional_blend_radius] rhs
			fmt.Fprintf(&b, "      %s(%d, seq($.expression, %s, optional($.expression), $.expression)),\n",
				assoc, op.Prec, tok)
		} else {
			fmt.Fprintf(&b, "      %s(%d, seq($.expression, %s, $.expression)),\n",
				assoc, op.Prec, tok)
		}
	}
	w("    ),")
	w("")

	// ── unary_expression ─────────────────────────────────────
	w("    unary_expression: $ => choice(")
	for _, op := range lang.UnaryOperators {
		fmt.Fprintf(&b, "      prec(%d, seq(%s, $.expression)),\n", op.Prec, jsString(op.Token))
	}
	w("    ),")
	w("")

	// ── method_chain ─────────────────────────────────────────
	fmt.Fprintf(&b, "    method_chain: $ => prec.left(%d, seq(\n", lang.PrecPostfix)
	w("      $.primary_expression,")
	w("      repeat1(seq('.', choice($.method_call, $.swizzle))),")
	w("    )),")
	w("")
	w("    method_call: $ => choice(")
	w("      prec(1, seq($.identifier, '(', commaSep($.argument), ')')),")
	w("      $.identifier,")
	w("    ),")
	w("")
	w("    swizzle: $ => /[xyzrgb]{1,4}/,")
	w("")
	w("    argument: $ => choice(")
	w("      seq($.identifier, ':', $.expression),   // named argument")
	w("      $.expression,                            // positional argument")
	w("    ),")
	w("")

	// ── primary_expression ───────────────────────────────────
	w("    // ── Primary expressions ──────────────────────────────────")
	w("    primary_expression: $ => choice(")
	w("      $.number,")
	w("      $.boolean,")
	w("      $.string,")
	w("      $.hex_color,")
	w("      $.vector,")
	w("      $.function_call,")
	w("      $.identifier,")
	w("      seq('(', $.expression, ')'),")
	w("      $.block,")
	w("      $.for_expression,")
	w("      $.if_expression,")
	w("      $.glsl_escape,")
	w("    ),")
	w("")
	w("    function_call: $ => prec(1, seq(")
	w("      $.identifier,")
	w("      '(',")
	w("      commaSep($.argument),")
	w("      ')',")
	w("    )),")
	w("")
	w("    block: $ => seq('{', repeat(choice($.assignment, $.expression)), '}'),")
	w("")
	w("    vector: $ => seq('[', commaSep1($.expression), ']'),")
	w("")

	// ── control flow ─────────────────────────────────────────
	w("    // ── Control flow ─────────────────────────────────────────")
	w("    for_expression: $ => seq(")
	w("      'for',")
	w("      commaSep1($.iterator),")
	w("      $.block,")
	w("    ),")
	w("")
	w("    iterator: $ => seq(")
	w("      $.identifier,")
	w("      'in',")
	w("      $.expression,")
	w("      '..',")
	w("      $.expression,")
	w("      optional(seq('step', $.expression)),")
	w("    ),")
	w("")
	w("    if_expression: $ => seq(")
	w("      'if',")
	w("      $.expression,")
	w("      $.block,")
	w("      optional(seq('else', choice($.if_expression, $.block))),")
	w("    ),")
	w("")

	// ── GLSL escape ──────────────────────────────────────────
	w("    // ── GLSL escape ──────────────────────────────────────────")
	w("    glsl_escape: $ => seq(")
	w("      'glsl',")
	w("      '(',")
	w("      $.identifier,")
	w("      ')',")
	w("      $.glsl_body,")
	w("    ),")
	w("")
	// GLSL body matches balanced braces up to 2 levels of nesting.
	// Deeper nesting requires a tree-sitter external scanner.
	w("    glsl_body: $ => seq('{', /([^{}]|\\{([^{}]|\\{[^{}]*\\})*\\})*/, '}'),")
	w("")

	// ── terminals ────────────────────────────────────────────
	w("    // ── Terminals ────────────────────────────────────────────")
	w("    number: $ => {")
	w("      const decimal_digits = /\\d+/;")
	w("      const decimal_point = /\\.\\d+/;")
	w("      const exponent = /[eE][+-]?\\d+/;")
	w("      return token(choice(")
	w("        seq(decimal_digits, decimal_point, optional(exponent)),  // 1.5, 1.5e10")
	w("        seq(decimal_digits, exponent),                           // 1e10")
	w("        decimal_digits,                                          // 42")
	w("      ));")
	w("    },")
	w("")
	w("    boolean: $ => choice('true', 'false'),")
	w("")
	w("    string: $ => choice(")
	w("      seq('\"', /[^\"]*/, '\"'),")
	w("      seq(\"'\", /[^']*/, \"'\"),")
	w("    ),")
	w("")
	fmt.Fprintf(&b, "    hex_color: $ => token(seq('#', /[0-9a-fA-F]{3,8}/)),\n")
	w("")
	fmt.Fprintf(&b, "    identifier: $ => /%s/,\n", lang.Terminals.Ident)
	w("  },")
	w("});")
	w("")

	// ── helper functions ─────────────────────────────────────
	w("/**")
	w(" * Comma-separated list (zero or more).")
	w(" */")
	w("function commaSep(rule) {")
	w("  return optional(commaSep1(rule));")
	w("}")
	w("")
	w("/**")
	w(" * Comma-separated list (one or more).")
	w(" */")
	w("function commaSep1(rule) {")
	w("  return seq(rule, repeat(seq(',', rule)));")
	w("}")
	w("")

	return os.WriteFile(path, []byte(b.String()), 0644)
}

// jsString returns a JavaScript string literal for a tree-sitter token.
func jsString(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "\\'") + "'"
}

// ---------------------------------------------------------------------------
// highlights.scm
// ---------------------------------------------------------------------------

func generateHighlights(path string) error {
	var shapeNames3D, shapeNames2D []string
	for _, s := range lang.Shapes3D {
		shapeNames3D = append(shapeNames3D, s.Name)
	}
	for _, s := range lang.Shapes2D {
		shapeNames2D = append(shapeNames2D, s.Name)
	}

	var constNames []string
	for _, c := range lang.Constants {
		if !c.Vec && c.Name != "t" {
			constNames = append(constNames, c.Name)
		}
	}

	var b strings.Builder
	w := func(s string) { b.WriteString(s); b.WriteByte('\n') }

	w("; ── Generated from pkg/chisel/lang/lang.go ──────────────────")
	w("; Do not edit — run: go generate ./pkg/chisel/lang/")
	w("")
	w("; ── Comments ────────────────────────────────────────────────")
	w("(comment) @comment")
	w("")
	w("; ── Literals ────────────────────────────────────────────────")
	w("(number) @number")
	w("(hex_color) @constant")
	w("(string) @string")
	w("(boolean) @constant.builtin")
	w("")
	w("; ── Keywords ────────────────────────────────────────────────")
	w("[\"" + strings.Join(lang.Keywords, "\" \"") + "\"] @keyword")
	w("[\"" + strings.Join(lang.SettingNames(), "\" \"") + "\"] @keyword")
	w("")

	w("; ── Built-in shapes ─────────────────────────────────────────")
	w("((identifier) @function.builtin")
	w("  (#match? @function.builtin \"^(" + joinNames(shapeNames3D) + ")$\"))")
	w("")
	w("((identifier) @function.builtin")
	w("  (#match? @function.builtin \"^(" + joinNames(shapeNames2D) + ")$\"))")
	w("")

	w("; ── Built-in functions ─────────────────────────────────────")
	w("((identifier) @function.builtin")
	w("  (#match? @function.builtin \"^(" + joinNames(lang.FuncNames()) + ")$\"))")
	w("")

	w("; ── Method calls ────────────────────────────────────────────")
	w("(method_call (identifier) @method)")
	w("")
	w("; ── Swizzle ─────────────────────────────────────────────────")
	w("(swizzle) @property")
	w("")

	w("; ── Named colors ────────────────────────────────────────────")
	w("((identifier) @constant.builtin")
	w("  (#match? @constant.builtin \"^(" + joinNames(lang.ColorNames()) + ")$\"))")
	w("")

	w("; ── Constants ───────────────────────────────────────────────")
	w("((identifier) @constant.builtin")
	w("  (#match? @constant.builtin \"^(" + joinNames(constNames) + ")$\"))")
	w("")

	w("; ── Built-in variables ──────────────────────────────────────")
	w("((identifier) @variable.builtin")
	w("  (#match? @variable.builtin \"^(t|p)$\"))")
	w("")

	// Generate CSG operator highlights from lang.Operators
	var sharpCSG, blendCSG []string
	var arithOps, cmpOps []string
	for _, op := range lang.Operators {
		switch {
		case op.Blend:
			blendCSG = append(blendCSG, "\""+op.Token+"\"")
		case op.Token == "|" || op.Token == "&":
			sharpCSG = append(sharpCSG, "\""+op.Token+"\"")
		case op.Token == "+" || op.Token == "-" || op.Token == "*" || op.Token == "/" || op.Token == "%":
			arithOps = append(arithOps, "\""+op.Token+"\"")
		case op.Prec == lang.PrecCompare:
			cmpOps = append(cmpOps, "\""+op.Token+"\"")
		}
	}

	w("; ── CSG operators ───────────────────────────────────────────")
	w("[" + strings.Join(sharpCSG, " ") + "] @operator")
	w("[" + strings.Join(blendCSG, " ") + "] @operator")
	w("")
	w("; ── Arithmetic / comparison operators ───────────────────────")
	// Add unary operators
	for _, u := range lang.UnaryOperators {
		found := false
		for _, a := range arithOps {
			if a == "\""+u.Token+"\"" {
				found = true
				break
			}
		}
		if !found {
			arithOps = append(arithOps, "\""+u.Token+"\"")
		}
	}
	w("[" + strings.Join(arithOps, " ") + "] @operator")
	w("[" + strings.Join(cmpOps, " ") + "] @operator")
	w("[\"=\"] @operator")
	w("")
	w("; ── Punctuation ─────────────────────────────────────────────")
	w("[\"(\" \")\" \"[\" \"]\" \"{\" \"}\"] @punctuation.bracket")
	w("[\".\" \",\" \":\" \"..\" \"->\"] @punctuation.delimiter")
	w("")
	w("; ── Assignment target ───────────────────────────────────────")
	w("(assignment (identifier) @variable)")
	w("")
	w("; ── Function definition (assignment with params) ────────────")
	w("(assignment (identifier) @function (params))")
	w("")
	w("; ── Parameter names ─────────────────────────────────────────")
	w("(param (identifier) @variable.parameter)")
	w("")
	w("; ── Settings keys ───────────────────────────────────────────")
	w("(settings_entry (identifier) @property)")

	return os.WriteFile(path, []byte(b.String()), 0644)
}

// ---------------------------------------------------------------------------
// folds.scm
// ---------------------------------------------------------------------------

func generateFolds(path string) error {
	var b strings.Builder
	w := func(s string) { b.WriteString(s); b.WriteByte('\n') }

	w("; Generated from pkg/chisel/lang/lang.go")
	w("; Do not edit — run: go generate ./pkg/chisel/lang/")
	w("(block) @fold")
	w("(settings_block) @fold")
	w("(for_expression) @fold")
	w("(if_expression) @fold")
	w("(glsl_body) @fold")

	return os.WriteFile(path, []byte(b.String()), 0644)
}

// ---------------------------------------------------------------------------
// indents.scm
// ---------------------------------------------------------------------------

func generateIndents(path string) error {
	var b strings.Builder
	w := func(s string) { b.WriteString(s); b.WriteByte('\n') }

	w("; Generated from pkg/chisel/lang/lang.go")
	w("; Do not edit — run: go generate ./pkg/chisel/lang/")
	w("")
	w("(block \"{\" @indent)")
	w("(block \"}\" @outdent)")
	w("")
	w("(settings_block \"{\" @indent)")
	w("(settings_block \"}\" @outdent)")
	w("")
	w("(for_expression (block \"{\" @indent))")
	w("(for_expression (block \"}\" @outdent))")
	w("")
	w("(if_expression (block \"{\" @indent))")
	w("(if_expression (block \"}\" @outdent))")
	w("")
	w("(glsl_body \"{\" @indent)")
	w("(glsl_body \"}\" @outdent)")

	return os.WriteFile(path, []byte(b.String()), 0644)
}

// ---------------------------------------------------------------------------
// tmLanguage
// ---------------------------------------------------------------------------

type tmGrammar struct {
	Name       string                `json:"name"`
	ScopeName  string                `json:"scopeName"`
	FileTypes  []string              `json:"fileTypes"`
	Patterns   []tmPattern           `json:"patterns"`
	Repository map[string]tmRepoRule `json:"repository"`
}

type tmPattern struct {
	Include string `json:"include,omitempty"`
	Match   string `json:"match,omitempty"`
	Name    string `json:"name,omitempty"`
	Begin   string `json:"begin,omitempty"`
	End     string `json:"end,omitempty"`
}

type tmRepoRule struct {
	Match    string      `json:"match,omitempty"`
	Name     string      `json:"name,omitempty"`
	Begin    string      `json:"begin,omitempty"`
	End      string      `json:"end,omitempty"`
	Patterns []tmPattern `json:"patterns,omitempty"`
}

func generateTMLanguage(path string) error {
	var shapeNames []string
	for _, s := range lang.Shapes3D {
		shapeNames = append(shapeNames, s.Name)
	}
	for _, s := range lang.Shapes2D {
		shapeNames = append(shapeNames, s.Name)
	}

	var methodNames []string
	for _, m := range lang.Methods {
		if !m.IsColor {
			methodNames = append(methodNames, m.Name)
		}
	}

	funcNames := lang.FuncNames()
	colorNames := lang.ColorNames()

	var constNames []string
	for _, c := range lang.Constants {
		if !c.Vec {
			constNames = append(constNames, c.Name)
		}
	}

	var builtinVarNames []string
	for _, c := range lang.Constants {
		if c.Vec {
			builtinVarNames = append(builtinVarNames, c.Name)
		}
	}
	builtinVarNames = append(builtinVarNames, "t")

	filtered := constNames[:0]
	for _, n := range constNames {
		if n != "t" {
			filtered = append(filtered, n)
		}
	}
	constNames = filtered
	constNames = append(constNames, "true", "false")

	// Build smooth/chamfer operator patterns from lang.Operators.
	// Group by base character (|, -, &) and collect suffixes (~, /).
	type baseGroup struct {
		base     string
		suffixes map[byte]bool
	}
	groupOrder := []string{}
	groups := map[string]*baseGroup{}
	for _, op := range lang.Operators {
		if !op.Blend {
			continue
		}
		base := string(op.Token[0])
		suffix := op.Token[1]
		g, ok := groups[base]
		if !ok {
			g = &baseGroup{base: base, suffixes: map[byte]bool{}}
			groups[base] = g
			groupOrder = append(groupOrder, base)
		}
		g.suffixes[suffix] = true
	}
	var smoothOps []tmPattern
	for _, base := range groupOrder {
		g := groups[base]
		baseEsc := base
		if base == "|" {
			baseEsc = "\\|"
		}
		// Stable suffix order: ~ before /
		var suffixChars string
		if g.suffixes['~'] {
			suffixChars += "~"
		}
		if g.suffixes['/'] {
			suffixChars += "/"
		}
		smoothOps = append(smoothOps, tmPattern{
			Match: baseEsc + "[" + suffixChars + "]",
			Name:  "keyword.operator.boolean.smooth.chisel",
		})
	}

	grammar := tmGrammar{
		Name:      "Chisel",
		ScopeName: "source.chisel",
		FileTypes: []string{"chisel"},
		Patterns: []tmPattern{
			{Include: "#comment"},
			{Include: "#keyword"},
			{Include: "#setting"},
			{Include: "#shape"},
			{Include: "#builtin-function"},
			{Include: "#named-color"},
			{Include: "#constant"},
			{Include: "#builtin-var"},
			{Include: "#operator"},
			{Include: "#number"},
			{Include: "#hex-color"},
			{Include: "#string"},
			{Include: "#method"},
			{Include: "#punctuation"},
		},
		Repository: map[string]tmRepoRule{
			"comment": {
				Patterns: []tmPattern{
					{Match: "//.*$", Name: "comment.line.double-slash.chisel"},
					{Begin: "/\\*", End: "\\*/", Name: "comment.block.chisel"},
				},
			},
			"keyword": {
				Match: "\\b(" + joinNames(lang.Keywords) + ")\\b",
				Name:  "keyword.control.chisel",
			},
			"setting": {
				Match: "\\b(" + joinNames(lang.SettingNames()) + ")\\b",
				Name:  "keyword.other.chisel",
			},
			"shape": {
				Match: "\\b(" + joinNames(shapeNames) + ")\\b",
				Name:  "support.function.shape.chisel",
			},
			"builtin-function": {
				Match: "\\b(" + joinNames(funcNames) + ")\\b",
				Name:  "support.function.builtin.chisel",
			},
			"named-color": {
				Match: "\\b(" + joinNames(colorNames) + ")\\b",
				Name:  "constant.language.color.chisel",
			},
			"constant": {
				Match: "\\b(" + joinNames(constNames) + ")\\b",
				Name:  "constant.language.chisel",
			},
			"builtin-var": {
				Match: "(?<![.a-zA-Z_])\\b(" + joinNames(builtinVarNames) + ")\\b",
				Name:  "variable.language.chisel",
			},
			"operator": {
				Patterns: append(smoothOps,
					tmPattern{Match: "[|&]", Name: "keyword.operator.boolean.chisel"},
					tmPattern{Match: "==|!=|<=|>=|<|>", Name: "keyword.operator.comparison.chisel"},
					tmPattern{Match: "->", Name: "keyword.operator.arrow.chisel"},
					tmPattern{Match: "\\.\\.", Name: "keyword.operator.range.chisel"},
				),
			},
			"number": {
				Match: "\\b" + lang.Terminals.Number + "\\b",
				Name:  "constant.numeric.chisel",
			},
			"hex-color": {
				Match: lang.Terminals.HexColor + "\\b",
				Name:  "constant.other.color.chisel",
			},
			"string": {
				Patterns: []tmPattern{
					{Begin: "\"", End: "\"", Name: "string.quoted.double.chisel"},
					{Begin: "'", End: "'", Name: "string.quoted.single.chisel"},
				},
			},
			"method": {
				Match: "(?<=\\.)(" + joinNames(methodNames) + ")\\b",
				Name:  "entity.name.function.method.chisel",
			},
			"punctuation": {
				Patterns: []tmPattern{
					{Match: "[()\\[\\]{}]", Name: "punctuation.bracket.chisel"},
					{Match: "[.,:]", Name: "punctuation.delimiter.chisel"},
				},
			},
		},
	}

	out, err := json.MarshalIndent(grammar, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	return os.WriteFile(path, out, 0644)
}

// ---------------------------------------------------------------------------
// tmPreferences (comment toggling for TextMate/IntelliJ/Sublime)
// ---------------------------------------------------------------------------

func generateTMPreferences(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	content := `<?xml version="1.0" encoding="UTF-8"?>
<!--
  Generated from pkg/chisel/lang/lang.go
  Do not edit — run: go generate ./pkg/chisel/lang/
-->
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>name</key>
    <string>Comments and Brackets</string>
    <key>scope</key>
    <string>source.chisel</string>
    <key>settings</key>
    <dict>
        <key>shellVariables</key>
        <array>
            <dict>
                <key>name</key>
                <string>TM_COMMENT_START</string>
                <key>value</key>
                <string>// </string>
            </dict>
            <dict>
                <key>name</key>
                <string>TM_COMMENT_START_2</string>
                <key>value</key>
                <string>/* </string>
            </dict>
            <dict>
                <key>name</key>
                <string>TM_COMMENT_END_2</string>
                <key>value</key>
                <string> */</string>
            </dict>
        </array>
        <key>foldingStartMarker</key>
        <string>\{\s*$</string>
        <key>foldingStopMarker</key>
        <string>^\s*\}</string>
    </dict>
    <key>uuid</key>
    <string>chisel-comments-and-brackets</string>
</dict>
</plist>
`
	return os.WriteFile(path, []byte(content), 0644)
}

// ---------------------------------------------------------------------------
// language-configuration.json (VS Code / editors that support it)
// ---------------------------------------------------------------------------

func generateLanguageConfiguration(path string) error {
	type commentRule struct {
		LineComment  string   `json:"lineComment"`
		BlockComment [2]string `json:"blockComment"`
	}
	type bracketPair = [2]string
	type autoClosePair struct {
		Open  string `json:"open"`
		Close string `json:"close"`
	}
	type indentRules struct {
		IncreaseIndent string `json:"increaseIndentPattern"`
		DecreaseIndent string `json:"decreaseIndentPattern"`
	}
	type foldingMarkers struct {
		Start string `json:"start"`
		End   string `json:"end"`
	}
	type foldingRules struct {
		OffSide bool           `json:"offSide,omitempty"`
		Markers foldingMarkers `json:"markers"`
	}
	type langConfig struct {
		Comments          commentRule     `json:"comments"`
		Brackets          []bracketPair   `json:"brackets"`
		AutoClosingPairs  []autoClosePair `json:"autoClosingPairs"`
		SurroundingPairs  []autoClosePair `json:"surroundingPairs"`
		Folding           foldingRules    `json:"folding"`
		IndentationRules  indentRules     `json:"indentationRules"`
		WordPattern       string          `json:"wordPattern,omitempty"`
	}

	cfg := langConfig{
		Comments: commentRule{
			LineComment:  "//",
			BlockComment: [2]string{"/*", "*/"},
		},
		Brackets: []bracketPair{
			{"{", "}"},
			{"[", "]"},
			{"(", ")"},
		},
		AutoClosingPairs: []autoClosePair{
			{"{", "}"},
			{"[", "]"},
			{"(", ")"},
			{"\"", "\""},
			{"'", "'"},
			{"/*", "*/"},
		},
		SurroundingPairs: []autoClosePair{
			{"{", "}"},
			{"[", "]"},
			{"(", ")"},
			{"\"", "\""},
			{"'", "'"},
		},
		Folding: foldingRules{
			Markers: foldingMarkers{
				Start: `\{|\(/\*`,
				End:   `\}|\*/)`,
			},
		},
		IndentationRules: indentRules{
			IncreaseIndent: `\{[^}]*$`,
			DecreaseIndent: `^\s*\}`,
		},
	}

	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	// Prepend a comment (JSON doesn't support comments, but VS Code does in jsonc)
	header := "// Generated from pkg/chisel/lang/lang.go\n// Do not edit — run: go generate ./pkg/chisel/lang/\n"
	out = append([]byte(header), out...)
	out = append(out, '\n')
	return os.WriteFile(path, out, 0644)
}
