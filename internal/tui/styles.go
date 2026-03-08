// Package tui provides rich terminal UI components using bubbletea v2.
package tui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

var (
	// Colors
	ColorPrimary   = lipgloss.Color("#7C3AED")
	ColorSecondary = lipgloss.Color("#06B6D4")
	ColorSuccess   = lipgloss.Color("#10B981")
	ColorWarning   = lipgloss.Color("#F59E0B")
	ColorDanger    = lipgloss.Color("#EF4444")
	ColorMuted     = lipgloss.Color("#6B7280")

	// Styles
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary).
			MarginBottom(1)

	LabelStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Width(16)

	ValueStyle = lipgloss.NewStyle().
			Bold(true)

	BoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPrimary).
			Padding(0, 1)

	HelpStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			MarginTop(1)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(ColorDanger).
			Bold(true)
)

// TempColor returns a color based on temperature value.
func TempColor(temp float64) color.Color {
	switch {
	case temp >= 85:
		return ColorDanger
	case temp >= 70:
		return ColorWarning
	case temp >= 50:
		return ColorSecondary
	default:
		return ColorSuccess
	}
}

// DutyColor returns a color based on fan duty percentage.
func DutyColor(pct int) color.Color {
	switch {
	case pct >= 80:
		return ColorDanger
	case pct >= 50:
		return ColorWarning
	default:
		return ColorSuccess
	}
}
