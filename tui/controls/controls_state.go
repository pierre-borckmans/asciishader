package controls

import (
	"asciishader/pkg/core"
	gpupkg "asciishader/pkg/gpu"
)

// AppState is the interface that the app model must satisfy for controls to work.
type AppState interface {
	GetRenderConfig() *core.RenderConfig
	GetGPU() *gpupkg.GPURenderer
	GetScene() int
	SetScene(int)
	NumScenes() int
	SceneName(int) string
	GetTime() float64
	SetTime(float64)
	SyncSceneGLSL()
	GetCamAngleX() float64
	GetCamAngleY() float64
	GetCamDist() float64
	GetCamTarget() core.Vec3
	IsAutoRotate() bool
}
