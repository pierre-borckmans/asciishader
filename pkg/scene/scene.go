package scene

import (
	"os"
	"path/filepath"
	"strings"
)

// Scene is a named shader scene.
type Scene struct {
	Name     string
	GLSL     string // optional GLSL code for GPU editor (sceneSDF + sceneColor)
	Chisel   string // optional Chisel source (compiled to GLSL)
	FilePath string // source file path (for file watching)
}

var Scenes = []Scene{
	{Name: "Bullet Train"},
	{Name: "Sphere & Cube"},
	{Name: "Torus Knot"},
	{Name: "Morphing Shapes"},
	{Name: "Infinite Pillars"},
	{Name: "Alien Egg"},
	{Name: "Alien Egg Color"},
	{Name: "Gyroid"},
	{Name: "Crystal Cluster"},
	{Name: "Plasma Orb"},
	{Name: "Plasma Rainbow"},
	{Name: "Deep Nebula"},
	{Name: "Solar Flare"},
	{Name: "Void Bloom"},
	{Name: "Jellyfish"},
	{Name: "Frozen Star"},
	{Name: "Railway Express"},
	{Name: "Lava Lamp"},
	{Name: "Mercury"},
	{Name: "Amoeba"},
}

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

	// Load .glsl and .chisel files from shaders/ and shaders/glsl-legacy/
	dirs := []string{"shaders", filepath.Join("shaders", "glsl-legacy")}
	for _, ext := range []string{"*.glsl", "*.chisel"} {
		var matches []string
		for _, d := range dirs {
			pattern := filepath.Join(dir, d, ext)
			m, _ := filepath.Glob(pattern)
			matches = append(matches, m...)
			cwdPattern := filepath.Join(d, ext)
			m, _ = filepath.Glob(cwdPattern)
			matches = append(matches, m...)
		}
		if len(matches) == 0 {
			continue
		}

		for _, path := range matches {
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			content := string(data)
			name := ShaderFileName(path, content)
			isChisel := strings.HasSuffix(path, ".chisel")

			// Try to match an existing scene by name
			found := false
			for i := range Scenes {
				if Scenes[i].Name == name {
					if isChisel {
						Scenes[i].Chisel = content
					} else {
						Scenes[i].GLSL = content
					}
					Scenes[i].FilePath = path
					found = true
					break
				}
			}
			if !found {
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
