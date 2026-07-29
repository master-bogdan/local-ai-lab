package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const maxTaskOutputBytes = 256 * 1024

type taskModel struct {
	title, success string
	spinner        spinner.Model
	viewport       viewport.Model
	stream         *taskStream
	action         func(context.Context, io.Writer) error
	taskContext    context.Context
	cancel         context.CancelFunc
	width, height  int
	isDark         bool
	interactive    bool
	complete       bool
	cancelled      bool
	err            error
}

type taskOutputMsg string

type taskFinishedMsg struct {
	output string
	err    error
}

type taskStream struct {
	mu      sync.Mutex
	content []byte
	updates chan string
}

func newTaskModel(ctx context.Context, title, success string, interactive bool, action func(context.Context, io.Writer) error) taskModel {
	taskContext, cancel := context.WithCancel(ctx)
	stream := &taskStream{updates: make(chan string, 128)}
	output := viewport.New(viewport.WithWidth(defaultPromptWidth-4), viewport.WithHeight(defaultPromptHeight-12))
	output.SoftWrap = true
	output.FillHeight = true
	return taskModel{
		title: safeSingleLine(title), success: safeSingleLine(success),
		spinner:  spinner.New(spinner.WithSpinner(spinner.MiniDot)),
		viewport: output, stream: stream, action: action,
		taskContext: taskContext, cancel: cancel,
		width: defaultPromptWidth, height: defaultPromptHeight,
		isDark: true, interactive: interactive,
	}
}

func (m taskModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, tea.RequestBackgroundColor, m.run(), waitForTaskOutput(m.stream.updates))
}

func (m taskModel) run() tea.Cmd {
	return func() tea.Msg {
		err := m.action(m.taskContext, m.stream)
		return taskFinishedMsg{output: m.stream.snapshot(), err: err}
	}
}

func waitForTaskOutput(updates <-chan string) tea.Cmd {
	return func() tea.Msg {
		return taskOutputMsg(<-updates)
	}
}

func (m taskModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		m.resize(message.Width, message.Height)
	case tea.BackgroundColorMsg:
		m.isDark = message.IsDark()
	case taskOutputMsg:
		m.setOutput(m.stream.snapshot())
		return m, waitForTaskOutput(m.stream.updates)
	case taskFinishedMsg:
		m.complete = true
		m.err = message.err
		m.setOutput(message.output)
		if !m.interactive {
			return m, tea.Quit
		}
	case spinner.TickMsg:
		if !m.complete {
			var command tea.Cmd
			m.spinner, command = m.spinner.Update(message)
			return m, command
		}
	case tea.InterruptMsg:
		return m.cancelOrExit()
	case tea.KeyPressMsg:
		switch message.String() {
		case "ctrl+c", "esc", "q":
			return m.cancelOrExit()
		case "enter":
			if m.complete {
				return m, tea.Quit
			}
		}
	}

	var command tea.Cmd
	m.viewport, command = m.viewport.Update(message)
	return m, command
}

func (m taskModel) cancelOrExit() (tea.Model, tea.Cmd) {
	if m.complete {
		return m, tea.Quit
	}
	m.cancelled = true
	m.cancel()
	return m, nil
}

func (m *taskModel) resize(width, height int) {
	m.width, m.height = width, height
	m.viewport.SetWidth(scrollViewportWidth(width))
	m.viewport.SetHeight(max(height-13, 5))
}

func (m *taskModel) setOutput(content string) {
	content = safeText(content)
	wasAtBottom := m.viewport.AtBottom()
	if strings.TrimSpace(content) == "" {
		content = "Waiting for command output…"
	}
	m.viewport.SetContent(content)
	if wasAtBottom {
		m.viewport.GotoBottom()
	}
}

func (m taskModel) View() tea.View {
	colors := themeFor(m.isDark)
	width := screenWidth(m.width)
	status := m.renderStatus(colors)
	output := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colors.border).
		Padding(0, 1).
		Width(scrollPanelWidth(m.width)).
		Height(max(m.viewport.Height(), 5)).
		Render(m.viewport.View())
	help := renderHelp(colors, "↑/↓", "scroll", "ctrl+c", "cancel")
	if m.complete {
		help = renderHelp(colors, "↑/↓", "scroll", "enter", "return")
	}
	content := lipgloss.JoinVertical(lipgloss.Left,
		renderBrand(colors, width), "",
		lipgloss.NewStyle().Bold(true).Foreground(colors.text).Render(m.title),
		status, "",
		lipgloss.NewStyle().Bold(true).Foreground(colors.muted).Render("OUTPUT"),
		output, "", help,
	)
	view := tea.NewView(lipgloss.NewStyle().Padding(1, 2).Render(content))
	view.AltScreen = true
	view.WindowTitle = "Local AI Lab · " + m.title
	return view
}

func (m taskModel) renderStatus(colors theme) string {
	if !m.complete {
		label := "Running"
		if m.cancelled {
			label = "Cancelling"
		}
		return m.spinner.View() + " " + lipgloss.NewStyle().Foreground(colors.accent).Render(label)
	}
	if m.err != nil && !m.cancelled {
		return lipgloss.NewStyle().Bold(true).Foreground(colors.danger).Render("✗ Failed: " + safeSingleLine(m.err.Error()))
	}
	label := m.success
	if label == "" {
		label = "Complete"
	}
	if m.cancelled {
		label = "Cancelled"
	}
	return lipgloss.NewStyle().Bold(true).Foreground(colors.success).Render("✓ " + label)
}

func (s *taskStream) Write(payload []byte) (int, error) {
	s.mu.Lock()
	s.content = append(s.content, payload...)
	if len(s.content) > maxTaskOutputBytes {
		tail := append([]byte(nil), s.content[len(s.content)-maxTaskOutputBytes:]...)
		s.content = tail
	}
	s.mu.Unlock()
	s.updates <- string(append([]byte(nil), payload...))
	return len(payload), nil
}

func (s *taskStream) snapshot() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return string(append([]byte(nil), s.content...))
}

func (t *Terminal) RunTask(ctx context.Context, title, success string, action func(context.Context, io.Writer) error) error {
	model := newTaskModel(ctx, title, success, t.isInteractive(), action)
	model.setOutput("")
	program := tea.NewProgram(
		model,
		tea.WithContext(ctx), tea.WithInput(t.source), tea.WithOutput(t.output),
		tea.WithEnvironment(os.Environ()),
		tea.WithWindowSize(defaultPromptWidth, defaultPromptHeight),
	)
	final, err := program.Run()
	if err != nil {
		return fmt.Errorf("run %s task: %w", title, err)
	}
	finished, ok := final.(taskModel)
	if !ok {
		return fmt.Errorf("run %s task: unexpected model", title)
	}
	finished.cancel()
	if finished.cancelled {
		return nil
	}
	if finished.err != nil && !errors.Is(finished.err, context.Canceled) {
		return finished.err
	}
	return nil
}
