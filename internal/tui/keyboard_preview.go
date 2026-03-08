package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/hugo-andrade/avellcc/internal/keyboard"
)

// KeyboardModel displays a visual keyboard layout with colors.
type KeyboardModel struct {
	keymap map[string][2]int
	grid   [keyboard.GridRows][keyboard.GridCols]string // key name at each position
	colors map[string][3]byte                           // key name -> RGB
	width  int
	height int
}

func NewKeyboardModel(keymap map[string][2]int, colors map[string][3]byte) KeyboardModel {
	m := KeyboardModel{
		keymap: keymap,
		colors: colors,
	}
	// Build reverse grid
	for name, pos := range keymap {
		if pos[0] >= 0 && pos[0] < keyboard.GridRows && pos[1] >= 0 && pos[1] < keyboard.GridCols {
			m.grid[pos[0]][pos[1]] = name
		}
	}
	return m
}

func (m KeyboardModel) Init() tea.Cmd {
	return tea.RequestWindowSize
}

func (m KeyboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case KeyQuit, KeyCtrlC, KeyEsc:
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}
	return m, nil
}

func (m KeyboardModel) View() tea.View {
	var sb strings.Builder
	sb.WriteString(TitleStyle.Render("  Keyboard Layout"))
	sb.WriteString("\n\n")

	cellWidth := 6

	for row := 0; row < keyboard.GridRows; row++ {
		var cells []string
		for col := 0; col < keyboard.GridCols; col++ {
			name := m.grid[row][col]
			if name == "" {
				cells = append(cells, strings.Repeat(" ", cellWidth))
				continue
			}

			// Truncate/pad name
			display := name
			if len(display) > cellWidth-1 {
				display = display[:cellWidth-1]
			}
			display = fmt.Sprintf("%-*s", cellWidth, display)

			style := lipgloss.NewStyle()
			if rgb, ok := m.colors[name]; ok {
				style = style.Background(lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", rgb[0], rgb[1], rgb[2])))
				// Choose contrasting text
				brightness := int(rgb[0])*299 + int(rgb[1])*587 + int(rgb[2])*114
				if brightness > 128000 {
					style = style.Foreground(lipgloss.Color("#000000"))
				} else {
					style = style.Foreground(lipgloss.Color("#ffffff"))
				}
			} else {
				style = style.Foreground(ColorMuted)
			}

			cells = append(cells, style.Render(display))
		}
		sb.WriteString("  ")
		sb.WriteString(strings.Join(cells, ""))
		sb.WriteString("\n")
	}

	sb.WriteString(HelpStyle.Render("\n  q quit"))
	sb.WriteString("\n")

	return tea.NewView(sb.String())
}
