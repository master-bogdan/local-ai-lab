package ui

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type multiSelectModel struct {
	title       string
	options     []Option
	selected    map[int]bool
	cursor      int
	width       int
	height      int
	isDark      bool
	submitted   bool
	interrupted bool
}

func newMultiSelectModel(title string, options []Option) multiSelectModel {
	selected := make(map[int]bool, len(options))
	for i, option := range options {
		selected[i] = option.Selected
	}
	return multiSelectModel{
		title: title, options: options, selected: selected,
		width: defaultPromptWidth, height: defaultPromptHeight, isDark: true,
	}
}

func (m multiSelectModel) Init() tea.Cmd {
	return tea.RequestBackgroundColor
}

func (m multiSelectModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = message.Width, message.Height
	case tea.BackgroundColorMsg:
		m.isDark = message.IsDark()
	case tea.InterruptMsg:
		m.interrupted = true
		return m, tea.Quit
	case tea.KeyPressMsg:
		switch message.String() {
		case "up", "k":
			m.move(-1)
		case "down", "j":
			m.move(1)
		case "home", "g":
			m.cursor = 0
		case "end", "G":
			m.cursor = len(m.options) - 1
		case "space", "x":
			m.selected[m.cursor] = !m.selected[m.cursor]
		case "a":
			m.toggleAll()
		case "enter":
			m.submitted = true
			return m, tea.Quit
		case "esc", "ctrl+c":
			m.interrupted = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *multiSelectModel) move(offset int) {
	if len(m.options) == 0 {
		return
	}
	m.cursor = (m.cursor + offset + len(m.options)) % len(m.options)
}

func (m *multiSelectModel) toggleAll() {
	selectAll := false
	for i := range m.options {
		if !m.selected[i] {
			selectAll = true
			break
		}
	}
	for i := range m.options {
		m.selected[i] = selectAll
	}
}

func (m multiSelectModel) View() tea.View {
	colors := themeFor(m.isDark)
	width := screenWidth(m.width)
	help := renderHelp(colors, "↑/↓", "navigate", "space", "toggle", "a", "all", "enter", "continue", "esc", "back")
	if width < 58 {
		help = lipgloss.NewStyle().Width(width).Render(
			renderHelp(colors, "↑/↓", "move", "space", "toggle", "enter", "done", "esc", "back"),
		)
	}
	content := lipgloss.JoinVertical(lipgloss.Left,
		renderBrand(colors, width),
		"",
		lipgloss.NewStyle().Bold(true).Foreground(colors.text).Render(m.title),
		lipgloss.NewStyle().Foreground(colors.muted).Render("Select one or more options"),
		"",
		m.renderOptions(colors, width),
		"",
		help,
	)
	view := tea.NewView(lipgloss.NewStyle().Padding(1, 2).Render(content))
	view.AltScreen = true
	view.WindowTitle = "Local AI Lab"
	return view
}

func (m multiSelectModel) renderOptions(colors theme, width int) string {
	rows := make([]string, 0, len(m.options))
	for i, option := range m.options {
		check := "[ ]"
		if m.selected[i] {
			check = lipgloss.NewStyle().Foreground(colors.success).Render("[x]")
		}
		marker := "  "
		style := lipgloss.NewStyle().Foreground(colors.text).Padding(0, 1).Width(width)
		if i == m.cursor {
			marker = "› "
			style = style.Background(colors.accentSoft)
		}
		label := marker + check + " " + option.Label
		rows = append(rows, style.Render(label))
		if option.Description != "" && width >= 48 && m.height >= 26 {
			description := lipgloss.NewStyle().Foreground(colors.muted).PaddingLeft(7).MaxWidth(width - 7).Render(option.Description)
			rows = append(rows, description)
		}
	}
	return strings.Join(rows, "\n")
}

func (t *Terminal) MultiSelect(title string, options []Option) ([]string, error) {
	title = safeSingleLine(title)
	options = safeOptions(options)
	if len(options) == 0 {
		return nil, fmt.Errorf("%s has no options", title)
	}
	program := tea.NewProgram(
		newMultiSelectModel(title, options),
		tea.WithInput(t.source), tea.WithOutput(t.output),
		tea.WithEnvironment(os.Environ()),
		tea.WithWindowSize(defaultPromptWidth, defaultPromptHeight),
	)
	final, err := program.Run()
	if err != nil {
		return nil, fmt.Errorf("run %s prompt: %w", title, err)
	}
	model, ok := final.(multiSelectModel)
	if !ok {
		return nil, fmt.Errorf("run %s prompt: unexpected model", title)
	}
	if model.interrupted || !model.submitted {
		return nil, ErrInterrupted
	}
	selected := make([]string, 0, len(options))
	for i, option := range model.options {
		if model.selected[i] {
			selected = append(selected, option.Value)
		}
	}
	return selected, nil
}
