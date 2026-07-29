package ui

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	defaultPromptWidth  = 80
	defaultPromptHeight = 28
)

type selectModel struct {
	screen      menuScreen
	options     []Option
	cursor      int
	width       int
	height      int
	isDark      bool
	selected    bool
	interrupted bool
}

type menuScreen struct {
	title, subtitle, status string
	details                 []menuDetail
	compactDetails          []menuDetail
	showLogo                bool
}

type menuDetail struct {
	label, value string
}

func newSelectModel(screen menuScreen, options []Option, defaultValue string) selectModel {
	cursor := 0
	for i, option := range options {
		if option.Value == defaultValue {
			cursor = i
			break
		}
	}
	return selectModel{
		screen: screen, options: options, cursor: cursor,
		width: defaultPromptWidth, height: defaultPromptHeight, isDark: true,
	}
}

func (m selectModel) Init() tea.Cmd {
	return tea.RequestBackgroundColor
}

func (m selectModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = message.Width, message.Height
	case tea.BackgroundColorMsg:
		m.isDark = message.IsDark()
	case tea.InterruptMsg:
		m.interrupted = true
		return m, tea.Quit
	case tea.KeyPressMsg:
		if m.selectShortcut(message.String()) {
			return m, tea.Quit
		}
		switch message.String() {
		case "up", "k":
			m.move(-1)
		case "down", "j":
			m.move(1)
		case "home", "g":
			m.cursor = 0
		case "end", "G":
			m.cursor = len(m.options) - 1
		case "enter":
			m.selected = true
			return m, tea.Quit
		case "esc", "q", "ctrl+c":
			m.interrupted = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *selectModel) selectShortcut(key string) bool {
	for i, option := range m.options {
		if option.Shortcut == key {
			m.cursor = i
			m.selected = true
			return true
		}
	}
	return false
}

func (m *selectModel) move(offset int) {
	if len(m.options) == 0 {
		return
	}
	m.cursor = (m.cursor + offset + len(m.options)) % len(m.options)
}

func (m selectModel) View() tea.View {
	colors := themeFor(m.isDark)
	width := screenWidth(m.width)
	compact := m.height < 34 && len(m.screen.compactDetails) > 0
	showLogo := m.screen.showLogo && m.width >= 70 && m.height >= 24
	sections := []string{
		renderBrand(colors, width),
	}
	if showLogo {
		sections = append(sections, "", lipgloss.NewStyle().Foreground(colors.accent).Render(asciiLogo))
	}
	sections = append(sections, "", m.renderHeading(colors, width, compact))
	details := m.screen.details
	if compact {
		details = m.screen.compactDetails
	}
	if showLogo && m.height < 30 {
		details = nil
	}
	if len(details) > 0 {
		if !compact {
			sections = append(sections, "")
		}
		sections = append(sections, m.renderDetails(colors, width, details))
	}
	if compact {
		sections = append(sections, m.renderOptions(colors, width))
	} else {
		sections = append(sections,
			"", lipgloss.NewStyle().Bold(true).Foreground(colors.muted).Render("ACTIONS"),
			m.renderOptions(colors, width),
		)
	}
	sections = append(sections, "", m.renderHelp(colors, width))
	content := lipgloss.JoinVertical(lipgloss.Left, sections...)

	view := tea.NewView(lipgloss.NewStyle().Padding(1, 2).Render(content))
	view.AltScreen = true
	view.WindowTitle = "Local AI Lab"
	return view
}

func (m selectModel) renderHeading(colors theme, width int, compact bool) string {
	title := lipgloss.NewStyle().Bold(true).Foreground(colors.text).Render(m.screen.title)
	if m.screen.status != "" {
		statusColor := colors.accent
		switch {
		case strings.Contains(strings.ToLower(m.screen.status), "running"):
			statusColor = colors.success
		case strings.Contains(strings.ToLower(m.screen.status), "partial"),
			strings.Contains(strings.ToLower(m.screen.status), "required"):
			statusColor = colors.warning
		}
		status := lipgloss.NewStyle().Bold(true).Foreground(statusColor).Render(m.screen.status)
		space := strings.Repeat(" ", max(width-lipgloss.Width(title)-lipgloss.Width(status), 1))
		title += space + status
	}
	if m.screen.subtitle == "" || compact {
		return title
	}
	subtitle := lipgloss.NewStyle().Foreground(colors.muted).Width(width).Render(m.screen.subtitle)
	return lipgloss.JoinVertical(lipgloss.Left, title, subtitle)
}

func (m selectModel) renderDetails(colors theme, width int, details []menuDetail) string {
	if width < 64 {
		rows := make([]string, 0, len(details))
		for _, detail := range details {
			label := lipgloss.NewStyle().Foreground(colors.muted).Width(12).Render(detail.label)
			value := lipgloss.NewStyle().Foreground(colors.text).MaxWidth(width - 13).Render(detail.value)
			rows = append(rows, label+" "+value)
		}
		return strings.Join(rows, "\n")
	}

	columnWidth := (width - 3) / 2
	rows := make([]string, 0, (len(details)+1)/2)
	for i := 0; i < len(details); i += 2 {
		left := renderDetail(colors, details[i], columnWidth)
		right := ""
		if i+1 < len(details) {
			right = renderDetail(colors, details[i+1], columnWidth)
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, left, "   ", right))
	}
	return strings.Join(rows, "\n")
}

func (m selectModel) renderHelp(colors theme, width int) string {
	if width < 58 {
		return renderHelp(colors, "↑/↓", "move", "enter", "select", "esc", "back")
	}
	return renderHelp(colors, "↑/↓ or j/k", "navigate", "enter", "select", "esc", "back")
}

func renderDetail(colors theme, detail menuDetail, width int) string {
	label := lipgloss.NewStyle().Foreground(colors.muted).Render(strings.ToUpper(detail.label))
	value := lipgloss.NewStyle().Foreground(colors.text).MaxWidth(width).Render(detail.value)
	return lipgloss.NewStyle().Width(width).Render(label + "\n" + value)
}

func (m selectModel) renderOptions(colors theme, width int) string {
	rows := make([]string, 0, len(m.options))
	for i, option := range m.options {
		label := option.Label
		marker := "  "
		style := lipgloss.NewStyle().Foreground(colors.text).Padding(0, 1).Width(width)
		if i == m.cursor {
			marker = "› "
			style = style.Bold(true).Foreground(colors.accent).Background(colors.accentSoft)
		}
		row := marker + label
		if option.Description != "" && width >= 58 {
			descriptionColumn := min(36, width/2)
			available := max(width-descriptionColumn-4, 12)
			descriptionText := ansi.Truncate(option.Description, available, "…")
			description := lipgloss.NewStyle().Foreground(colors.muted).Render(descriptionText)
			row += strings.Repeat(" ", max(descriptionColumn-lipgloss.Width(row), 2)) + description
		}
		rows = append(rows, style.Render(row))
	}
	return strings.Join(rows, "\n")
}

func renderBrand(colors theme, width int) string {
	name := lipgloss.NewStyle().Bold(true).Foreground(colors.accent).Render("LOCAL AI LAB")
	owner := lipgloss.NewStyle().Foreground(colors.muted).Render("bogdanlabs.dev")
	space := strings.Repeat(" ", max(width-lipgloss.Width(name)-lipgloss.Width(owner), 1))
	line := name + space + owner
	return lipgloss.NewStyle().BorderBottom(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(colors.border).Width(width).Render(line)
}

func renderHelp(colors theme, bindings ...string) string {
	parts := make([]string, 0, len(bindings)/2)
	for i := 0; i+1 < len(bindings); i += 2 {
		key := lipgloss.NewStyle().Bold(true).Foreground(colors.text).Render(bindings[i])
		label := lipgloss.NewStyle().Foreground(colors.muted).Render(bindings[i+1])
		parts = append(parts, fmt.Sprintf("%s %s", key, label))
	}
	return strings.Join(parts, lipgloss.NewStyle().Foreground(colors.border).Render("  ·  "))
}

func (t *Terminal) runSelect(title string, options []Option, defaultValue string) (string, error) {
	return t.runMenu(menuScreen{title: title, subtitle: "Choose an action"}, options, defaultValue)
}

func (t *Terminal) runMenu(screen menuScreen, options []Option, defaultValue string) (string, error) {
	screen = safeMenuScreen(screen)
	options = safeOptions(options)
	if len(options) == 0 {
		return "", fmt.Errorf("%s has no options", screen.title)
	}
	program := tea.NewProgram(
		newSelectModel(screen, options, defaultValue),
		tea.WithInput(t.source), tea.WithOutput(t.output),
		tea.WithEnvironment(os.Environ()),
		tea.WithWindowSize(defaultPromptWidth, defaultPromptHeight),
	)
	final, err := program.Run()
	if err != nil {
		return "", fmt.Errorf("run %s prompt: %w", screen.title, err)
	}
	model, ok := final.(selectModel)
	if !ok {
		return "", fmt.Errorf("run %s prompt: unexpected model", screen.title)
	}
	if model.interrupted || !model.selected {
		return "", ErrInterrupted
	}
	return model.options[model.cursor].Value, nil
}

func safeMenuScreen(screen menuScreen) menuScreen {
	screen.title = safeSingleLine(screen.title)
	screen.subtitle = safeSingleLine(screen.subtitle)
	screen.status = safeSingleLine(screen.status)
	screen.details = safeDetails(screen.details)
	screen.compactDetails = safeDetails(screen.compactDetails)
	return screen
}

func safeDetails(details []menuDetail) []menuDetail {
	safe := append([]menuDetail(nil), details...)
	for index := range safe {
		safe[index].label = safeSingleLine(safe[index].label)
		safe[index].value = safeSingleLine(safe[index].value)
	}
	return safe
}

func (t *Terminal) OnboardingMenu(options []Option, defaultValue string) (string, error) {
	return t.runMenu(menuScreen{
		title:    "Private AI workstation",
		subtitle: "Local models, coding agents, web search, workspace knowledge, and images.",
		status:   "SETUP REQUIRED",
		showLogo: true,
		details: []menuDetail{
			{label: "Access", value: "localhost only"},
			{label: "Cost", value: "$0 local inference"},
			{label: "Platforms", value: "Linux · Apple Silicon"},
			{label: "Entry point", value: "make start"},
		},
	}, options, defaultValue)
}

func (t *Terminal) InstallationMenu(summary InstallationSummary, options []Option, defaultValue string) (string, error) {
	details := []menuDetail{
		{label: "System", value: summary.Platform},
		{label: "Accelerator", value: summary.GPU},
		{label: "Runtime", value: strings.ToUpper(summary.Runtime)},
		{label: "Memory", value: fmt.Sprintf("%s VRAM · %s RAM", formatBytes(summary.VRAMBytes), formatBytes(summary.RAMBytes))},
		{label: "Storage", value: fmt.Sprintf("%s used · %s free", formatBytes(summary.DataBytes), formatBytes(summary.FreeDiskBytes))},
		{label: "Models", value: fmt.Sprintf("%d models", summary.Models)},
		{label: "Inference", value: fmt.Sprintf("%s · %s context", summary.ModelProfile, formatContext(summary.ContextLength))},
		{label: "Services", value: summary.Services},
		{label: "Modules", value: summary.Modules},
		{label: "Workload", value: summary.Workload},
	}
	return t.runMenu(menuScreen{
		title:    "Local AI control center",
		subtitle: summary.DataDir,
		status:   strings.ToUpper(summary.Status),
		details:  details,
		compactDetails: []menuDetail{
			{label: "System", value: summary.Platform + " · " + strings.ToUpper(summary.Runtime) + " · " + summary.Workload},
			{label: "GPU", value: summary.GPU + " · " + formatBytes(summary.VRAMBytes)},
			{label: "Lab", value: fmt.Sprintf("%d models · %s", summary.Models, summary.Services)},
			{label: "Inference", value: fmt.Sprintf("%s · %s context", summary.ModelProfile, formatContext(summary.ContextLength))},
		},
	}, options, defaultValue)
}
