package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hugo-andrade/avellcc/internal/config"
	"github.com/hugo-andrade/avellcc/internal/lightbar"
)

type lbEffectItem struct {
	name string
	code byte
}

type lbColorItem struct {
	name string
	id   byte
	hex  string
}

var lbEffects = []lbEffectItem{
	{"static", 0x05},
	{"breathe", 0x06},
	{"wave", 0x07},
	{"change-color", 0x08},
	{"granular", 0x09},
	{"color-wave", 0x0A},
}

var lbColorItems = []lbColorItem{
	{"red", 0x01, "#ff0000"},
	{"yellow", 0x02, "#ffff00"},
	{"lime", 0x03, "#80ff00"},
	{"green", 0x04, "#00ff00"},
	{"cyan", 0x05, "#00ffff"},
	{"blue", 0x06, "#0000ff"},
	{"purple", 0x07, "#8000ff"},
}

const (
	lbSectionEffect     = 0
	lbSectionColor      = 1
	lbSectionBrightness = 2
	lbSectionSpeed      = 3
	lbSectionCount      = 4
)

// LightbarPanel is the interactive lightbar control TUI.
type LightbarPanel struct {
	lb *lightbar.ITE8911

	// Current state
	effectIdx  int
	colorIdx   int
	brightness int // 0-4
	speed      int // 1-5

	// UI
	section int
	err     error
	applied string

	width, height int
}

// NewLightbarPanel creates the interactive lightbar panel.
func NewLightbarPanel(lb *lightbar.ITE8911) *LightbarPanel {
	effectIdx := 0
	colorIdx := 0
	brightness := lightbar.X58DefaultBrightness
	speed := lightbar.X58DefaultSpeed

	// Load saved state
	bundle := config.LoadStateBundle()
	if lbState, ok := bundle["lightbar"].(map[string]any); ok {
		merged := config.MergeLightbarState(lbState, nil)
		if ec, ok := config.GetInt(merged, "effect_code"); ok {
			for i, e := range lbEffects {
				if e.code == byte(ec) {
					effectIdx = i
					break
				}
			}
		}
		if ci, ok := config.GetInt(merged, "color_id"); ok {
			for i, c := range lbColorItems {
				if c.id == byte(ci) {
					colorIdx = i
					break
				}
			}
		}
		if b, ok := config.GetInt(merged, "brightness"); ok {
			brightness = b
		}
		if s, ok := config.GetInt(merged, "speed"); ok {
			speed = s
		}
	}

	return &LightbarPanel{
		lb:         lb,
		effectIdx:  effectIdx,
		colorIdx:   colorIdx,
		brightness: brightness,
		speed:      speed,
		section:    lbSectionEffect,
	}
}

func (m *LightbarPanel) Init() tea.Cmd {
	return tea.RequestWindowSize
}

func (m *LightbarPanel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyPressMsg:
		m.applied = ""
		m.err = nil

		switch msg.String() {
		case KeyQuit, KeyCtrlC, KeyEsc:
			return m, tea.Quit

		case "tab", "down", "j":
			m.section = (m.section + 1) % lbSectionCount

		case "shift+tab", "up", "k":
			m.section = (m.section - 1 + lbSectionCount) % lbSectionCount

		case "left", "h":
			m.adjustLeft()

		case "right", "l":
			m.adjustRight()

		case "enter":
			m.applyAll()

		case "o":
			if err := m.lb.X58Off(); err != nil {
				m.err = err
			} else {
				m.applied = "Lightbar off"
				_ = config.SaveLightbarState(map[string]any{"mode": "off"})
			}

		case "r":
			bundle := config.LoadStateBundle()
			if lbState, ok := bundle["lightbar"].(map[string]any); ok {
				merged := config.MergeLightbarState(lbState, nil)
				// Reload indices
				if ec, ok := config.GetInt(merged, "effect_code"); ok {
					for i, e := range lbEffects {
						if e.code == byte(ec) {
							m.effectIdx = i
							break
						}
					}
				}
				if ci, ok := config.GetInt(merged, "color_id"); ok {
					for i, c := range lbColorItems {
						if c.id == byte(ci) {
							m.colorIdx = i
							break
						}
					}
				}
				if b, ok := config.GetInt(merged, "brightness"); ok {
					m.brightness = b
				}
				if s, ok := config.GetInt(merged, "speed"); ok {
					m.speed = s
				}
			}
			m.applyAll()
			m.applied = "State restored"
		}
	}
	return m, nil
}

func (m *LightbarPanel) adjustLeft() {
	switch m.section {
	case lbSectionEffect:
		if m.effectIdx > 0 {
			m.effectIdx--
		} else {
			m.effectIdx = len(lbEffects) - 1
		}
	case lbSectionColor:
		if m.colorIdx > 0 {
			m.colorIdx--
		} else {
			m.colorIdx = len(lbColorItems) - 1
		}
	case lbSectionBrightness:
		if m.brightness > 0 {
			m.brightness--
			m.applyAll()
		}
	case lbSectionSpeed:
		if m.speed > 1 {
			m.speed--
			m.applyAll()
		}
	}
}

func (m *LightbarPanel) adjustRight() {
	switch m.section {
	case lbSectionEffect:
		m.effectIdx = (m.effectIdx + 1) % len(lbEffects)
	case lbSectionColor:
		m.colorIdx = (m.colorIdx + 1) % len(lbColorItems)
	case lbSectionBrightness:
		if m.brightness < 4 {
			m.brightness++
			m.applyAll()
		}
	case lbSectionSpeed:
		if m.speed < 5 {
			m.speed++
			m.applyAll()
		}
	}
}

func (m *LightbarPanel) applyAll() {
	eff := lbEffects[m.effectIdx]
	clr := lbColorItems[m.colorIdx]
	ec := eff.code
	ci := clr.id
	br := m.brightness
	sp := byte(m.speed)

	if err := m.lb.X58Apply(&ec, &ci, &br, &sp); err != nil {
		m.err = err
		return
	}

	// Save state
	state := map[string]any{
		"mode":        "active",
		"effect":      eff.name,
		"effect_code": float64(ec),
		"color_id":    float64(ci),
		"brightness":  float64(br),
		"speed":       float64(m.speed),
	}
	_ = config.SaveLightbarState(state)

	if m.applied == "" {
		m.applied = fmt.Sprintf("Applied: %s / %s / bright=%d / speed=%d", eff.name, clr.name, br, m.speed)
	}
}

func (m *LightbarPanel) View() tea.View {
	if m.width == 0 {
		return tea.NewView("Loading...")
	}

	var sb strings.Builder

	RenderHeader(&sb, "lightbar", "ITE 8911", m.width)

	// Current state summary — clean inline format
	eff := lbEffects[m.effectIdx]
	clr := lbColorItems[m.colorIdx]
	swatch := lipgloss.NewStyle().
		Background(lipgloss.Color(clr.hex)).
		Render("  ")

	lbl := lipgloss.NewStyle().Foreground(ColorMuted)
	val := lipgloss.NewStyle().Bold(true)

	fmt.Fprintf(&sb, "  %s %s  %s  %s %s  %s %s\n\n",
		swatch,
		val.Render(eff.name),
		lbl.Render(clr.name),
		lbl.Render("brightness"), val.Render(fmt.Sprintf("%d", m.brightness)),
		lbl.Render("speed"), val.Render(fmt.Sprintf("%d", m.speed)),
	)

	// Effect section
	RenderSection(&sb, "Effect", m.section == lbSectionEffect, m.width, func(sb *strings.Builder) {
		var items []string
		for i, e := range lbEffects {
			style := lipgloss.NewStyle()
			name := e.name
			isCursor := m.section == lbSectionEffect && i == m.effectIdx

			switch {
			case isCursor:
				style = style.Bold(true).Foreground(lipgloss.Color("#ffffff"))
				name = "▸ " + name
			case i == m.effectIdx:
				style = style.Bold(true).Foreground(ColorSecondary)
			default:
				style = style.Foreground(ColorMuted)
			}
			items = append(items, style.Render(name))
		}
		sb.WriteString(wrapItems(items, m.width-8, "    "))
	})

	sb.WriteString("\n")

	// Color section
	RenderSection(&sb, "Color", m.section == lbSectionColor, m.width, func(sb *strings.Builder) {
		var items []string
		for i, c := range lbColorItems {
			sw := lipgloss.NewStyle().
				Background(lipgloss.Color(c.hex)).
				Render("  ")
			style := lipgloss.NewStyle()
			name := c.name
			isCursor := m.section == lbSectionColor && i == m.colorIdx

			switch {
			case isCursor:
				style = style.Bold(true).Foreground(lipgloss.Color("#ffffff"))
				name = "▸ " + name
			case i == m.colorIdx:
				style = style.Bold(true).Foreground(ColorSecondary)
			default:
				style = style.Foreground(ColorMuted)
			}
			items = append(items, fmt.Sprintf("%s %s", sw, style.Render(name)))
		}
		sb.WriteString(wrapItems(items, m.width-8, "    "))
	})

	sb.WriteString("\n")

	// Brightness section
	RenderSection(&sb, "Brightness", m.section == lbSectionBrightness, m.width, func(sb *strings.Builder) {
		bar := renderBrightnessBar(m.brightness, 4, 16)
		bval := lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf(" %d/4", m.brightness))
		hint := lipgloss.NewStyle().Foreground(ColorMuted).Render("  ← → adjust")
		fmt.Fprintf(sb, "%s%s%s", bar, bval, hint)
	})

	sb.WriteString("\n")

	// Speed section
	RenderSection(&sb, "Speed", m.section == lbSectionSpeed, m.width, func(sb *strings.Builder) {
		bar := renderBrightnessBar(m.speed, 5, 16)
		sval := lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf(" %d/5", m.speed))
		hint := lipgloss.NewStyle().Foreground(ColorMuted).Render("  ← → adjust")
		fmt.Fprintf(sb, "%s%s%s", bar, sval, hint)
	})

	RenderFeedback(&sb, m.err, m.applied)
	sb.WriteString(RenderHelp("↑↓ section", "← → select", "enter apply", "o off", "r restore", "q quit"))
	sb.WriteString("\n")

	v := tea.NewView(sb.String())
	v.AltScreen = true
	return v
}
