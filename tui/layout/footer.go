package layout

import (
	"strings"

	"asciishader/tui/styles"

	"charm.land/lipgloss/v2"
)

// FooterBinding represents a key binding shown in the footer bar.
type FooterBinding struct {
	Key  string
	Desc string
}

// RenderFooter renders a styled full-width footer bar with key bindings.
func RenderFooter(bindings []FooterBinding, width int, rightText string) string {
	if width == 0 {
		return ""
	}

	keyStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.ChromeFg).
		Background(styles.ChromeBg)

	descStyle := lipgloss.NewStyle().
		Foreground(styles.ChromeFgMuted).
		Background(styles.ChromeBg)

	sepStyle := lipgloss.NewStyle().
		Foreground(styles.ChromeFgAccent).
		Background(styles.ChromeBg)

	bgStyle := lipgloss.NewStyle().Background(styles.ChromeBg)

	separator := " · "
	separatorWidth := lipgloss.Width(separator)

	rightTextWidth := 0
	if rightText != "" {
		rightTextWidth = lipgloss.Width(rightText) + 3
	}

	type footerPart struct {
		text string
	}
	var parts []footerPart
	currentWidth := 1

	for _, b := range bindings {
		part := b.Key + ": " + b.Desc
		partWidth := len(part)

		newWidth := currentWidth + partWidth
		if len(parts) > 0 {
			newWidth += separatorWidth
		}

		if newWidth > width-1-rightTextWidth {
			break
		}

		parts = append(parts, footerPart{text: part})
		currentWidth = newWidth
	}

	var styledParts []string
	for _, part := range parts {
		idx := strings.Index(part.text, ": ")
		var styled string
		if idx > 0 {
			styled = keyStyle.Render(part.text[:idx]) + descStyle.Render(part.text[idx:])
		} else {
			styled = keyStyle.Render(part.text)
		}
		styledParts = append(styledParts, styled)
	}

	styledSeparator := sepStyle.Render(separator)
	leftContent := " " + strings.Join(styledParts, styledSeparator)

	var content string
	if rightText != "" {
		leftWidth := lipgloss.Width(leftContent)
		rightStyled := descStyle.Render(rightText)
		rightWidth := lipgloss.Width(rightStyled)
		padding := width - leftWidth - rightWidth - 1
		if padding < 1 {
			padding = 1
		}
		content = leftContent + bgStyle.Render(strings.Repeat(" ", padding)) + rightStyled
	} else {
		content = leftContent
	}

	barStyle := lipgloss.NewStyle().
		Background(styles.ChromeBg).
		Width(width)

	return barStyle.Render(content)
}
