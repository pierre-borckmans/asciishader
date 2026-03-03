package main

import (
	"fmt"
	"math"
	"time"

	"asciishader/clip"
)

// RecordingState is the state machine for the recording system.
type RecordingState int

const (
	RecordIdle      RecordingState = iota
	RecordSelecting                // Region selection UI active
	RecordLive                     // Capturing keyframes
	RecordBaking                   // Re-rendering frames at each scale
	RecordDone                     // Bake complete, file saved
)

// RecordScale defines a scale multiplier and its resulting dimensions.
type RecordScale struct {
	Factor float64
	Width  int
	Height int
}

// Recorder manages the recording pipeline.
type Recorder struct {
	// Region (in viewport cells)
	RegionX, RegionY int
	RegionW, RegionH int

	// Scales to render
	Scales []RecordScale

	// Captured keyframes during live recording
	Keyframes []clip.Keyframe
	StartTime time.Time

	// Bake state
	bakeScaleIdx int
	bakeFrameIdx int
	bakeFrames   [][][]clip.ClipCell // [scaleIdx][frameIdx] = flat cell grid
	bakeTrackRaw [][]byte            // compressed track data per scale

	// Output
	OutputPath string
	Error      error
}

// NewRecorder creates a new recorder with default region and scales.
func NewRecorder(regionX, regionY, regionW, regionH int, scales []RecordScale) *Recorder {
	return &Recorder{
		RegionX: regionX,
		RegionY: regionY,
		RegionW: regionW,
		RegionH: regionH,
		Scales:  scales,
	}
}

// DefaultScales returns the default scale set for a given base region size.
func DefaultScales(baseW, baseH int) []RecordScale {
	scales := []RecordScale{
		{0.5, max(1, baseW/2), max(1, baseH/2)},
		{1.0, baseW, baseH},
		{2.0, baseW * 2, baseH * 2},
	}
	return scales
}

// CaptureKeyframe snapshots the current model state as a keyframe.
func (rec *Recorder) CaptureKeyframe(m *model) {
	elapsed := time.Since(rec.StartTime)
	kf := clip.Keyframe{
		TimeMs:      uint32(elapsed.Milliseconds()),
		ShaderTime:  float32(m.time),
		CamAngleX:   float32(m.camAngleX),
		CamAngleY:   float32(m.camAngleY),
		CamDist:     float32(m.camDist),
		CamTargetX:  float32(m.camTarget.X),
		CamTargetY:  float32(m.camTarget.Y),
		CamTargetZ:  float32(m.camTarget.Z),
		Contrast:    float32(m.renderer.Contrast),
		Ambient:     float32(m.renderer.Ambient),
		SpecPower:   float32(m.renderer.SpecPower),
		ShadowSteps: uint16(m.renderer.ShadowSteps),
		AOSteps:     uint16(m.renderer.AOSteps),
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
	rec.bakeScaleIdx = 0
	rec.bakeFrameIdx = 0
	rec.bakeFrames = make([][][]clip.ClipCell, len(rec.Scales))
	for i := range rec.bakeFrames {
		rec.bakeFrames[i] = make([][]clip.ClipCell, len(rec.Keyframes))
	}
	rec.bakeTrackRaw = nil
}

// BakeProgress returns (current, total) for progress display.
func (rec *Recorder) BakeProgress() (int, int) {
	total := len(rec.Scales) * len(rec.Keyframes)
	current := rec.bakeScaleIdx*len(rec.Keyframes) + rec.bakeFrameIdx
	return current, total
}

// BakeDone returns true when all frames at all scales have been rendered.
func (rec *Recorder) BakeDone() bool {
	return rec.bakeScaleIdx >= len(rec.Scales)
}

// BakeStep renders one frame at the current scale. Must be called on the main
// thread (GPU context). Returns true when bake is complete.
func (rec *Recorder) BakeStep(m *model) bool {
	if rec.BakeDone() {
		return true
	}

	scale := rec.Scales[rec.bakeScaleIdx]
	kf := rec.Keyframes[rec.bakeFrameIdx]

	// Apply keyframe state to model/renderer for this frame
	rec.applyKeyframe(m, kf, scale.Width, scale.Height)

	// Render at this scale's resolution
	var cells [][]cell
	if m.gpuMode && m.gpu != nil {
		cells = m.gpu.RenderToCells(m.renderer)
	} else {
		cells = m.renderer.RenderCells()
	}

	// Extract the region and convert to ClipCells
	clipCells := rec.extractRegion(cells, scale.Width, scale.Height)
	rec.bakeFrames[rec.bakeScaleIdx][rec.bakeFrameIdx] = clipCells

	// Advance to next frame/scale
	rec.bakeFrameIdx++
	if rec.bakeFrameIdx >= len(rec.Keyframes) {
		rec.bakeFrameIdx = 0
		rec.bakeScaleIdx++
	}

	return rec.BakeDone()
}

// applyKeyframe configures the renderer from a keyframe for baking.
func (rec *Recorder) applyKeyframe(m *model, kf clip.Keyframe, w, h int) {
	m.renderer.Resize(w, h)
	m.renderer.Time = float64(kf.ShaderTime)
	m.renderer.Contrast = float64(kf.Contrast)
	m.renderer.Ambient = float64(kf.Ambient)
	m.renderer.SpecPower = float64(kf.SpecPower)
	m.renderer.ShadowSteps = int(kf.ShadowSteps)
	m.renderer.AOSteps = int(kf.AOSteps)

	// Camera
	camAngleX := float64(kf.CamAngleX)
	camAngleY := float64(kf.CamAngleY)
	camDist := float64(kf.CamDist)
	camTarget := Vec3{float64(kf.CamTargetX), float64(kf.CamTargetY), float64(kf.CamTargetZ)}

	m.renderer.Camera.Pos = Vec3{
		camTarget.X + math.Sin(camAngleY)*math.Cos(camAngleX)*camDist,
		camTarget.Y + math.Sin(camAngleX)*camDist,
		camTarget.Z - math.Cos(camAngleY)*math.Cos(camAngleX)*camDist,
	}
	m.renderer.Camera.Target = camTarget

	// Animated light (same formula as main tick)
	shaderTime := float64(kf.ShaderTime)
	m.renderer.LightDir = V(
		math.Sin(shaderTime*0.5)*0.5,
		0.8,
		math.Cos(shaderTime*0.5)*0.5-0.5,
	).Normalize()
}

// extractRegion converts the full cell grid to a flat ClipCell slice at the given dimensions.
// During baking, the renderer is already sized to the scale dimensions, so we take the full grid.
func (rec *Recorder) extractRegion(cells [][]cell, w, h int) []clip.ClipCell {
	out := make([]clip.ClipCell, w*h)
	for y := 0; y < h && y < len(cells); y++ {
		for x := 0; x < w && x < len(cells[y]); x++ {
			out[y*w+x] = CellToClipCell(cells[y][x])
		}
	}
	return out
}

// Finalize encodes, compresses, and writes the .asciirec file.
func (rec *Recorder) Finalize() error {
	if len(rec.Keyframes) == 0 {
		return fmt.Errorf("no keyframes recorded")
	}

	numFrames := len(rec.Keyframes)
	numScales := len(rec.Scales)

	// Build header
	lastKf := rec.Keyframes[numFrames-1]
	header := clip.ClipHeader{
		FPS:        30,
		NumFrames:  uint16(numFrames),
		NumScales:  uint8(numScales),
		BaseWidth:  uint16(rec.RegionW),
		BaseHeight: uint16(rec.RegionH),
		DurationMs: lastKf.TimeMs,
	}

	// Build scale table and compress tracks
	scales := make([]clip.ScaleEntry, numScales)
	trackData := make([][]byte, numScales)

	for i, sc := range rec.Scales {
		scales[i] = clip.ScaleEntry{
			Width:  uint16(sc.Width),
			Height: uint16(sc.Height),
		}

		// Delta-encode the frames for this scale
		raw := clip.EncodeTrack(rec.bakeFrames[i])

		// Zlib compress
		compressed, err := clip.CompressTrack(raw)
		if err != nil {
			return fmt.Errorf("compress scale %d: %w", i, err)
		}
		trackData[i] = compressed
	}

	// Generate filename
	if rec.OutputPath == "" {
		rec.OutputPath = fmt.Sprintf("recording_%s.asciirec", time.Now().Format("20060102_150405"))
	}

	return clip.WriteClip(rec.OutputPath, header, scales, rec.Keyframes, trackData)
}

// RecordingDuration returns the elapsed time since recording started.
func (rec *Recorder) RecordingDuration() time.Duration {
	return time.Since(rec.StartTime)
}
