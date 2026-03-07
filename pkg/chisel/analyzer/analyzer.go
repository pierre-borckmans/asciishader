package analyzer

import (
	"fmt"
	"strings"

	"asciishader/pkg/chisel/ast"
	"asciishader/pkg/chisel/diagnostic"
	"asciishader/pkg/chisel/lang"
	"asciishader/pkg/chisel/token"
)

// knownMethods is derived from the lang registry.
var knownMethods = lang.MethodNames()

// methodAliases is derived from the lang registry.
var methodAliases = lang.MethodAliases

// shapeArity is derived from the lang registry.
var shapeArity = buildShapeArity()

func buildShapeArity() map[string]struct{ min, max int } {
	m := make(map[string]struct{ min, max int })
	for _, s := range lang.Shapes3D {
		m[s.Name] = struct{ min, max int }{s.MinArgs, s.MaxArgs}
	}
	for _, s := range lang.Shapes2D {
		m[s.Name] = struct{ min, max int }{s.MinArgs, s.MaxArgs}
	}
	return m
}

// Analyze walks the AST and performs semantic analysis, returning any
// diagnostics (errors, warnings, hints). It checks:
//   - Undefined identifiers (with fuzzy suggestions)
//   - Unknown method names (with suggestions)
//   - Common mistakes (e.g. '+' between shapes)
//   - Variable-defined-before-use
//   - Function arity for built-in shapes
func Analyze(prog *ast.Program) []diagnostic.Diagnostic {
	a := &analyzer{
		builtins: NewBuiltinScope(),
	}
	a.scope = NewScope(a.builtins)

	// First pass: collect all top-level definitions so forward references work.
	for _, stmt := range prog.Statements {
		if assign, ok := stmt.(*ast.AssignStmt); ok {
			sym := &Symbol{
				Name: assign.Name,
				Type: TypeFloat, // default; refined below
				Node: assign,
			}
			if assign.Params != nil {
				// Function definition — infer type from shape names (heuristic).
				sym.Type = TypeSDF3D
			}
			// Allow redefinition at top level (last one wins).
			a.scope.Symbols[assign.Name] = sym
		}
	}

	// Second pass: walk AST.
	for _, stmt := range prog.Statements {
		a.checkStmt(stmt)
	}

	return a.diags
}

type analyzer struct {
	builtins *Scope
	scope    *Scope
	diags    []diagnostic.Diagnostic
}

func (a *analyzer) addDiag(sev diagnostic.Severity, msg string, span token.Span, help string) {
	d := diagnostic.Diagnostic{
		Severity: sev,
		Message:  msg,
		Span:     span,
	}
	if help != "" {
		d.Help = help
	}
	a.diags = append(a.diags, d)
}

func (a *analyzer) pushScope() {
	a.scope = NewScope(a.scope)
}

func (a *analyzer) popScope() {
	a.scope = a.scope.Parent
}

// ---------------------------------------------------------------------------
// Statement checking
// ---------------------------------------------------------------------------

func (a *analyzer) checkStmt(stmt ast.Statement) {
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		a.checkAssign(s)
	case *ast.ExprStmt:
		a.checkExpr(s.Expression)
	case *ast.SettingStmt:
		// Settings are not deeply analyzed in this phase.
	}
}

func (a *analyzer) checkAssign(s *ast.AssignStmt) {
	if s.Params != nil {
		// Function definition: create a child scope with params.
		a.pushScope()
		for _, p := range s.Params {
			a.scope.Symbols[p.Name] = &Symbol{
				Name: p.Name,
				Type: TypeFloat,
			}
			if p.Default != nil {
				a.checkExpr(p.Default)
			}
		}
		a.checkExpr(s.Value)
		a.popScope()
	} else {
		// Variable assignment.
		a.checkExpr(s.Value)
	}
}

// ---------------------------------------------------------------------------
// Expression checking
// ---------------------------------------------------------------------------

func (a *analyzer) checkExpr(expr ast.Expr) {
	if expr == nil {
		return
	}

	switch e := expr.(type) {
	case *ast.NumberLit, *ast.BoolLit, *ast.StringLit, *ast.HexColorLit:
		// Leaf — nothing to check.

	case *ast.Ident:
		a.checkIdent(e)

	case *ast.VecLit:
		for _, elem := range e.Elems {
			a.checkExpr(elem)
		}

	case *ast.BinaryExpr:
		a.checkBinaryExpr(e)

	case *ast.UnaryExpr:
		a.checkExpr(e.Operand)

	case *ast.FuncCall:
		a.checkFuncCall(e)

	case *ast.MethodCall:
		a.checkMethodCall(e)

	case *ast.Swizzle:
		a.checkExpr(e.Receiver)

	case *ast.Block:
		a.pushScope()
		for _, s := range e.Stmts {
			a.checkStmt(s)
		}
		if e.Result != nil {
			a.checkExpr(e.Result)
		}
		a.popScope()

	case *ast.ForExpr:
		a.pushScope()
		for _, it := range e.Iterators {
			a.checkExpr(it.Start)
			a.checkExpr(it.End)
			if it.Step != nil {
				a.checkExpr(it.Step)
			}
			a.scope.Symbols[it.Name] = &Symbol{
				Name: it.Name,
				Type: TypeFloat,
			}
		}
		if e.Body != nil {
			for _, s := range e.Body.Stmts {
				a.checkStmt(s)
			}
			if e.Body.Result != nil {
				a.checkExpr(e.Body.Result)
			}
		}
		a.popScope()

	case *ast.IfExpr:
		a.checkExpr(e.Cond)
		if e.Then != nil {
			a.pushScope()
			for _, s := range e.Then.Stmts {
				a.checkStmt(s)
			}
			if e.Then.Result != nil {
				a.checkExpr(e.Then.Result)
			}
			a.popScope()
		}
		if e.Else != nil {
			a.checkExpr(e.Else)
		}

	case *ast.GlslEscape:
		// Raw GLSL — not analyzed.
	}
}

func (a *analyzer) checkIdent(e *ast.Ident) {
	sym := a.scope.Lookup(e.Name)
	if sym != nil {
		sym.Used = true
		return
	}

	// Undefined variable — try to suggest a close match.
	candidates := a.scope.AllSymbolNames()
	suggestion := suggest(e.Name, candidates, 2)

	msg := fmt.Sprintf("undefined variable '%s'", e.Name)
	help := ""
	if suggestion != "" {
		help = fmt.Sprintf("did you mean '%s'?", suggestion)
	}

	a.addDiag(diagnostic.Error, msg, e.NodeSpan(), help)
}

func (a *analyzer) checkBinaryExpr(e *ast.BinaryExpr) {
	a.checkExpr(e.Left)
	a.checkExpr(e.Right)

	// Common mistake: '+' between two identifiers that look like shapes.
	if e.Op == ast.Add {
		leftIdent, leftOk := e.Left.(*ast.Ident)
		rightIdent, rightOk := e.Right.(*ast.Ident)
		if leftOk && rightOk {
			leftSym := a.scope.Lookup(leftIdent.Name)
			rightSym := a.scope.Lookup(rightIdent.Name)
			if leftSym != nil && rightSym != nil {
				if isSDFType(leftSym.Type) && isSDFType(rightSym.Type) {
					a.addDiag(diagnostic.Error,
						"'+' is arithmetic only",
						e.NodeSpan(),
						"did you mean '|' for union?")
				}
			}
		}
	}
}

func (a *analyzer) checkFuncCall(e *ast.FuncCall) {
	sym := a.scope.Lookup(e.Name)
	if sym == nil {
		candidates := a.scope.AllSymbolNames()
		suggestion := suggest(e.Name, candidates, 2)
		msg := fmt.Sprintf("undefined variable '%s'", e.Name)
		help := ""
		if suggestion != "" {
			help = fmt.Sprintf("did you mean '%s'?", suggestion)
		}
		a.addDiag(diagnostic.Error, msg, e.NodeSpan(), help)
	} else {
		sym.Used = true

		// Check arity for built-in shapes.
		if arity, ok := shapeArity[e.Name]; ok {
			positional := countPositionalArgs(e.Args)
			if positional > arity.max {
				a.addDiag(diagnostic.Error,
					fmt.Sprintf("'%s' takes at most %d argument(s), got %d",
						e.Name, arity.max, positional),
					e.NodeSpan(), "")
			}
		}
	}

	// Check argument expressions.
	for _, arg := range e.Args {
		a.checkExpr(arg.Value)
	}
}

func (a *analyzer) checkMethodCall(e *ast.MethodCall) {
	a.checkExpr(e.Receiver)

	// Check that the method name is known.
	if !isKnownMethod(e.Name) {
		msg := fmt.Sprintf("unknown method '%s'", e.Name)
		help := ""

		// First check common aliases (translate -> at, move -> at, etc.).
		if alias, ok := methodAliases[e.Name]; ok {
			help = fmt.Sprintf("did you mean '%s'?", alias)
		} else {
			// Fall back to fuzzy matching.
			suggestion := suggest(e.Name, knownMethods, 2)
			if suggestion != "" {
				help = fmt.Sprintf("did you mean '%s'?", suggestion)
			}
		}

		a.addDiag(diagnostic.Error, msg, e.NodeSpan(), help)
	}

	for _, arg := range e.Args {
		a.checkExpr(arg.Value)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func isSDFType(t Type) bool {
	return t == TypeSDF2D || t == TypeSDF3D
}

func isKnownMethod(name string) bool {
	for _, m := range knownMethods {
		if m == name {
			return true
		}
	}
	return false
}

func countPositionalArgs(args []ast.Arg) int {
	count := 0
	for _, arg := range args {
		if arg.Name == "" {
			count++
		}
	}
	return count
}

// ---------------------------------------------------------------------------
// Levenshtein distance & suggestion
// ---------------------------------------------------------------------------

// levenshtein computes the edit distance between two strings using the
// standard dynamic programming algorithm.
func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	// Use a single row for space efficiency.
	prev := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr := make([]int, lb+1)
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			curr[j] = minInt(del, minInt(ins, sub))
		}
		prev = curr
	}

	return prev[lb]
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// suggest returns the closest match to name from candidates within maxDist
// edit distance. Returns "" if no match is close enough.
func suggest(name string, candidates []string, maxDist int) string {
	best := ""
	bestDist := maxDist + 1
	lower := strings.ToLower(name)

	for _, c := range candidates {
		d := levenshtein(lower, strings.ToLower(c))
		if d < bestDist {
			bestDist = d
			best = c
		}
	}

	if bestDist <= maxDist {
		return best
	}
	return ""
}
