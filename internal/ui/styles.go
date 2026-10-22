package ui

import "charm.land/lipgloss/v2"

// Styles shared by the views. lipgloss v2 styles are plain values, so they are
// built once here instead of on every View call.
var (
	titleStyle = lipgloss.NewStyle().Bold(true)

	// Tone styles for RenderTable and RenderDetail cells.
	goodStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	badStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	mutedStyle  = lipgloss.NewStyle().Faint(true)
	strongStyle = lipgloss.NewStyle().Bold(true)

	errorBadgeStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("1")).
			Foreground(lipgloss.Color("15"))
)
