package recorder

import (
	"asciishader/pkg/core"
	gpupkg "asciishader/pkg/gpu"
)

// AppState is the interface the app model must satisfy for recording.
type AppState interface {
	GetRenderConfig() *core.RenderConfig
	GetGPU() *gpupkg.GPURenderer
	GetTime() float64
	GetCamAngleX() float64
	GetCamAngleY() float64
	GetCamDist() float64
	GetCamTarget() core.Vec3
}
