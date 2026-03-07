package main

import (
	"fmt"
	"os"
	"runtime"

	gpupkg "asciishader/pkg/gpu"
	"asciishader/pkg/scene"
	"asciishader/tui/app"

	tea "charm.land/bubbletea/v2"
	zone "github.com/lrstanley/bubblezone/v2"
)

func main() {
	runtime.LockOSThread()

	if len(os.Args) >= 3 && os.Args[1] == "--play" {
		runPlayer(os.Args[2])
		return
	}

	zone.NewGlobal()
	scene.LoadShaderFiles()

	m := app.NewModel()

	gpuRenderer, gpuErr := gpupkg.NewGPURenderer()
	if gpuErr != nil {
		fmt.Fprintf(os.Stderr, "GPU init failed: %v\n", gpuErr)
		os.Exit(1)
	}
	m.GPU = gpuRenderer
	m.SyncSceneGLSL()
	defer gpuRenderer.Destroy()

	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
