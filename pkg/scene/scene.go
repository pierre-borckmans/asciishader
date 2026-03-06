package scene

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Scene is a named shader scene.
type Scene struct {
	Name     string
	GLSL     string // optional GLSL code for GPU editor (sceneSDF + sceneColor)
	Chisel   string // optional Chisel source (compiled to GLSL)
	FilePath string // source file path (for file watching)
}

// Scenes is populated by LoadShaderFiles from .glsl and .chisel files in shaders/.
var Scenes []Scene

// LoadShaderFiles scans the shaders/ directory for .glsl and .chisel files and
// associates them with scenes. For each file, if a built-in scene with a
// matching name exists, its GLSL/Chisel field is set. Otherwise a new scene is
// appended.
//
// File naming: snake_case.glsl -> "Title Case" scene name.
// A "// Scene: Custom Name" header line overrides the filename-derived name.
func LoadShaderFiles() {
	exePath, err := os.Executable()
	if err != nil {
		return
	}
	dir := filepath.Dir(exePath)

	// Collect all .glsl and .chisel files from shaders/ and shaders/glsl-legacy/
	dirs := []string{"shaders", filepath.Join("shaders", "glsl-legacy")}
	seen := make(map[string]bool) // deduplicate by absolute path
	var allPaths []string
	for _, ext := range []string{"*.glsl", "*.chisel"} {
		for _, d := range dirs {
			for _, pattern := range []string{filepath.Join(dir, d, ext), filepath.Join(d, ext)} {
				m, _ := filepath.Glob(pattern)
				for _, p := range m {
					abs, _ := filepath.Abs(p)
					if !seen[abs] {
						seen[abs] = true
						allPaths = append(allPaths, p)
					}
				}
			}
		}
	}

	// Sort alphabetically by filename (base name, case-insensitive)
	sort.Slice(allPaths, func(i, j int) bool {
		return strings.ToLower(filepath.Base(allPaths[i])) < strings.ToLower(filepath.Base(allPaths[j]))
	})

	for _, path := range allPaths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := string(data)
		name := ShaderFileName(path, content)
		isChisel := strings.HasSuffix(path, ".chisel")

		s := Scene{
			Name:     name,
			FilePath: path,
		}
		if isChisel {
			s.Chisel = content
		} else {
			s.GLSL = content
		}
		Scenes = append(Scenes, s)
	}
}

// ShaderFileName derives a scene name from a .glsl file path.
// If the file contains a "// Scene: Name" header, that name is used.
// Otherwise the filename is converted from snake_case to Title Case.
func ShaderFileName(path, content string) string {
	// Check for "// Scene: ..." header in first 5 lines
	lines := strings.SplitN(content, "\n", 6)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "// Scene:") {
			name := strings.TrimSpace(strings.TrimPrefix(line, "// Scene:"))
			if name != "" {
				return name
			}
		}
	}

	// Fall back to filename -> title case
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, ".glsl")
	base = strings.TrimSuffix(base, ".chisel")
	words := strings.Split(base, "_")
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}
