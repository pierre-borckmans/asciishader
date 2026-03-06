package chisel_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"asciishader/pkg/chisel"
	"asciishader/pkg/chisel/diagnostic"
)

func TestCompileBasic(t *testing.T) {
	glsl, diags := chisel.Compile("sphere")
	for _, d := range diags {
		if d.Severity == diagnostic.Error {
			t.Errorf("unexpected error: %s", d.Error())
		}
	}
	if !strings.Contains(glsl, "sceneSDF") {
		t.Errorf("expected GLSL to contain sceneSDF, got:\n%s", glsl)
	}
	if !strings.Contains(glsl, "sceneColor") {
		t.Errorf("expected GLSL to contain sceneColor, got:\n%s", glsl)
	}
	if !strings.Contains(glsl, "sdSphere") {
		t.Errorf("expected GLSL to contain sdSphere, got:\n%s", glsl)
	}
}

func TestCompileBooleanOps(t *testing.T) {
	glsl, diags := chisel.Compile("sphere | box")
	for _, d := range diags {
		if d.Severity == diagnostic.Error {
			t.Errorf("unexpected error: %s", d.Error())
		}
	}
	if !strings.Contains(glsl, "opUnion") {
		t.Errorf("expected GLSL to contain opUnion, got:\n%s", glsl)
	}
}

func TestCompileTransform(t *testing.T) {
	glsl, diags := chisel.Compile("sphere.at(1, 2, 3)")
	for _, d := range diags {
		if d.Severity == diagnostic.Error {
			t.Errorf("unexpected error: %s", d.Error())
		}
	}
	if !strings.Contains(glsl, "vec3(1.0, 2.0, 3.0)") {
		t.Errorf("expected GLSL to contain translation vector, got:\n%s", glsl)
	}
}

func TestCompileFunction(t *testing.T) {
	glsl, diags := chisel.Compile("f(x) = sphere(x)\nf(2)")
	for _, d := range diags {
		if d.Severity == diagnostic.Error {
			t.Errorf("unexpected error: %s", d.Error())
		}
	}
	if !strings.Contains(glsl, "fn_f") {
		t.Errorf("expected GLSL to contain fn_f, got:\n%s", glsl)
	}
}

func TestCompileForLoop(t *testing.T) {
	glsl, diags := chisel.Compile("for i in 0..3 { sphere.at(i, 0, 0) }")
	for _, d := range diags {
		if d.Severity == diagnostic.Error {
			t.Errorf("unexpected error: %s", d.Error())
		}
	}
	count := strings.Count(glsl, "sdSphere")
	if count != 3 {
		t.Errorf("expected 3 sdSphere calls, got %d:\n%s", count, glsl)
	}
}

func TestCompileEmpty(t *testing.T) {
	glsl, diags := chisel.Compile("")
	for _, d := range diags {
		if d.Severity == diagnostic.Error {
			t.Errorf("unexpected error: %s", d.Error())
		}
	}
	if !strings.Contains(glsl, "sceneSDF") {
		t.Errorf("expected GLSL to contain sceneSDF, got:\n%s", glsl)
	}
}

// ---------------------------------------------------------------------------
// Fixture-based tests
// ---------------------------------------------------------------------------

func TestFixtures(t *testing.T) {
	// Test all valid fixtures.
	validFiles, err := filepath.Glob("testdata/valid/*.chisel")
	if err != nil {
		t.Fatalf("failed to glob valid fixtures: %v", err)
	}
	if len(validFiles) == 0 {
		t.Fatal("no valid fixture files found in testdata/valid/")
	}

	for _, path := range validFiles {
		name := filepath.Base(path)
		t.Run("valid/"+name, func(t *testing.T) {
			source, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("failed to read %s: %v", path, err)
			}

			glsl, diags := chisel.Compile(string(source))

			// Must have zero errors.
			for _, d := range diags {
				if d.Severity == diagnostic.Error {
					t.Errorf("unexpected error: %s", d.Message)
				}
			}

			// Must produce non-empty GLSL.
			if glsl == "" {
				t.Fatal("empty GLSL output")
			}

			// Check expected patterns from sidecar file.
			expectedPath := strings.TrimSuffix(path, ".chisel") + ".expected"
			if expectedData, err := os.ReadFile(expectedPath); err == nil {
				checkExpected(t, glsl, string(expectedData))
			}
		})
	}

	// Test all error fixtures.
	errorFiles, err := filepath.Glob("testdata/errors/*.chisel")
	if err != nil {
		t.Fatalf("failed to glob error fixtures: %v", err)
	}
	if len(errorFiles) == 0 {
		t.Fatal("no error fixture files found in testdata/errors/")
	}

	for _, path := range errorFiles {
		name := filepath.Base(path)
		t.Run("errors/"+name, func(t *testing.T) {
			source, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("failed to read %s: %v", path, err)
			}

			_, diags := chisel.Compile(string(source))

			// Must have at least one error.
			hasError := false
			for _, d := range diags {
				if d.Severity == diagnostic.Error {
					hasError = true
					break
				}
			}

			expectedPath := strings.TrimSuffix(path, ".chisel") + ".expected"
			expectedData, readErr := os.ReadFile(expectedPath)
			if readErr == nil {
				checkErrorExpected(t, diags, string(expectedData))
			} else if !hasError {
				t.Error("expected at least one error but got none")
			}
		})
	}
}

// checkExpected verifies that the GLSL output contains (or does not contain)
// the patterns specified in the expected file.
//
// Lines starting with '#' are comments. Lines starting with '!' are negative
// assertions (the pattern must NOT appear). All other non-empty lines are
// positive assertions (the pattern MUST appear).
func checkExpected(t *testing.T, glsl, expected string) {
	t.Helper()
	for _, line := range strings.Split(expected, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "!") {
			pattern := strings.TrimSpace(line[1:])
			if strings.Contains(glsl, pattern) {
				t.Errorf("GLSL should NOT contain %q", pattern)
			}
		} else {
			if !strings.Contains(glsl, line) {
				t.Errorf("GLSL should contain %q but doesn't.\nGLSL output:\n%s", line, glsl)
			}
		}
	}
}

// checkErrorExpected verifies that the compiler diagnostics contain the
// expected error substrings. Each non-empty, non-comment line in the expected
// file is a substring that must appear in at least one diagnostic message.
func checkErrorExpected(t *testing.T, diags []diagnostic.Diagnostic, expected string) {
	t.Helper()
	allErrors := ""
	for _, d := range diags {
		allErrors += d.Message + "\n"
	}
	for _, line := range strings.Split(expected, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.Contains(strings.ToLower(allErrors), strings.ToLower(line)) {
			t.Errorf("expected error containing %q but not found in:\n%s", line, allErrors)
		}
	}
}
