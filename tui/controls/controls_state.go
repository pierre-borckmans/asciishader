package controls

import (
	gpupkg "asciishader/pkg/gpu"
	"asciishader/pkg/render"
)

// AppState is the interface that the app model must satisfy for controls to work.
type AppState interface {
	GetRenderer() *render.Renderer
	GetGPU() *gpupkg.GPURenderer
	IsGPUMode() bool
	SetGPUMode(bool)
	GetScene() int
	SetScene(int)
	NumScenes() int
	SceneName(int) string
	GetTime() float64
	SetTime(float64)
	SyncSceneGLSL()
}
