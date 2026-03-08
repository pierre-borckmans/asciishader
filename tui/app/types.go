// Package app implements the main BubbleTea application model for AsciiShader.
package app

import "time"

// TickMsg is sent on each frame tick.
type TickMsg time.Time

// FocusZone identifies what has keyboard focus.
type FocusZone int

const (
	FocusViewport FocusZone = iota
	FocusControls
	FocusTree
	FocusEditor
)

// ViewMode identifies which top-level view is active.
type ViewMode int

const (
	ViewShader  ViewMode = iota // current raymarching (default)
	ViewPlayer                  // clip player
	ViewGallery                 // scene browser
	ViewHelp                    // keybindings reference
)
