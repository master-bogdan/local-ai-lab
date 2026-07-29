package ui

import (
	"context"
	"io"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/master-bogdan/local-ai-lab/internal/hardware"
	"github.com/master-bogdan/local-ai-lab/internal/models"
)

func TestOnboardingFitsStandardTerminalWithLogo(t *testing.T) {
	model := newSelectModel(menuScreen{
		title: "Private AI workstation", subtitle: "Local models, coding agents, web search, workspace knowledge, and images.",
		status: "SETUP REQUIRED", showLogo: true,
	}, []Option{{Label: "Install", Value: "install"}, {Label: "Exit", Value: "exit"}}, "install")

	assertViewFits(t, resizeSelect(model, 80, 24), 80, 24, "|_____\\___/", "SETUP REQUIRED", "esc", "back")
}

func TestModelPickerUsesSplitPaneOnWideTerminalAndFitsNarrowTerminal(t *testing.T) {
	catalog := []models.Model{
		{
			Name: "qwen3.5:9b", Purpose: "fast daily coding", Kind: models.CodingModel,
			SizeBytes: 6600 * 1024 * 1024, Context: 32768, NativeContext: 256000,
			Fit: models.Fast, Compatible: true, Selected: true, Reason: "model and context stay GPU-resident",
		},
		{
			Name: "gpt-oss:120b", Purpose: "high-end reasoning", Kind: models.Chat,
			SizeBytes: 65 * hardware.GiB, NativeContext: 128000,
			Fit: models.Unsupported, Reason: "hardware does not meet minimum memory",
		},
	}

	wide := newModelPickerModel(catalog)
	wideUpdated, _ := wide.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	assertRenderedViewFits(
		t, wideUpdated.(modelPickerModel).View().Content, 160, 40,
		"qwen3.5:9b", "WHY THIS FITS", "model and context stay GPU-resident", "tab", "filter",
	)

	narrow := newModelPickerModel(catalog)
	narrowUpdated, _ := narrow.Update(tea.WindowSizeMsg{Width: 50, Height: 24})
	assertRenderedViewFits(t, narrowUpdated.(modelPickerModel).View().Content, 50, 24, "qwen3.5:9b", "FAST", "esc", "back")
}

func TestModelPickerStartsOnFirstRecommendedModel(t *testing.T) {
	model := newModelPickerModel([]models.Model{
		{Name: "daily", Fit: models.Fast, Compatible: true, Selected: true},
		{Name: "agent", Fit: models.Tight, Compatible: true, Selected: true},
		{Name: "embedding", Fit: models.Fast, Compatible: true, Selected: true},
	})

	if model.cursor != 0 {
		t.Fatalf("cursor = %d, want first recommended model", model.cursor)
	}
	if got := formatContext(32000); got != "32K" {
		t.Fatalf("decimal context = %q, want 32K", got)
	}
	if got := formatContext(32768); got != "32K" {
		t.Fatalf("binary context = %q, want 32K", got)
	}
}

func TestInstalledDashboardFitsNarrowTerminal(t *testing.T) {
	options := make([]Option, 10)
	for i := range options {
		options[i] = Option{Label: "Dashboard action", Description: "A longer explanation for this action", Value: string(rune('a' + i))}
	}
	model := newSelectModel(menuScreen{
		title: "Local AI control center", subtitle: "/home/alice/.local/share/local-ai-lab", status: "RUNNING",
		details: []menuDetail{
			{label: "System", value: "fedora 42"}, {label: "Accelerator", value: "RTX 4090 Laptop GPU"},
			{label: "Services", value: "Search, Knowledge, Web UI, Monitoring"},
		},
		compactDetails: []menuDetail{
			{label: "System", value: "fedora 42 · CUDA"},
			{label: "GPU", value: "RTX 4090 Laptop GPU · 16.0 GiB"},
			{label: "Lab", value: "6 models · Search, Knowledge"},
		},
	}, options, options[0].Value)

	assertViewFits(t, resizeSelect(model, 50, 24), 50, 24, "RUNNING", "RTX 4090", "6 models", "esc", "back")
}

func TestMessageFitsNarrowTerminal(t *testing.T) {
	model := newMessageModel(
		"System requirements",
		"Detected hardware and supported runtimes.",
		strings.Repeat("A complete requirements line that wraps on a narrow terminal.\n", 20),
	)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 50, Height: 24})

	assertRenderedViewFits(t, updated.(messageModel).View().Content, 50, 24, "System requirements", "esc", "back")
}

func TestTaskFitsNarrowTerminal(t *testing.T) {
	model := newTaskModel(
		context.Background(),
		"Start coding workload",
		"Workload running",
		true,
		func(context.Context, io.Writer) error { return nil },
	)
	model.setOutput(strings.Repeat("A complete task output line that wraps on a narrow terminal.\n", 20))
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 50, Height: 24})

	assertRenderedViewFits(t, updated.(taskModel).View().Content, 50, 24, "Start coding workload", "ctrl+c", "cancel")
}

func TestWideScreensUseAvailableTerminalWidth(t *testing.T) {
	const width, height = 160, 50

	selectScreen := newSelectModel(
		menuScreen{title: "Private AI workstation", subtitle: "Choose an action"},
		[]Option{{Label: "Install", Value: "install"}},
		"install",
	)
	selectScreen = resizeSelect(selectScreen, width, height)

	messageScreen := newMessageModel("System requirements", "Supported hosts", "Linux\nmacOS")
	messageUpdated, _ := messageScreen.Update(tea.WindowSizeMsg{Width: width, Height: height})

	reviewScreen := newReviewModel("Installation plan", "Review changes", "Download 42 GiB", "Install", false)
	reviewUpdated, _ := reviewScreen.Update(tea.WindowSizeMsg{Width: width, Height: height})

	inputScreen := newInputModel("Data directory", "/home/alice/.local/share/local-ai-lab")
	inputUpdated, _ := inputScreen.Update(tea.WindowSizeMsg{Width: width, Height: height})

	multiScreen := newMultiSelectModel("Core services", []Option{{Label: "Web search", Value: "search"}})
	multiUpdated, _ := multiScreen.Update(tea.WindowSizeMsg{Width: width, Height: height})

	taskScreen := newTaskModel(
		context.Background(),
		"Start workload",
		"Running",
		true,
		func(context.Context, io.Writer) error { return nil },
	)
	taskUpdated, _ := taskScreen.Update(tea.WindowSizeMsg{Width: width, Height: height})

	screens := map[string]string{
		"select":       selectScreen.View().Content,
		"message":      messageUpdated.(messageModel).View().Content,
		"review":       reviewUpdated.(reviewModel).View().Content,
		"input":        inputUpdated.(inputModel).View().Content,
		"multi-select": multiUpdated.(multiSelectModel).View().Content,
		"task":         taskUpdated.(taskModel).View().Content,
	}
	for name, content := range screens {
		t.Run(name, func(t *testing.T) {
			if got := lipgloss.Width(content); got != width {
				t.Fatalf("view width = %d, terminal width = %d", got, width)
			}
		})
	}
}

func TestShortScrollablePanelsStayCompactOnTallTerminal(t *testing.T) {
	const width, height = 160, 50

	messageScreen := newMessageModel("Setup", "Review before continuing", "Nothing runs without confirmation.")
	messageUpdated, _ := messageScreen.Update(tea.WindowSizeMsg{Width: width, Height: height})
	if got := lipgloss.Height(messageUpdated.(messageModel).View().Content); got >= height/2 {
		t.Fatalf("short message height = %d, want less than %d", got, height/2)
	}

	reviewScreen := newReviewModel("Installation plan", "Review changes", "Download 42 GiB", "Install", false)
	reviewUpdated, _ := reviewScreen.Update(tea.WindowSizeMsg{Width: width, Height: height})
	if got := lipgloss.Height(reviewUpdated.(reviewModel).View().Content); got >= height/2 {
		t.Fatalf("short review height = %d, want less than %d", got, height/2)
	}
}

func resizeSelect(model selectModel, width, height int) selectModel {
	updated, _ := model.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return updated.(selectModel)
}

func assertViewFits(t *testing.T, model selectModel, width, height int, wanted ...string) {
	t.Helper()
	assertRenderedViewFits(t, model.View().Content, width, height, wanted...)
}

func assertRenderedViewFits(t *testing.T, content string, width, height int, wanted ...string) {
	t.Helper()
	if got := lipgloss.Width(content); got > width {
		t.Fatalf("view width = %d, terminal width = %d\n%s", got, width, content)
	}
	if got := lipgloss.Height(content); got > height {
		t.Fatalf("view height = %d, terminal height = %d\n%s", got, height, content)
	}
	for _, text := range wanted {
		if !strings.Contains(content, text) {
			t.Fatalf("view is missing %q\n%s", text, content)
		}
	}
}
