package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"asciishader/pkg/scene"
	"asciishader/tui/components"
)

// switchView changes the active view mode and updates the sidebar.
func switchView(m *Model, mode ViewMode) {
	m.Mode = mode
	ids := []string{"view-shader", "view-player", "view-gallery", "view-help"}
	if int(mode) < len(ids) {
		m.Sidebar.SetActiveID(ids[mode])
	}
	if mode == ViewPlayer {
		m.PlayerView.ScanFiles()
	}
	if mode == ViewGallery {
		m.Gallery.Selected = m.Scene
	}
}

// syncSceneGLSL populates the editor with scene GLSL or Chisel code if available.
func syncSceneGLSL(m *Model) {
	if m.Scene < 0 || m.Scene >= len(scene.Scenes) {
		return
	}
	s := scene.Scenes[m.Scene]

	// Set up file watching
	m.WatchFile = s.FilePath
	m.WatchModTime = time.Time{}
	if s.FilePath != "" {
		if info, err := os.Stat(s.FilePath); err == nil {
			m.WatchModTime = info.ModTime()
		}
	}

	// Prefer Chisel source if available
	if s.Chisel != "" {
		m.Editor.SetChiselCode(s.Chisel)
		if m.GPU != nil {
			m.Editor.Compile(m.GPU)
		}
		syncCompileErr(m)
		syncSceneTree(m, s.Chisel)
		return
	}

	if s.GLSL != "" {
		m.Editor.ChiselMode = false
		m.Editor.SetCode(s.GLSL)
		if m.GPU != nil {
			err := m.GPU.CompileGLSLCode(s.GLSL)
			if err != nil {
				m.Editor.Status = fmt.Sprintf("Error: %s", err.Error())
				m.Editor.StatusErr = true
			} else {
				m.Editor.Status = "Compiled OK"
				m.Editor.StatusErr = false
			}
		}
		syncCompileErr(m)
		m.SceneTree.SetRoots(nil) // no tree for raw GLSL
	}
}

// syncCompileErr updates the persistent compile error from the editor status.
func syncCompileErr(m *Model) {
	if m.Editor.StatusErr {
		m.CompileErr = m.Editor.Status
	} else {
		m.CompileErr = ""
	}
}

// checkFileChanged polls the current scene's source file for changes.
func checkFileChanged(m *Model) {
	if m.WatchFile == "" {
		return
	}
	now := time.Now()
	if now.Sub(m.WatchCheck) < 500*time.Millisecond {
		return
	}
	m.WatchCheck = now

	info, err := os.Stat(m.WatchFile)
	if err != nil {
		return
	}
	if !info.ModTime().After(m.WatchModTime) {
		return
	}
	m.WatchModTime = info.ModTime()

	data, err := os.ReadFile(m.WatchFile)
	if err != nil {
		return
	}
	content := string(data)

	// Update scene and editor
	s := &scene.Scenes[m.Scene]
	isChisel := strings.HasSuffix(m.WatchFile, ".chisel")
	if isChisel {
		s.Chisel = content
		m.Editor.SetChiselCode(content)
	} else {
		s.GLSL = content
		m.Editor.ChiselMode = false
		m.Editor.SetCode(content)
	}

	// Recompile
	if m.GPU != nil {
		m.Editor.Compile(m.GPU)
	}
	syncCompileErr(m)
	if isChisel {
		syncSceneTree(m, content)
	}

	m.RecMessage = fmt.Sprintf("Reloaded %s", filepath.Base(m.WatchFile))
	m.RecMessageTime = time.Now()
}

// syncSceneTree rebuilds the scene tree from Chisel source.
func syncSceneTree(m *Model, chiselSource string) {
	roots := components.BuildSceneTree(chiselSource)
	m.SceneTree.SetRoots(roots)
}
