package ui

import (
	"fmt"
	"os"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type inputModel struct {
	title       string
	field       textinput.Model
	width       int
	isDark      bool
	submitted   bool
	interrupted bool
}

func newInputModel(title, defaultValue string) inputModel {
	field := textinput.New()
	field.Prompt = "› "
	field.SetValue(defaultValue)
	field.CursorEnd()
	field.SetWidth(defaultPromptWidth - 10)
	field.Focus()
	return inputModel{
		title: title, field: field, width: defaultPromptWidth, isDark: true,
	}
}

func (m inputModel) Init() tea.Cmd {
	return tea.RequestBackgroundColor
}

func (m inputModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		m.width = message.Width
		m.field.SetWidth(max(message.Width-10, 20))
	case tea.BackgroundColorMsg:
		m.isDark = message.IsDark()
		m.field.SetStyles(textinput.DefaultStyles(m.isDark))
	case tea.InterruptMsg:
		m.interrupted = true
		return m, tea.Quit
	case tea.KeyPressMsg:
		switch message.String() {
		case "enter":
			m.submitted = true
			return m, tea.Quit
		case "esc", "ctrl+c":
			m.interrupted = true
			return m, tea.Quit
		}
	}

	var command tea.Cmd
	m.field, command = m.field.Update(message)
	return m, command
}

func (m inputModel) View() tea.View {
	colors := themeFor(m.isDark)
	width := screenWidth(m.width)
	fieldWidth := max(width-4, 20)
	field := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colors.border).
		Padding(0, 1).
		Width(fieldWidth).
		Render(m.field.View())
	content := lipgloss.JoinVertical(lipgloss.Left,
		renderBrand(colors, width),
		"",
		lipgloss.NewStyle().Bold(true).Foreground(colors.text).Render(m.title),
		lipgloss.NewStyle().Foreground(colors.muted).Render("Enter a value"),
		"",
		field,
		"",
		renderHelp(colors, "enter", "continue", "ctrl+u", "clear", "esc", "back"),
	)
	view := tea.NewView(lipgloss.NewStyle().Padding(1, 2).Render(content))
	view.AltScreen = true
	view.WindowTitle = "Local AI Lab"
	return view
}

func (t *Terminal) runInput(title, defaultValue string) (string, error) {
	title = safeSingleLine(title)
	defaultValue = safeSingleLine(defaultValue)
	program := tea.NewProgram(
		newInputModel(title, defaultValue),
		tea.WithInput(t.source), tea.WithOutput(t.output),
		tea.WithEnvironment(os.Environ()),
		tea.WithWindowSize(defaultPromptWidth, defaultPromptHeight),
	)
	final, err := program.Run()
	if err != nil {
		return "", fmt.Errorf("run %s input: %w", title, err)
	}
	model, ok := final.(inputModel)
	if !ok {
		return "", fmt.Errorf("run %s input: unexpected model", title)
	}
	if model.interrupted || !model.submitted {
		return "", ErrInterrupted
	}
	return strings.TrimSpace(safeSingleLine(model.field.Value())), nil
}
