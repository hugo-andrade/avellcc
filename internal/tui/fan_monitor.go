package tui

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/harmonica"

	"github.com/hugo-andrade/avellcc/internal/fan"
)

const (
	sparklineWidth = 40
	maxHistory     = sparklineWidth
)

type tickMsg time.Time

type FanModel struct {
	fc         *fan.FanController
	fans       fan.FanStatus
	temps      []fan.TempReading
	history    [2][]float64 // RPM history for sparklines
	dutySpring [2]harmonica.Spring
	dutyPos    [2]float64
	dutyVel    [2]float64
	width      int
	height     int
	err        error
}

func NewFanModel(fc *fan.FanController) FanModel {
	return FanModel{
		fc: fc,
		dutySpring: [2]harmonica.Spring{
			harmonica.NewSpring(harmonica.FPS(30), 5.0, 0.4),
			harmonica.NewSpring(harmonica.FPS(30), 5.0, 0.4),
		},
	}
}

func (m FanModel) Init() tea.Cmd {
	return tickCmd()
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m FanModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "+", "=":
			if err := m.fc.SetFanSpeed(0, 100); err != nil {
				m.err = err
			}
			return m, nil
		case "-":
			if err := m.fc.SetFanSpeed(0, 30); err != nil {
				m.err = err
			}
			return m, nil
		case "a":
			if err := m.fc.SetAuto(); err != nil {
				m.err = err
			}
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tickMsg:
		m.fans = m.fc.GetFanRPM()
		m.temps = m.fc.GetTemperatures()
		m.err = nil

		// Update RPM history
		for i, key := range []string{"fan1_rpm", "fan2_rpm"} {
			if rpm, ok := m.fans[key]; ok {
				m.history[i] = append(m.history[i], float64(rpm))
				if len(m.history[i]) > maxHistory {
					m.history[i] = m.history[i][len(m.history[i])-maxHistory:]
				}
			}
		}

		// Update duty springs
		for i, key := range []string{"fan1_duty_pct", "fan2_duty_pct"} {
			if pct, ok := m.fans[key]; ok {
				m.dutyPos[i], m.dutyVel[i] = m.dutySpring[i].Update(m.dutyPos[i], m.dutyVel[i], float64(pct))
			}
		}

		return m, tickCmd()
	}

	return m, nil
}

func (m FanModel) View() tea.View {
	if m.width == 0 {
		return tea.NewView("Loading...")
	}

	var sb strings.Builder

	sb.WriteString(TitleStyle.Render("  Avell Fan Monitor"))
	sb.WriteString("\n\n")

	// Fan section
	for i := 1; i <= 2; i++ {
		fanKey := fmt.Sprintf("fan%d", i)
		rpm, hasRPM := m.fans[fanKey+"_rpm"]
		dutyPct, hasDuty := m.fans[fanKey+"_duty_pct"]

		header := lipgloss.NewStyle().Bold(true).Foreground(ColorSecondary).Render(fmt.Sprintf("Fan %d", i))
		sb.WriteString(header)

		if hasRPM {
			fmt.Fprintf(&sb, "  %d RPM", rpm)
		} else {
			sb.WriteString("  ? RPM")
		}

		if hasDuty {
			clr := DutyColor(dutyPct)
			bar := renderProgressBar(int(m.dutyPos[i-1]), 30, clr)
			fmt.Fprintf(&sb, "  %s %d%%", bar, dutyPct)
		}
		sb.WriteString("\n")

		// Sparkline
		if len(m.history[i-1]) > 1 {
			sb.WriteString("  ")
			sb.WriteString(renderSparkline(m.history[i-1]))
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	// Temperature section
	if len(m.temps) > 0 {
		sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render("Temperatures"))
		sb.WriteString("\n")

		var coreTemps []float64
		for _, t := range m.temps {
			if strings.HasPrefix(t.Name, "Core ") {
				coreTemps = append(coreTemps, t.Value)
				continue
			}
			clr := TempColor(t.Value)
			name := LabelStyle.Render(t.Name)
			val := lipgloss.NewStyle().Foreground(clr).Render(fmt.Sprintf("%.1f°C", t.Value))
			fmt.Fprintf(&sb, "  %s %s\n", name, val)
		}
		if len(coreTemps) > 0 {
			min, max := coreTemps[0], coreTemps[0]
			for _, v := range coreTemps[1:] {
				if v < min {
					min = v
				}
				if v > max {
					max = v
				}
			}
			clr := TempColor(max)
			label := LabelStyle.Render(fmt.Sprintf("CPU Cores (%d)", len(coreTemps)))
			val := lipgloss.NewStyle().Foreground(clr).Render(fmt.Sprintf("%.0f-%.0f°C", min, max))
			fmt.Fprintf(&sb, "  %s %s\n", label, val)
		}
	}

	if m.err != nil {
		sb.WriteString("\n")
		sb.WriteString(ErrorStyle.Render(fmt.Sprintf("Error: %v", m.err)))
		sb.WriteString("\n")
	}

	sb.WriteString(HelpStyle.Render("  q quit • + max • - min • a auto"))
	sb.WriteString("\n")

	v := tea.NewView(sb.String())
	v.AltScreen = true
	return v
}

func renderProgressBar(pct, width int, clr color.Color) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := pct * width / 100
	empty := width - filled
	bar := lipgloss.NewStyle().Foreground(clr).Render(strings.Repeat("█", filled))
	bar += lipgloss.NewStyle().Foreground(ColorMuted).Render(strings.Repeat("░", empty))
	return bar
}

var sparkChars = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

func renderSparkline(data []float64) string {
	if len(data) == 0 {
		return ""
	}
	min, max := data[0], data[0]
	for _, v := range data {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	rng := max - min
	if rng == 0 {
		rng = 1
	}

	var sb strings.Builder
	for _, v := range data {
		idx := int((v - min) / rng * float64(len(sparkChars)-1))
		if idx >= len(sparkChars) {
			idx = len(sparkChars) - 1
		}
		sb.WriteRune(sparkChars[idx])
	}
	return lipgloss.NewStyle().Foreground(ColorSecondary).Render(sb.String())
}
