package ui

import "charm.land/lipgloss/v2"

// Styles shared by the views. lipgloss v2 styles are plain values, so they are
// built once here instead of on every View call.
var (
	titleStyle = lipgloss.NewStyle().Bold(true)

	errorBadgeStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("1")).
			Foreground(lipgloss.Color("15"))
)
