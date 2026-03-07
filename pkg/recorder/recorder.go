package recorder

import (
	"fmt"
	"math"
	"time"

	"asciishader/pkg/clip"
	"asciishader/pkg/core"
)

// RecordingState is the state machine for the recording system.
type RecordingState int

const (
	RecordIdle      RecordingState = iota
	RecordSelecting                // Region selection UI active
	RecordLive                     // Capturing keyframes
	RecordBaking                   // Re-rendering frames
	RecordDone                     // Bake complete, file saved
)

// Recorder manages the recording pipeline.
type Recorder struct {
	// Region (in viewport cells)
	RegionX, RegionY int
	RegionW, RegionH int

	// Captured keyframes during live recording
	Keyframes []clip.Keyframe
	StartTime time.Time

	// Bake state
	bakeFrameIdx int
	bakeFrames   [][]byte // [frameIdx] = planar sub-pixel frame

	// Output
	OutputPath string
	Error      error
}

// NewRecorder creates a new recorder.
func NewRecorder(regionX, regionY, regionW, regionH int) *Recorder {
	return &Recorder{
		RegionX: regionX,
		RegionY: regionY,
		RegionW: regionW,
		RegionH: regionH,
	}
}

// CaptureKeyframe snapshots the current model state as a keyframe.
func (rec *Recorder) CaptureKeyframe(m AppState) {
	elapsed := time.Since(rec.StartTime)
	kf := clip.Keyframe{
		TimeMs:      uint32(elapsed.Milliseconds()),
		ShaderTime:  float32(m.GetTime()),
		CamAngleX:   float32(m.GetCamAngleX()),
		CamAngleY:   float32(m.GetCamAngleY()),
		CamDist:     float32(m.GetCamDist()),
		CamTargetX:  float32(m.GetCamTarget().X),
		CamTargetY:  float32(m.GetCamTarget().Y),
		CamTargetZ:  float32(m.GetCamTarget().Z),
		Contrast:    float32(m.GetRenderConfig().Contrast),
		Ambient:     float32(m.GetRenderConfig().Ambient),
		SpecPower:   float32(m.GetRenderConfig().SpecPower),
		ShadowSteps: uint16(m.GetRenderConfig().ShadowSteps),
		AOSteps:     uint16(m.GetRenderConfig().AOSteps),
	}
	rec.Keyframes = append(rec.Keyframes, kf)
}

// StartLive begins the live capture phase.
func (rec *Recorder) StartLive() {
	rec.Keyframes = nil
	rec.StartTime = time.Now()
}

// StartBake initializes the bake phase.
func (rec *Recorder) StartBake() {
	rec.bakeFrameIdx = 0
	rec.bakeFrames = make([][]byte, len(rec.Keyframes))
}

// BakeProgress returns (current, total) for progress display.
func (rec *Recorder) BakeProgress() (int, int) {
	return rec.bakeFrameIdx, len(rec.Keyframes)
}

// BakeDone returns true when all frames have been rendered.
func (rec *Recorder) BakeDone() bool {
	return rec.bakeFrameIdx >= len(rec.Keyframes)
}

// BakeStep renders one frame as sub-pixel data. Returns true when bake is complete.
func (rec *Recorder) BakeStep(m AppState) bool {
	if rec.BakeDone() {
		return true
	}

	kf := rec.Keyframes[rec.bakeFrameIdx]
	rec.applyKeyframe(m, kf)

	// Render at 2×4 sub-pixel resolution
	m.GetGPU().RenderRaw(m.GetRenderConfig(), clip.SubW, clip.SubH)
	pixels, pixW := m.GetGPU().RawPixels()

	rec.bakeFrames[rec.bakeFrameIdx] = rec.extractSubPixels(pixels, pixW)

	rec.bakeFrameIdx++
	return rec.BakeDone()
}

// extractSubPixels reads the raw RGBA pixel buffer and packs brightness + color
// into a planar frame suitable for the clip format.
func (rec *Recorder) extractSubPixels(pixels []byte, pixW int) []byte {
	w, h := rec.RegionW, rec.RegionH
	cellCount := w * h

	brightness := make([]uint8, cellCount*clip.SubPixels)
	colors := make([]uint16, cellCount)

	for cy := 0; cy < h; cy++ {
		for cx := 0; cx < w; cx++ {
			cellIdx := cy*w + cx
			bx := cx * clip.SubW
			by := cy * clip.SubH

			var colR, colG, colB float64
			var litCount float64
			for sy := 0; sy < clip.SubH; sy++ {
				for sx := 0; sx < clip.SubW; sx++ {
					px := bx + sx
					py := by + sy
					off := (py*pixW + px) * 4
					spIdx := sy*clip.SubW + sx
					a := pixels[off+3]
					brightness[cellIdx*clip.SubPixels+spIdx] = a
					if a > 2 {
						colR += float64(pixels[off])
						colG += float64(pixels[off+1])
						colB += float64(pixels[off+2])
						litCount++
					}
				}
			}

			if litCount > 0 {
				colors[cellIdx] = clip.RGB565Encode(colR/litCount/255, colG/litCount/255, colB/litCount/255)
			}
		}
	}

	return clip.PackFrame(brightness, colors, w, h)
}

// applyKeyframe configures the renderer from a keyframe for baking.
func (rec *Recorder) applyKeyframe(m AppState, kf clip.Keyframe) {
	m.GetRenderConfig().Resize(rec.RegionW, rec.RegionH)
	m.GetRenderConfig().Time = float64(kf.ShaderTime)
	m.GetRenderConfig().Contrast = float64(kf.Contrast)
	m.GetRenderConfig().Ambient = float64(kf.Ambient)
	m.GetRenderConfig().SpecPower = float64(kf.SpecPower)
	m.GetRenderConfig().ShadowSteps = int(kf.ShadowSteps)
	m.GetRenderConfig().AOSteps = int(kf.AOSteps)

	camAngleX := float64(kf.CamAngleX)
	camAngleY := float64(kf.CamAngleY)
	camDist := float64(kf.CamDist)
	camTarget := core.Vec3{X: float64(kf.CamTargetX), Y: float64(kf.CamTargetY), Z: float64(kf.CamTargetZ)}

	m.GetRenderConfig().Camera.Pos = core.Vec3{
		X: camTarget.X + math.Sin(camAngleY)*math.Cos(camAngleX)*camDist,
		Y: camTarget.Y + math.Sin(camAngleX)*camDist,
		Z: camTarget.Z - math.Cos(camAngleY)*math.Cos(camAngleX)*camDist,
	}
	m.GetRenderConfig().Camera.Target = camTarget

	shaderTime := float64(kf.ShaderTime)
	m.GetRenderConfig().LightDir = core.V(
		math.Sin(shaderTime*0.5)*0.5,
		0.8,
		math.Cos(shaderTime*0.5)*0.5-0.5,
	).Normalize()
}

// Finalize encodes, compresses, and writes the .asciirec file.
func (rec *Recorder) Finalize() error {
	if len(rec.Keyframes) == 0 {
		return fmt.Errorf("no keyframes recorded")
	}

	numFrames := len(rec.Keyframes)
	lastKf := rec.Keyframes[numFrames-1]

	// Compute actual FPS from captured timestamps
	fps := uint8(30)
	if lastKf.TimeMs > 0 {
		fps = uint8(float64(numFrames) * 1000 / float64(lastKf.TimeMs))
		if fps < 1 {
			fps = 1
		}
	}

	header := clip.ClipHeader{
		FPS:        fps,
		NumFrames:  uint16(numFrames),
		Width:      uint16(rec.RegionW),
		Height:     uint16(rec.RegionH),
		DurationMs: lastKf.TimeMs,
	}

	raw := clip.EncodeTrack(rec.bakeFrames)

	compressed, err := clip.CompressTrack(raw)
	if err != nil {
		return fmt.Errorf("compress: %w", err)
	}

	if rec.OutputPath == "" {
		rec.OutputPath = fmt.Sprintf("recording_%s.asciirec", time.Now().Format("20060102_150405"))
	}

	return clip.WriteClip(rec.OutputPath, header, rec.Keyframes, compressed)
}

// RecordingDuration returns the elapsed time since recording started.
func (rec *Recorder) RecordingDuration() time.Duration {
	return time.Since(rec.StartTime)
}
