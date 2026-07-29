package ui

import (
	"fmt"
	"os"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type reviewModel struct {
	title, subtitle, confirmLabel string
	viewport                      viewport.Model
	cursor                        int
	width, height                 int
	isDark, dangerous             bool
	submitted, confirmed          bool
	interrupted                   bool
}

func newReviewModel(title, subtitle, body, confirmLabel string, dangerous bool) reviewModel {
	title = safeSingleLine(title)
	subtitle = safeSingleLine(subtitle)
	body = safeText(body)
	confirmLabel = safeSingleLine(confirmLabel)
	content := viewport.New(viewport.WithWidth(defaultPromptWidth-4), viewport.WithHeight(defaultPromptHeight-14))
	content.SoftWrap = true
	content.FillHeight = false
	content.SetContent(body)
	return reviewModel{
		title: title, subtitle: subtitle, confirmLabel: confirmLabel,
		viewport: content, width: defaultPromptWidth, height: defaultPromptHeight,
		isDark: true, dangerous: dangerous,
	}
}

func (m reviewModel) Init() tea.Cmd {
	return tea.RequestBackgroundColor
}

func (m reviewModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = message.Width, message.Height
		m.viewport.SetWidth(scrollViewportWidth(message.Width))
		availableHeight := max(message.Height-15, 5)
		m.viewport.SetHeight(min(max(m.viewport.TotalLineCount(), 1), availableHeight))
	case tea.BackgroundColorMsg:
		m.isDark = message.IsDark()
	case tea.InterruptMsg:
		m.interrupted = true
		return m, tea.Quit
	case tea.KeyPressMsg:
		switch message.String() {
		case "left", "h", "shift+tab":
			m.cursor = 0
		case "right", "l", "tab":
			m.cursor = 1
		case "y":
			m.submitted, m.confirmed = true, true
			return m, tea.Quit
		case "n", "esc", "q":
			m.submitted = true
			return m, tea.Quit
		case "ctrl+c":
			m.interrupted = true
			return m, tea.Quit
		case "enter":
			m.submitted, m.confirmed = true, m.cursor == 1
			return m, tea.Quit
		}
	}
	var command tea.Cmd
	m.viewport, command = m.viewport.Update(message)
	return m, command
}

func (m reviewModel) View() tea.View {
	colors := themeFor(m.isDark)
	width := screenWidth(m.width)
	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colors.border).
		Padding(0, 1).
		Width(scrollPanelWidth(m.width)).
		Render(m.viewport.View())
	buttons := lipgloss.JoinHorizontal(lipgloss.Top,
		m.button(colors, 0, "Cancel"), "  ", m.button(colors, 1, m.confirmLabel),
	)
	content := lipgloss.JoinVertical(lipgloss.Left,
		renderBrand(colors, width), "",
		lipgloss.NewStyle().Bold(true).Foreground(colors.text).Render(m.title),
		lipgloss.NewStyle().Foreground(colors.muted).Width(width).Render(m.subtitle),
		"", panel, "", buttons, "",
		renderHelp(colors, "←/→", "choose", "enter", "confirm", "esc", "cancel"),
	)
	view := tea.NewView(lipgloss.NewStyle().Padding(1, 2).Render(content))
	view.AltScreen = true
	view.WindowTitle = "Local AI Lab · " + m.title
	return view
}

func (m reviewModel) button(colors theme, index int, label string) string {
	foreground := colors.text
	if index == 1 && m.dangerous {
		foreground = colors.danger
	}
	style := lipgloss.NewStyle().Foreground(foreground).Padding(0, 2)
	if m.cursor == index {
		style = style.Bold(true).Foreground(colors.accent).Background(colors.accentSoft)
		if index == 1 && m.dangerous {
			style = style.Foreground(colors.danger)
		}
	}
	return style.Render(label)
}

func (t *Terminal) Review(title, subtitle, body, confirmLabel string, dangerous bool) (bool, error) {
	if strings.TrimSpace(confirmLabel) == "" {
		return false, fmt.Errorf("review %s: confirm label is required", title)
	}
	program := tea.NewProgram(
		newReviewModel(title, subtitle, body, confirmLabel, dangerous),
		tea.WithInput(t.source), tea.WithOutput(t.output),
		tea.WithEnvironment(os.Environ()),
		tea.WithWindowSize(defaultPromptWidth, defaultPromptHeight),
	)
	final, err := program.Run()
	if err != nil {
		return false, fmt.Errorf("review %s: %w", title, err)
	}
	model, ok := final.(reviewModel)
	if !ok {
		return false, fmt.Errorf("review %s: unexpected model", title)
	}
	if model.interrupted || !model.submitted {
		return false, ErrInterrupted
	}
	return model.confirmed, nil
}
