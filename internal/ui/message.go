package ui

import (
	"fmt"
	"os"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type messageModel struct {
	title, subtitle string
	viewport        viewport.Model
	width, height   int
	isDark          bool
	interrupted     bool
}

func newMessageModel(title, subtitle, body string) messageModel {
	title = safeSingleLine(title)
	subtitle = safeSingleLine(subtitle)
	body = safeText(body)
	content := viewport.New(viewport.WithWidth(defaultPromptWidth-4), viewport.WithHeight(defaultPromptHeight-11))
	content.SoftWrap = true
	content.FillHeight = false
	content.SetContent(body)
	return messageModel{
		title: title, subtitle: subtitle, viewport: content,
		width: defaultPromptWidth, height: defaultPromptHeight, isDark: true,
	}
}

func (m messageModel) Init() tea.Cmd {
	return tea.RequestBackgroundColor
}

func (m messageModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = message.Width, message.Height
		m.viewport.SetWidth(scrollViewportWidth(message.Width))
		availableHeight := max(message.Height-12, 5)
		m.viewport.SetHeight(min(max(m.viewport.TotalLineCount(), 1), availableHeight))
	case tea.BackgroundColorMsg:
		m.isDark = message.IsDark()
	case tea.InterruptMsg:
		m.interrupted = true
		return m, tea.Quit
	case tea.KeyPressMsg:
		switch message.String() {
		case "enter":
			return m, tea.Quit
		case "esc", "q", "ctrl+c":
			m.interrupted = true
			return m, tea.Quit
		}
	}
	var command tea.Cmd
	m.viewport, command = m.viewport.Update(message)
	return m, command
}

func (m messageModel) View() tea.View {
	colors := themeFor(m.isDark)
	width := screenWidth(m.width)
	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colors.border).
		Padding(0, 1).
		Width(scrollPanelWidth(m.width)).
		Render(m.viewport.View())
	content := lipgloss.JoinVertical(lipgloss.Left,
		renderBrand(colors, width), "",
		lipgloss.NewStyle().Bold(true).Foreground(colors.text).Render(m.title),
		lipgloss.NewStyle().Foreground(colors.muted).MaxWidth(width).Render(m.subtitle),
		"", panel, "",
		renderHelp(colors, "↑/↓", "scroll", "enter", "return", "esc", "back"),
	)
	view := tea.NewView(lipgloss.NewStyle().Padding(1, 2).Render(content))
	view.AltScreen = true
	view.WindowTitle = "Local AI Lab · " + m.title
	return view
}

func (t *Terminal) Show(title, subtitle, body string) error {
	program := tea.NewProgram(
		newMessageModel(title, subtitle, body),
		tea.WithInput(t.source), tea.WithOutput(t.output),
		tea.WithEnvironment(os.Environ()),
		tea.WithWindowSize(defaultPromptWidth, defaultPromptHeight),
	)
	final, err := program.Run()
	if err != nil {
		return fmt.Errorf("show %s: %w", title, err)
	}
	model, ok := final.(messageModel)
	if !ok {
		return fmt.Errorf("show %s: unexpected model", title)
	}
	if model.interrupted {
		return ErrInterrupted
	}
	return nil
}
