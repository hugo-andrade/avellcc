package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hugo-andrade/avellcc/internal/config"
	"github.com/hugo-andrade/avellcc/internal/keyboard"
)

type colorOption struct {
	name    string
	r, g, b byte
}

var kbColors = []colorOption{
	{"red", 255, 0, 0},
	{"orange", 255, 128, 0},
	{"yellow", 255, 255, 0},
	{"lime", 0, 255, 0},
	{"green", 0, 128, 0},
	{"cyan", 0, 255, 255},
	{"blue", 0, 0, 255},
	{"purple", 128, 0, 255},
	{"pink", 255, 100, 200},
	{"white", 255, 255, 255},
}

type effectOption struct {
	name    string
	display string
	isSW    bool
}

var kbEffects = []effectOption{
	{"none", "None", false},
	{"rainbow", "Rainbow", false},
	{"sw_rainbow", "Rainbow Wave", true},
	{"sw_breathing", "Breathing", true},
	{"sw_wave", "Color Wave", true},
}

const (
	sectionColor  = 0
	sectionEffect = 1
)

// KeyboardPanel is the interactive keyboard control TUI.
type KeyboardPanel struct {
	kb *keyboard.ITE8295

	// Current hardware state
	brightness int
	colorIdx   int
	effectIdx  int

	// UI navigation
	section int // sectionColor or sectionEffect
	cursor  int // cursor within current section

	// Effect runner for software effects
	runner *keyboard.EffectRunner

	width, height int
	err           error
	applied       string // brief feedback message
}

// NewKeyboardPanel creates the interactive keyboard panel.
func NewKeyboardPanel(kb *keyboard.ITE8295) *KeyboardPanel {
	brightness := 7
	colorIdx := 0

	bundle := config.LoadStateBundle()
	if kbState, ok := bundle["keyboard"].(map[string]any); ok {
		if b, ok := config.GetInt(kbState, "brightness"); ok {
			brightness = b
		}
	}

	return &KeyboardPanel{
		kb:         kb,
		brightness: brightness,
		colorIdx:   colorIdx,
		effectIdx:  0,
		section:    sectionColor,
		cursor:     colorIdx,
	}
}

func (m *KeyboardPanel) Init() tea.Cmd {
	return tea.RequestWindowSize
}

func (m *KeyboardPanel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			if m.runner != nil {
				m.runner.Stop()
			}
			return m, tea.Quit

		case "tab", "down", "j":
			m.section = (m.section + 1) % 2
			if m.section == sectionColor {
				m.cursor = m.colorIdx
			} else {
				m.cursor = m.effectIdx
			}

		case "shift+tab", "up", "k":
			m.section = (m.section + 1) % 2
			if m.section == sectionColor {
				m.cursor = m.colorIdx
			} else {
				m.cursor = m.effectIdx
			}

		case "left", "h":
			max := m.sectionLen()
			if m.cursor > 0 {
				m.cursor--
			} else {
				m.cursor = max - 1
			}

		case "right", "l":
			max := m.sectionLen()
			m.cursor = (m.cursor + 1) % max

		case "enter":
			m.applySelection()

		case "+", "=":
			if m.brightness < 10 {
				m.brightness++
				if err := m.kb.SetBrightness(m.brightness); err != nil {
					m.err = err
				} else {
					m.saveState()
				}
			}

		case "-":
			if m.brightness > 0 {
				m.brightness--
				if err := m.kb.SetBrightness(m.brightness); err != nil {
					m.err = err
				} else {
					m.saveState()
				}
			}

		case "o":
			if m.runner != nil {
				m.runner.Stop()
				m.runner = nil
			}
			if err := m.kb.Off(); err != nil {
				m.err = err
			} else {
				m.brightness = 0
				m.effectIdx = 0
				m.applied = "Keyboard off"
				bundle := config.LoadStateBundle()
				bundle["keyboard"] = map[string]any{"mode": "off"}
				_ = config.SaveStateBundle(bundle)
			}

		case "r":
			if m.runner != nil {
				m.runner.Stop()
				m.runner = nil
			}
			bundle := config.LoadStateBundle()
			if kbState, ok := bundle["keyboard"].(map[string]any); ok {
				if b, ok := config.GetInt(kbState, "brightness"); ok {
					m.brightness = b
				}
			}
			m.applied = "State restored"
		}
	}
	return m, nil
}

func (m *KeyboardPanel) sectionLen() int {
	if m.section == sectionColor {
		return len(kbColors)
	}
	return len(kbEffects)
}

func (m *KeyboardPanel) applySelection() {
	if m.section == sectionColor {
		c := kbColors[m.cursor]
		// Stop any running effect
		if m.runner != nil {
			m.runner.Stop()
			m.runner = nil
		}
		m.effectIdx = 0 // reset to "none"
		if err := m.kb.SetAllKeys(c.r, c.g, c.b); err != nil {
			m.err = err
			return
		}
		m.colorIdx = m.cursor
		m.applied = fmt.Sprintf("Color: %s", c.name)
		m.saveState()
	} else {
		eff := kbEffects[m.cursor]
		// Stop any running effect
		if m.runner != nil {
			m.runner.Stop()
			m.runner = nil
		}

		switch {
		case eff.name == "none":
			// Re-apply current color
			c := kbColors[m.colorIdx]
			if err := m.kb.SetAllKeys(c.r, c.g, c.b); err != nil {
				m.err = err
				return
			}
			m.effectIdx = 0
			m.applied = "Effect stopped"
		case eff.isSW:
			fn, ok := keyboard.SoftwareEffects[eff.name]
			if !ok {
				m.err = fmt.Errorf("unknown effect: %s", eff.name)
				return
			}
			m.runner = keyboard.NewEffectRunner(m.kb, 30)
			opts := keyboard.DefaultEffectOpts()
			opts.Speed = 3
			// Use current color for breathing/wave
			c := kbColors[m.colorIdx]
			opts.R, opts.G, opts.B = c.r, c.g, c.b
			m.runner.Start(fn, opts)
			m.effectIdx = m.cursor
			m.applied = fmt.Sprintf("Effect: %s", eff.display)
		default:
			// Hardware effect
			if animID, ok := keyboard.EffectNames[eff.name]; ok {
				if err := m.kb.SetHWAnimation(animID); err != nil {
					m.err = err
					return
				}
				m.effectIdx = m.cursor
				m.applied = fmt.Sprintf("Effect: %s", eff.display)
			}
		}
		m.saveState()
	}
}

func (m *KeyboardPanel) saveState() {
	bundle := config.LoadStateBundle()
	state := map[string]any{
		"brightness": float64(m.brightness),
	}

	eff := kbEffects[m.effectIdx]
	if eff.name != "none" {
		state["mode"] = "effect"
		state["effect"] = eff.name
		state["speed"] = float64(3)
	} else {
		c := kbColors[m.colorIdx]
		state["mode"] = "static"
		state["color"] = []any{float64(c.r), float64(c.g), float64(c.b)}
	}

	bundle["keyboard"] = state
	_ = config.SaveStateBundle(bundle)
}

func (m *KeyboardPanel) View() tea.View {
	if m.width == 0 {
		return tea.NewView("Loading...")
	}

	var sb strings.Builder

	RenderHeader(&sb, "keyboard", "ITE 8295", m.width)

	// Brightness bar
	brightnessLabel := lipgloss.NewStyle().Foreground(ColorMuted).Render("  Brightness")
	bar := renderBrightnessBar(m.brightness, 10, 20)
	brightnessVal := lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf(" %d", m.brightness))
	hint := lipgloss.NewStyle().Foreground(ColorMuted).Render("  +/- adjust")
	fmt.Fprintf(&sb, "%s  %s%s%s\n\n", brightnessLabel, bar, brightnessVal, hint)

	// Color section
	RenderSection(&sb, "Color", m.section == sectionColor, m.width, func(sb *strings.Builder) {
		var items []string
		for i, c := range kbColors {
			swatch := lipgloss.NewStyle().
				Background(lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", c.r, c.g, c.b))).
				Render("  ")

			name := c.name
			style := lipgloss.NewStyle()

			isActive := i == m.colorIdx && m.effectIdx == 0
			isCursor := m.section == sectionColor && i == m.cursor

			switch {
			case isCursor:
				style = style.Bold(true).Foreground(lipgloss.Color("#ffffff"))
				name = "▸ " + name
			case isActive:
				style = style.Bold(true).Foreground(ColorSecondary)
			default:
				style = style.Foreground(ColorMuted)
			}

			items = append(items, fmt.Sprintf("%s %s", swatch, style.Render(name)))
		}
		sb.WriteString(wrapItems(items, m.width-8, "    "))
	})

	sb.WriteString("\n")

	// Effect section
	RenderSection(&sb, "Effect", m.section == sectionEffect, m.width, func(sb *strings.Builder) {
		var items []string
		for i, eff := range kbEffects {
			style := lipgloss.NewStyle()

			isActive := i == m.effectIdx
			isCursor := m.section == sectionEffect && i == m.cursor

			name := eff.display
			switch {
			case isCursor:
				style = style.Bold(true).Foreground(lipgloss.Color("#ffffff"))
				name = "▸ " + name
			case isActive:
				style = style.Bold(true).Foreground(ColorSecondary)
			default:
				style = style.Foreground(ColorMuted)
			}

			items = append(items, style.Render(name))
		}
		sb.WriteString(wrapItems(items, m.width-8, "    "))
	})

	RenderFeedback(&sb, m.err, m.applied)
	sb.WriteString(RenderHelp("tab section", "← → select", "enter apply", "+/- brightness", "o off", "r restore", "q quit"))
	sb.WriteString("\n")

	v := tea.NewView(sb.String())
	v.AltScreen = true
	return v
}

func renderBrightnessBar(level, maxLevel, width int) string {
	filled := level * width / maxLevel
	empty := width - filled
	clr := ColorSuccess
	if level >= 8 {
		clr = ColorWarning
	}
	bar := lipgloss.NewStyle().Foreground(clr).Render(strings.Repeat("━", filled))
	bar += lipgloss.NewStyle().Foreground(ColorMuted).Render(strings.Repeat("─", empty))
	return bar
}

// wrapItems joins items with spacing, wrapping to new lines with indent if needed.
func wrapItems(items []string, maxWidth int, indent string) string {
	if maxWidth <= 0 {
		maxWidth = 80
	}
	var lines []string
	var current []string
	currentWidth := 0

	for _, item := range items {
		w := lipgloss.Width(item)
		spacing := 2
		if currentWidth > 0 && currentWidth+spacing+w > maxWidth {
			lines = append(lines, strings.Join(current, "  "))
			current = nil
			currentWidth = 0
		}
		if currentWidth > 0 {
			currentWidth += spacing
		}
		current = append(current, item)
		currentWidth += w
	}
	if len(current) > 0 {
		lines = append(lines, strings.Join(current, "  "))
	}

	return strings.Join(lines, "\n"+indent)
}
