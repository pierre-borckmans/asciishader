package recorder

import (
	"asciishader/pkg/core"
	gpupkg "asciishader/pkg/gpu"
	"asciishader/pkg/render"
)

// AppState is the interface the app model must satisfy for recording.
type AppState interface {
	GetRenderer() *render.Renderer
	GetGPU() *gpupkg.GPURenderer
	IsGPUMode() bool
	GetTime() float64
	GetCamAngleX() float64
	GetCamAngleY() float64
	GetCamDist() float64
	GetCamTarget() core.Vec3
}
