package ui

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/master-bogdan/local-ai-lab/internal/models"
)

type modelFilter int

const (
	allModels modelFilter = iota
	codingModels
	reasoningModels
	visionModels
	embeddingModels
)

var modelFilterLabels = [...]string{"ALL", "CODING", "REASONING", "VISION", "RETRIEVAL"}

type modelPickerModel struct {
	models      []models.Model
	selected    map[int]bool
	cursor      int
	filter      modelFilter
	width       int
	height      int
	isDark      bool
	submitted   bool
	interrupted bool
	animate     bool
	frame       int
}

type modelPickerTick struct{}

func newModelPickerModel(catalog []models.Model) modelPickerModel {
	selected := make(map[int]bool, len(catalog))
	cursor := 0
	foundSelected := false
	for index, model := range catalog {
		compatible := model.Compatible || model.Fit != models.Unsupported
		catalog[index].Compatible = compatible
		selected[index] = model.Selected && compatible
		if model.Selected && compatible && !foundSelected {
			cursor = index
			foundSelected = true
		}
	}
	return modelPickerModel{
		models: catalog, selected: selected, cursor: cursor,
		width: defaultPromptWidth, height: defaultPromptHeight, isDark: true,
		animate: os.Getenv("NO_COLOR") == "" && os.Getenv("LOCAL_AI_REDUCE_MOTION") == "",
	}
}

func (m modelPickerModel) Init() tea.Cmd {
	if !m.animate {
		return tea.RequestBackgroundColor
	}
	return tea.Batch(tea.RequestBackgroundColor, modelPickerAnimation())
}

func modelPickerAnimation() tea.Cmd {
	return tea.Tick(650*time.Millisecond, func(time.Time) tea.Msg { return modelPickerTick{} })
}

func (m modelPickerModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = message.Width, message.Height
	case tea.BackgroundColorMsg:
		m.isDark = message.IsDark()
	case tea.InterruptMsg:
		m.interrupted = true
		return m, tea.Quit
	case modelPickerTick:
		if m.animate {
			m.frame++
			return m, modelPickerAnimation()
		}
	case tea.KeyPressMsg:
		switch message.String() {
		case "up", "k":
			m.move(-1)
		case "down", "j":
			m.move(1)
		case "home", "g":
			m.moveToEdge(false)
		case "end", "G":
			m.moveToEdge(true)
		case "tab":
			m.filter = (m.filter + 1) % modelFilter(len(modelFilterLabels))
			m.ensureVisibleCursor()
		case "shift+tab":
			m.filter = (m.filter + modelFilter(len(modelFilterLabels)) - 1) % modelFilter(len(modelFilterLabels))
			m.ensureVisibleCursor()
		case "space", "x":
			if m.models[m.cursor].Compatible {
				m.selected[m.cursor] = !m.selected[m.cursor]
			}
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

func (m *modelPickerModel) move(offset int) {
	visible := m.visibleIndexes()
	if len(visible) == 0 {
		return
	}
	position := 0
	for index, candidate := range visible {
		if candidate == m.cursor {
			position = index
			break
		}
	}
	m.cursor = visible[(position+offset+len(visible))%len(visible)]
}

func (m *modelPickerModel) moveToEdge(end bool) {
	visible := m.visibleIndexes()
	if len(visible) == 0 {
		return
	}
	m.cursor = visible[0]
	if end {
		m.cursor = visible[len(visible)-1]
	}
}

func (m *modelPickerModel) ensureVisibleCursor() {
	visible := m.visibleIndexes()
	if len(visible) == 0 {
		return
	}
	for _, index := range visible {
		if index == m.cursor {
			return
		}
	}
	m.cursor = visible[0]
}

func (m modelPickerModel) visibleIndexes() []int {
	visible := make([]int, 0, len(m.models))
	for index, model := range m.models {
		switch m.filter {
		case codingModels:
			if model.Kind != models.CodingModel {
				continue
			}
		case reasoningModels:
			if model.Kind != models.Chat {
				continue
			}
		case visionModels:
			if model.Kind != models.VisionModel {
				continue
			}
		case embeddingModels:
			if model.Kind != models.Embedding {
				continue
			}
		}
		visible = append(visible, index)
	}
	return visible
}

func (m modelPickerModel) View() tea.View {
	colors := themeFor(m.isDark)
	width := screenWidth(m.width)
	sections := []string{
		renderBrand(colors, width),
		"",
		lipgloss.NewStyle().Bold(true).Foreground(colors.text).Render("Recommended models"),
		lipgloss.NewStyle().Foreground(colors.muted).MaxWidth(width).Render("Preset for this hardware; customize before download"),
		m.renderSummary(colors),
		m.renderFilters(colors, width),
		"",
		m.renderPicker(colors, width),
		"",
	}
	help := renderHelp(colors, "↑/↓", "navigate", "space", "toggle", "tab", "filter", "enter", "continue", "esc", "back")
	if width < 70 {
		help = renderHelp(colors, "↑/↓", "move", "space", "toggle", "tab", "filter", "enter", "done", "esc", "back")
		help = lipgloss.NewStyle().Width(width).Render(help)
	}
	sections = append(sections, help)
	content := lipgloss.JoinVertical(lipgloss.Left, sections...)
	view := tea.NewView(lipgloss.NewStyle().Padding(1, 2).Render(content))
	view.AltScreen = true
	view.WindowTitle = "Local AI Lab · Models"
	return view
}

func (m modelPickerModel) renderSummary(colors theme) string {
	count := 0
	var size uint64
	for index, selected := range m.selected {
		if selected {
			count++
			size += m.models[index].SizeBytes
		}
	}
	label := "models"
	if count == 1 {
		label = "model"
	}
	return lipgloss.NewStyle().Bold(true).Foreground(colors.accent).
		Render(fmt.Sprintf("%d %s · %s selected", count, label, formatBytes(size)))
}

func (m modelPickerModel) renderFilters(colors theme, width int) string {
	labels := make([]string, len(modelFilterLabels))
	for index, label := range modelFilterLabels {
		if width < 70 {
			label = [...]string{"ALL", "CODE", "THINK", "VISION", "RAG"}[index]
		}
		style := lipgloss.NewStyle().Foreground(colors.muted).Padding(0, 1)
		if modelFilter(index) == m.filter {
			style = style.Bold(true).Foreground(colors.text).Background(colors.accentSoft)
		}
		labels[index] = style.Render(label)
	}
	return lipgloss.JoinHorizontal(lipgloss.Left, labels...)
}

func (m modelPickerModel) renderPicker(colors theme, width int) string {
	if width < 72 {
		return m.renderList(colors, width)
	}
	gap := 2
	listWidth := width * 55 / 100
	detailWidth := width - listWidth - gap
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		m.renderList(colors, listWidth),
		strings.Repeat(" ", gap),
		m.renderDetail(colors, detailWidth),
	)
}

func (m modelPickerModel) renderList(colors theme, width int) string {
	visible := m.visibleIndexes()
	maxRows := max(m.height-12, 4)
	if len(visible) > maxRows {
		cursor := 0
		for index, candidate := range visible {
			if candidate == m.cursor {
				cursor = index
				break
			}
		}
		start := max(cursor-maxRows/2, 0)
		start = min(start, len(visible)-maxRows)
		visible = visible[start : start+maxRows]
	}
	rows := make([]string, 0, len(visible))
	for _, index := range visible {
		model := m.models[index]
		check := "[ ]"
		if !model.Compatible {
			check = "[-]"
		} else if m.selected[index] {
			check = "[x]"
		}
		badge := string(model.Fit)
		nameWidth := max(width-len(badge)-9, 8)
		name := ansi.Truncate(model.Name, nameWidth, "…")
		row := fmt.Sprintf("  %s %-*s %s", check, nameWidth, name, badge)
		style := lipgloss.NewStyle().Foreground(colors.text).Width(width)
		if !model.Compatible {
			style = style.Foreground(colors.muted)
		}
		if index == m.cursor {
			marker := "›"
			if m.animate && m.frame%2 == 1 {
				marker = "»"
			}
			row = marker + row[1:]
			style = style.Background(colors.accentSoft)
			if model.Compatible {
				style = style.Foreground(colors.accent).Bold(true)
			}
		}
		rows = append(rows, style.Render(row))
	}
	if len(rows) == 0 {
		return lipgloss.NewStyle().Foreground(colors.muted).Width(width).Render("No models in this category")
	}
	return strings.Join(rows, "\n")
}

func (m modelPickerModel) renderDetail(colors theme, width int) string {
	model := m.models[m.cursor]
	context := "not available"
	if model.Context > 0 {
		context = formatContext(model.Context) + " context"
	}
	body := lipgloss.JoinVertical(
		lipgloss.Left,
		lipgloss.NewStyle().Bold(true).Foreground(colors.text).Render(model.Name),
		lipgloss.NewStyle().Foreground(colors.muted).Render(ansi.Wordwrap(model.Purpose, width-3, "")),
		"",
		detailLine(colors, "FIT", string(model.Fit)),
		detailLine(colors, "DOWNLOAD", formatBytes(model.SizeBytes)),
		detailLine(colors, "RUNTIME", context),
		detailLine(colors, "NATIVE", formatContext(model.NativeContext)),
		"",
		lipgloss.NewStyle().Bold(true).Foreground(colors.muted).Render("WHY THIS FITS"),
		lipgloss.NewStyle().Foreground(colors.text).Render(ansi.Wordwrap(model.Reason, width-3, "")),
	)
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), true, false, false, true).
		BorderForeground(colors.border).
		PaddingLeft(2).
		Width(width).
		Render(body)
}

func detailLine(colors theme, label, value string) string {
	return lipgloss.NewStyle().Foreground(colors.muted).Width(10).Render(label) +
		lipgloss.NewStyle().Foreground(colors.text).Render(value)
}

func formatContext(context int) string {
	if context >= 1024 {
		if context%1024 == 0 {
			return fmt.Sprintf("%dK", context/1024)
		}
		return fmt.Sprintf("%dK", context/1000)
	}
	return fmt.Sprintf("%d", context)
}

func (t *Terminal) runModelPicker(catalog []models.Model) ([]string, error) {
	program := tea.NewProgram(
		newModelPickerModel(catalog),
		tea.WithInput(t.source), tea.WithOutput(t.output),
		tea.WithEnvironment(os.Environ()),
		tea.WithWindowSize(defaultPromptWidth, defaultPromptHeight),
	)
	final, err := program.Run()
	if err != nil {
		return nil, fmt.Errorf("run model picker: %w", err)
	}
	model, ok := final.(modelPickerModel)
	if !ok {
		return nil, errors.New("run model picker: unexpected model")
	}
	if model.interrupted || !model.submitted {
		return nil, ErrInterrupted
	}
	selected := make([]string, 0, len(model.models))
	for index, candidate := range model.models {
		if model.selected[index] {
			selected = append(selected, candidate.Name)
		}
	}
	return selected, nil
}
