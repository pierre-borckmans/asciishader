// Package chisel provides the public API for compiling Chisel language source
// code into GLSL. It wires together the lexer, parser, and code generator.
package chisel

import (
	"asciishader/pkg/chisel/codegen"
	"asciishader/pkg/chisel/diagnostic"
	"asciishader/pkg/chisel/lexer"
	"asciishader/pkg/chisel/parser"
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

	// Generate GLSL.
	glsl, genDiags := codegen.Generate(prog)
	allDiags = append(allDiags, genDiags...)

	return glsl, allDiags
}
