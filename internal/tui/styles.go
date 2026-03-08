// Package tui provides rich terminal UI components using bubbletea v2.
package tui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

const (
	KeyQuit  = "q"
	KeyCtrlC = "ctrl+c"
	KeyEsc   = "esc"
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

// RenderSection renders a labeled section with a separator line.
// If active is true, the label uses ColorPrimary; otherwise ColorMuted.
func RenderSection(sb *strings.Builder, label string, active bool, width int, renderItems func(*strings.Builder)) {
	style := lipgloss.NewStyle().Bold(true)
	if active {
		style = style.Foreground(ColorPrimary)
	} else {
		style = style.Foreground(ColorMuted)
	}

	header := style.Render("  " + label)
	sepWidth := max(1, width-lipgloss.Width(header)-4)
	sep := lipgloss.NewStyle().Foreground(ColorMuted).Render(" " + strings.Repeat("─", sepWidth))
	fmt.Fprintf(sb, "%s%s\n", header, sep)

	sb.WriteString("    ")
	renderItems(sb)
	sb.WriteString("\n")
}

// RenderHelp renders a help footer with • separators.
func RenderHelp(items ...string) string {
	return HelpStyle.Render("  " + strings.Join(items, " • "))
}

// RenderFeedback renders the error or success feedback line.
func RenderFeedback(sb *strings.Builder, err error, applied string) {
	sb.WriteString("\n")
	switch {
	case err != nil:
		sb.WriteString(ErrorStyle.Render(fmt.Sprintf("  Error: %v", err)))
		sb.WriteString("\n")
	case applied != "":
		sb.WriteString(lipgloss.NewStyle().Foreground(ColorSuccess).Render(fmt.Sprintf("  ✓ %s", applied)))
		sb.WriteString("\n")
	}
}

// RenderHeader renders the standard panel header line.
func RenderHeader(sb *strings.Builder, name, device string, width int) {
	title := lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render("avellcc")
	subtitle := lipgloss.NewStyle().Foreground(ColorMuted).Render(" " + name)
	dev := lipgloss.NewStyle().Foreground(ColorSuccess).Render("● " + device)
	gap := strings.Repeat(" ", max(2, width-lipgloss.Width(title)-lipgloss.Width(subtitle)-lipgloss.Width(dev)-4))
	fmt.Fprintf(sb, "  %s%s%s%s\n\n", title, subtitle, gap, dev)
}

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
