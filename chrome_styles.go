package main

import "github.com/charmbracelet/lipgloss"

// Chrome style constants (adapted from Railway TUI styles)
var (
	ChromeBg         = lipgloss.Color("235") // Dark gray for header/footer
	ChromeBgLight    = lipgloss.Color("236") // Slightly lighter for sidebar
	ChromeBgDark     = lipgloss.Color("233") // Dark for panels
	ChromeFg         = lipgloss.Color("15")  // Bright white
	ChromeFgMuted    = lipgloss.Color("245") // Muted gray
	ChromeFgAccent   = lipgloss.Color("243") // Accent (separators)
	ChromeResizeEdge = lipgloss.Color("243") // Dim grey for panel resize edge
	TermBg           = lipgloss.Color("0")   // Terminal background (fallback)
)
