package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	gpupkg "asciishader/gpu"
	"asciishader/shader"
)

func TestCompileShaderFiles(t *testing.T) {
	runtime.LockOSThread()

	gpu, err := gpupkg.NewGPURenderer()
	if err != nil {
		t.Skipf("No GPU available: %v", err)
	}
	defer gpu.Destroy()

	matches, _ := filepath.Glob(filepath.Join("shaders", "*.glsl"))
	if len(matches) == 0 {
		t.Fatal("No shader files found in shaders/")
	}

	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("Cannot read %s: %v", path, err)
		}
		code := string(data)
		name := filepath.Base(path)
		t.Logf("Compiling %s (raw=%v, %d bytes)", name, shader.IsRawShader(code), len(code))

		err = gpu.CompileUserCode(code)
		if err != nil {
			t.Errorf("Compile failed for %s:\n%v", name, err)
		} else {
			t.Logf("  OK: %s", name)
		}
	}
}
