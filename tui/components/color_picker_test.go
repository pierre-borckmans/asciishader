package components

import (
	"strings"
	"testing"
)

func TestNewColorPicker(t *testing.T) {
	cp := NewColorPicker(255, 0, 0) // pure red
	r, g, b := cp.RGB()
	if r < 250 || g > 5 || b > 5 {
		t.Errorf("expected ~red, got (%d, %d, %d)", r, g, b)
	}
	hex := cp.HexString()
	if hex != "#ff0000" {
		t.Errorf("expected #ff0000, got %s", hex)
	}
}

func TestColorPickerBlack(t *testing.T) {
	cp := NewColorPicker(0, 0, 0)
	r, g, b := cp.RGB()
	if r != 0 || g != 0 || b != 0 {
		t.Errorf("expected black, got (%d, %d, %d)", r, g, b)
	}
}

func TestColorPickerWhite(t *testing.T) {
	cp := NewColorPicker(255, 255, 255)
	r, g, b := cp.RGB()
	if r < 250 || g < 250 || b < 250 {
		t.Errorf("expected ~white, got (%d, %d, %d)", r, g, b)
	}
}

func TestColorPickerSVNavigation(t *testing.T) {
	cp := NewColorPicker(128, 128, 128)
	startX, startY := cp.svX, cp.svY

	cp.HandleKey("right")
	if cp.svX != startX+1 {
		t.Errorf("expected svX=%d, got %d", startX+1, cp.svX)
	}

	cp.HandleKey("down")
	if cp.svY != startY+1 {
		t.Errorf("expected svY=%d, got %d", startY+1, cp.svY)
	}

	// Cursor should stay in bounds
	for i := 0; i < 50; i++ {
		cp.HandleKey("right")
	}
	if cp.svX != cpGridW-1 {
		t.Errorf("expected svX clamped to %d, got %d", cpGridW-1, cp.svX)
	}

	for i := 0; i < 50; i++ {
		cp.HandleKey("left")
	}
	if cp.svX != 0 {
		t.Errorf("expected svX clamped to 0, got %d", cp.svX)
	}
}

func TestColorPickerHueNavigation(t *testing.T) {
	cp := NewColorPicker(255, 0, 0) // hue=0 → hueY=0
	cp.HandleKey("tab")             // focus → hue
	if cp.focus != cpFocusHue {
		t.Fatalf("expected hue focus, got %d", cp.focus)
	}

	cp.HandleKey("down")
	if cp.hueY != 1 {
		t.Errorf("expected hueY=1, got %d", cp.hueY)
	}
	// Hue should have changed
	if cp.H < 1 {
		t.Error("expected hue to increase after moving down")
	}
}

func TestColorPickerHexEditing(t *testing.T) {
	cp := NewColorPicker(0, 0, 0)
	// Tab to hex focus
	cp.HandleKey("tab")
	cp.HandleKey("tab")
	if cp.focus != cpFocusHex {
		t.Fatalf("expected hex focus, got %d", cp.focus)
	}

	// Clear and type a color
	for len(cp.hexBuf) > 0 {
		cp.HandleKey("backspace")
	}
	for _, c := range "#ff8800" {
		cp.HandleKey(string(c))
	}

	r, g, b := cp.RGB()
	if r != 255 || g != 0x88 || b != 0 {
		t.Errorf("expected (255,136,0), got (%d,%d,%d)", r, g, b)
	}
}

func TestColorPickerTabCycles(t *testing.T) {
	cp := NewColorPicker(100, 100, 100)
	if cp.focus != cpFocusSV {
		t.Fatalf("expected SV focus initially, got %d", cp.focus)
	}
	cp.HandleKey("tab")
	if cp.focus != cpFocusHue {
		t.Errorf("expected hue focus, got %d", cp.focus)
	}
	cp.HandleKey("tab")
	if cp.focus != cpFocusHex {
		t.Errorf("expected hex focus, got %d", cp.focus)
	}
	cp.HandleKey("tab")
	if cp.focus != cpFocusSV {
		t.Errorf("expected SV focus after full cycle, got %d", cp.focus)
	}
}

func TestColorPickerShiftTab(t *testing.T) {
	cp := NewColorPicker(100, 100, 100)
	cp.HandleKey("shift+tab")
	if cp.focus != cpFocusHex {
		t.Errorf("expected hex focus from shift+tab, got %d", cp.focus)
	}
}

func TestColorPickerConfirmCancel(t *testing.T) {
	cp := NewColorPicker(100, 100, 100)
	if result := cp.HandleKey("enter"); result != ColorPickerConfirm {
		t.Errorf("expected Confirm, got %d", result)
	}

	cp2 := NewColorPicker(100, 100, 100)
	if result := cp2.HandleKey("esc"); result != ColorPickerCancel {
		t.Errorf("expected Cancel, got %d", result)
	}
}

func TestColorPickerRender(t *testing.T) {
	cp := NewColorPicker(255, 128, 0)
	output := cp.Render()

	// Should have the border
	if !strings.Contains(output, "╭") || !strings.Contains(output, "╰") {
		t.Error("expected box border characters in render")
	}
	// Should have "Color" title
	if !strings.Contains(output, "Color") {
		t.Error("expected 'Color' title in render")
	}
	// Should have grid cells (█)
	if !strings.Contains(output, "█") {
		t.Error("expected grid cells (█) in render")
	}
	// Should have cursor marker
	if !strings.Contains(output, "◆") {
		t.Error("expected cursor marker (◆) in render")
	}
	// Should have hex display
	if !strings.Contains(output, "#") {
		t.Error("expected hex display in render")
	}

	// Check line count matches Height()
	lines := strings.Split(output, "\n")
	if len(lines) != cp.Height() {
		t.Errorf("expected %d lines, got %d", cp.Height(), len(lines))
	}
}

func TestColorPickerRenderDimensions(t *testing.T) {
	cp := NewColorPicker(100, 200, 50)
	output := cp.Render()
	lines := strings.Split(output, "\n")

	// All lines should have the same visible width
	expectedW := cp.Width()
	for i, line := range lines {
		w := visibleWidth(line)
		if w != expectedW {
			t.Errorf("line %d: expected visible width %d, got %d", i, expectedW, w)
		}
	}
}

func TestColorPickerHueMarker(t *testing.T) {
	cp := NewColorPicker(255, 0, 0) // hue=0
	output := cp.Render()

	// SV focus: hue marker should be dimmed ◂
	if !strings.Contains(output, "◂") {
		t.Error("expected dimmed hue marker (◂) when SV is focused")
	}

	// Switch to hue focus
	cp.HandleKey("tab")
	output = cp.Render()
	// Now marker should be bright ◀
	if !strings.Contains(output, "◀") {
		t.Error("expected bright hue marker (◀) when hue is focused")
	}
}

func TestColorPickerSVCursorAppearance(t *testing.T) {
	cp := NewColorPicker(128, 128, 128)
	output := cp.Render()
	// SV focused: should show filled cursor ◆
	if !strings.Contains(output, "◆") {
		t.Error("expected filled cursor (◆) when SV is focused")
	}

	// Switch focus away from SV
	cp.HandleKey("tab")
	output = cp.Render()
	// SV unfocused: should show outline cursor ◇
	if !strings.Contains(output, "◇") {
		t.Error("expected outline cursor (◇) when SV is unfocused")
	}
}

func TestHSVRoundTrip(t *testing.T) {
	tests := [][3]uint8{
		{255, 0, 0},
		{0, 255, 0},
		{0, 0, 255},
		{255, 255, 255},
		{0, 0, 0},
		{128, 64, 32},
		{100, 200, 150},
	}
	for _, tc := range tests {
		h, s, v := rgbToHSV(tc[0], tc[1], tc[2])
		r, g, b := hsvToRGB(h, s, v)
		// Allow ±1 for rounding
		if absDiff(r, tc[0]) > 1 || absDiff(g, tc[1]) > 1 || absDiff(b, tc[2]) > 1 {
			t.Errorf("HSV round-trip failed for (%d,%d,%d): got (%d,%d,%d)",
				tc[0], tc[1], tc[2], r, g, b)
		}
	}
}

func absDiff(a, b uint8) uint8 {
	if a > b {
		return a - b
	}
	return b - a
}
