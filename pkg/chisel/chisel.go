// Package chisel provides the public API for compiling Chisel language source
// code into GLSL. It wires together the lexer, parser, and code generator.
package chisel

import (
	"asciishader/pkg/chisel/compiler/analyzer"
	"asciishader/pkg/chisel/compiler/codegen"
	"asciishader/pkg/chisel/compiler/diagnostic"
	"asciishader/pkg/chisel/compiler/lexer"
	"asciishader/pkg/chisel/compiler/parser"
)

// Compile takes Chisel source code and returns GLSL code defining
// sceneSDF(vec3 p) and sceneColor(vec3 p), along with any diagnostics.
func Compile(source string) (string, []diagnostic.Diagnostic) {
	var allDiags []diagnostic.Diagnostic

	// Lex.
	tokens, lexDiags := lexer.Lex("input.chisel", source)
	allDiags = append(allDiags, lexDiags...)

	// Check for lex errors.
	for _, d := range lexDiags {
		if d.Severity == diagnostic.Error {
			return "", allDiags
		}
	}

	// Parse.
	prog, parseDiags := parser.Parse(tokens)
	allDiags = append(allDiags, parseDiags...)

	// Check for parse errors.
	for _, d := range parseDiags {
		if d.Severity == diagnostic.Error {
			return "", allDiags
		}
	}

	// Analyze (warnings only — don't block compilation).
	analyzeDiags := analyzer.Analyze(prog)
	allDiags = append(allDiags, analyzeDiags...)

	// Generate GLSL.
	glsl, genDiags := codegen.Generate(prog)
	allDiags = append(allDiags, genDiags...)

	return glsl, allDiags
}
