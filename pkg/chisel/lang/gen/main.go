// Generator for Chisel editor tooling files.
// Reads the lang package (single source of truth) and produces:
//   - editors/chisel.tmbundle/Syntaxes/chisel.tmLanguage.json
//   - editors/tree-sitter-chisel/queries/highlights.scm
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

	tmPath := filepath.Join(editorsDir, "chisel.tmbundle", "Syntaxes", "chisel.tmLanguage.json")
	if err := generateTMLanguage(tmPath); err != nil {
		fmt.Fprintf(os.Stderr, "tmLanguage: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("generated %s\n", tmPath)

	hlPath := filepath.Join(editorsDir, "tree-sitter-chisel", "queries", "highlights.scm")
	if err := generateHighlights(hlPath); err != nil {
		fmt.Fprintf(os.Stderr, "highlights.scm: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("generated %s\n", hlPath)
}

func projectRoot() string {
	// Try go.mod detection first (works for `go run`)
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
	// Fallback: relative to source file
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
	// Collect names from lang registry
	var shapeNames []string
	for _, s := range lang.Shapes3D {
		shapeNames = append(shapeNames, s.Name)
	}
	for _, s := range lang.Shapes2D {
		shapeNames = append(shapeNames, s.Name)
	}

	var methodNames []string
	for _, m := range lang.Methods {
		if !m.IsColor { // color shorthands are highlighted as named-color, not method
			methodNames = append(methodNames, m.Name)
		}
	}

	funcNames := lang.FuncNames()
	colorNames := lang.ColorNames()

	var constNames []string
	for _, c := range lang.Constants {
		if !c.Vec { // vec constants (p, x, y, z) go to builtin-var
			constNames = append(constNames, c.Name)
		}
	}

	var builtinVarNames []string
	for _, c := range lang.Constants {
		if c.Vec {
			builtinVarNames = append(builtinVarNames, c.Name)
		}
	}
	// t is float but still a builtin var for highlighting
	builtinVarNames = append(builtinVarNames, "t")

	// Remove t from constNames (it's in builtinVarNames)
	filtered := constNames[:0]
	for _, n := range constNames {
		if n != "t" {
			filtered = append(filtered, n)
		}
	}
	constNames = filtered

	// Add true/false to constants
	constNames = append(constNames, "true", "false")

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
				Match: "\\b(" + joinNames(lang.Settings) + ")\\b",
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
				Patterns: []tmPattern{
					{Match: "\\|[~/]", Name: "keyword.operator.boolean.smooth.chisel"},
					{Match: "-[~/]", Name: "keyword.operator.boolean.smooth.chisel"},
					{Match: "&[~/]", Name: "keyword.operator.boolean.smooth.chisel"},
					{Match: "[|&]", Name: "keyword.operator.boolean.chisel"},
					{Match: "==|!=|<=|>=|<|>", Name: "keyword.operator.comparison.chisel"},
					{Match: "->", Name: "keyword.operator.arrow.chisel"},
					{Match: "\\.\\.", Name: "keyword.operator.range.chisel"},
				},
			},
			"number": {
				Match: "\\b\\d+(\\.\\d+)?([eE][+-]?\\d+)?\\b",
				Name:  "constant.numeric.chisel",
			},
			"hex-color": {
				Match: "#[0-9a-fA-F]{3,8}\\b",
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
	w("[\"" + strings.Join(lang.Settings, "\" \"") + "\"] @keyword")
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

	w("; ── CSG operators ───────────────────────────────────────────")
	w("[\"|\" \"&\"] @operator")
	w("[\"|~\" \"-~\" \"&~\" \"|/\" \"-/\" \"&/\"] @operator")
	w("")
	w("; ── Arithmetic / comparison operators ───────────────────────")
	w("[\"+\" \"-\" \"*\" \"/\" \"%\" \"!\"] @operator")
	w("[\"==\" \"!=\" \"<\" \">\" \"<=\" \">=\"] @operator")
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
