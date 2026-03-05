package chisel_test

import (
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
