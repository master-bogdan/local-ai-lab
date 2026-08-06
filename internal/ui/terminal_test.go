package ui_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/master-bogdan/local-ai-lab/internal/hardware"
	"github.com/master-bogdan/local-ai-lab/internal/models"
	"github.com/master-bogdan/local-ai-lab/internal/ui"
)

func TestSelectReturnsInterruptedForControlC(t *testing.T) {
	terminal := ui.NewTerminal(bytes.NewBufferString("\x03"), &bytes.Buffer{})

	_, err := terminal.Select("Menu", []ui.Option{{Label: "Exit", Value: "exit"}}, "exit")

	if !errors.Is(err, ui.ErrInterrupted) {
		t.Fatalf("expected interrupted error, got %v", err)
	}
}

func TestSelectNavigatesWithArrowKeys(t *testing.T) {
	output := &bytes.Buffer{}
	terminal := ui.NewTerminal(bytes.NewBufferString("\x1b[B\r"), output)

	selected, err := terminal.Select("Main menu", []ui.Option{
		{Label: "Start workload", Value: "start"},
		{Label: "Service status", Value: "status"},
		{Label: "Exit", Value: "exit"},
	}, "start")
	if err != nil {
		t.Fatalf("select action: %v", err)
	}
	if selected != "status" {
		t.Fatalf("selected = %q, want status", selected)
	}
	if !strings.Contains(output.String(), "navigate") {
		t.Fatalf("menu does not expose keyboard help:\n%s", output.String())
	}
	if strings.Contains(output.String(), "Choice [") {
		t.Fatalf("menu still uses numeric input:\n%s", output.String())
	}
}

func TestConfirmUsesNavigableChoices(t *testing.T) {
	output := &bytes.Buffer{}
	terminal := ui.NewTerminal(bytes.NewBufferString("\x1b[B\r"), output)

	confirmed, err := terminal.Confirm("Download selected models?", false)
	if err != nil {
		t.Fatalf("confirm action: %v", err)
	}
	if !confirmed {
		t.Fatal("confirmation was not selected")
	}
	if strings.Contains(output.String(), "[y/N]:") {
		t.Fatalf("confirmation still uses raw text input:\n%s", output.String())
	}
}

func TestInputSupportsTerminalEditing(t *testing.T) {
	output := &bytes.Buffer{}
	terminal := ui.NewTerminal(&keystrokeReader{reader: bytes.NewReader([]byte("/new/path\r"))}, output)

	value, err := terminal.Input("Data directory", "")
	if err != nil {
		t.Fatalf("input value: %v", err)
	}
	if value != "/new/path" {
		t.Fatalf("value = %q, want /new/path", value)
	}
	if !strings.Contains(output.String(), "ctrl+u") {
		t.Fatalf("input does not expose editing help:\n%s", output.String())
	}
}

func TestMultiSelectTogglesChoices(t *testing.T) {
	output := &bytes.Buffer{}
	terminal := ui.NewTerminal(bytes.NewBufferString("\x1b[B \r"), output)

	selected, err := terminal.MultiSelect("Core services", []ui.Option{
		{Label: "Local web search", Value: "search", Selected: true},
		{Label: "Workspace knowledge", Value: "knowledge"},
		{Label: "Monitoring", Value: "monitoring"},
	})
	if err != nil {
		t.Fatalf("select services: %v", err)
	}
	if got := strings.Join(selected, ","); got != "search,knowledge" {
		t.Fatalf("selected = %q, want search,knowledge", got)
	}
	if !strings.Contains(output.String(), "space") {
		t.Fatalf("multi-select does not expose toggle help:\n%s", output.String())
	}
}

type keystrokeReader struct {
	reader *bytes.Reader
}

func (r *keystrokeReader) Read(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	read, err := r.reader.Read(buffer[:1])
	if errors.Is(err, io.EOF) {
		return read, io.EOF
	}
	return read, err
}

func TestPauseWaitsForEnter(t *testing.T) {
	output := &bytes.Buffer{}
	terminal := ui.NewTerminal(bytes.NewBufferString("\n"), output)

	if err := terminal.Pause("Complete setup, then press Enter"); err != nil {
		t.Fatalf("pause returned error: %v", err)
	}
	if got := output.String(); got != "Complete setup, then press Enter: " {
		t.Fatalf("pause prompt = %q", got)
	}
}

func TestServicesExplainsPredefinedMonitoringDashboard(t *testing.T) {
	output := &bytes.Buffer{}
	terminal := ui.NewTerminal(bytes.NewBufferString("\x1b[B\x1b[B\x1b[B \r"), output)

	services, err := terminal.Services(models.Coding)
	if err != nil {
		t.Fatalf("choose services: %v", err)
	}
	if !services.Monitoring {
		t.Fatal("monitoring was not selected")
	}
	if !strings.Contains(output.String(), "predefined Grafana dashboard") {
		t.Fatalf("monitoring prompt does not explain dashboard: %q", output.String())
	}
}

func TestWorkloadPickerShowsHardwareAwareChoices(t *testing.T) {
	output := &bytes.Buffer{}
	terminal := ui.NewTerminal(bytes.NewBufferString("\r"), output)
	report := hardware.Report{
		MemoryBytes: 64 * hardware.GiB,
		GPU:         hardware.GPU{Name: "RTX 4090 Laptop GPU", VRAMBytes: 16 * hardware.GiB},
	}

	workload, err := terminal.Workload(report)
	if err != nil {
		t.Fatal(err)
	}
	if workload != models.Coding {
		t.Fatalf("workload = %q, want coding", workload)
	}
	for _, wanted := range []string{"RTX 4090 Laptop GPU", "Coding agent", "Complete lab", "Custom"} {
		if !strings.Contains(output.String(), wanted) {
			t.Fatalf("workload picker is missing %q:\n%s", wanted, output.String())
		}
	}
}

func TestModelPickerCannotSelectUnsupportedModel(t *testing.T) {
	terminal := ui.NewTerminal(bytes.NewBufferString(" \x1b[B \r"), &bytes.Buffer{})
	catalog := []models.Model{
		{Name: "too-large", Purpose: "oversized model", Fit: models.Unsupported, Reason: "requires more VRAM", SizeBytes: 65 * hardware.GiB},
		{Name: "daily", Purpose: "daily coding", Fit: models.Fast, Reason: "GPU-resident", SizeBytes: 6 * hardware.GiB, Context: 32768},
	}

	selected, err := terminal.Models(catalog)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(selected, ","); got != "daily" {
		t.Fatalf("selected = %q, want daily", got)
	}
}

func TestWelcomeShowsPurposeAndOwnership(t *testing.T) {
	output := &bytes.Buffer{}
	terminal := ui.NewTerminal(bytes.NewBuffer(nil), output)

	terminal.Welcome()

	for _, wanted := range []string{
		"LOCAL AI LAB",
		"Private AI workstation",
		"bogdanlabs.dev",
		"github.com/master-bogdan",
	} {
		if !strings.Contains(output.String(), wanted) {
			t.Fatalf("welcome is missing %q:\n%s", wanted, output.String())
		}
	}
}

func TestRawMessagesDoNotRenderTerminalControlSequences(t *testing.T) {
	output := &bytes.Buffer{}
	terminal := ui.NewTerminal(bytes.NewBuffer(nil), output)

	terminal.Info("before%safter", "\x1b]52;c;YXR0YWNr\x07")

	got := output.String()
	if strings.Contains(got, "\x1b]52") || strings.Contains(got, "YXR0YWNr") {
		t.Fatalf("message rendered terminal control sequence: %q", got)
	}
	if got != "beforeafter\n" {
		t.Fatalf("message = %q, want surrounding text", got)
	}
}

func TestInstallationMenuRendersMetadataAndNavigates(t *testing.T) {
	output := &bytes.Buffer{}
	terminal := ui.NewTerminal(bytes.NewBufferString("\x1b[B\r"), output)
	summary := ui.InstallationSummary{
		Status: "Running", DataDir: "/home/alice/.local/share/local-ai-lab",
		DataBytes: 84 * 1024 * 1024 * 1024, FreeDiskBytes: 312 * 1024 * 1024 * 1024,
		Platform: "fedora 42", GPU: "RTX 4090 Laptop GPU", Runtime: "cuda",
		VRAMBytes: 16 * 1024 * 1024 * 1024, RAMBytes: 64 * 1024 * 1024 * 1024,
		Models: 6, Services: "Search, Knowledge, Web UI", Modules: "OpenCode", Workload: "coding",
		ModelProfile: "coding", ContextLength: 32768,
	}

	selected, err := terminal.InstallationMenu(summary, []ui.Option{
		{Label: "Start workload", Value: "start"},
		{Label: "Service status", Value: "status"},
	}, "start")
	if err != nil {
		t.Fatalf("open installation menu: %v", err)
	}
	if selected != "status" {
		t.Fatalf("selected = %q, want status", selected)
	}
	for _, wanted := range []string{"RUNNING", "RTX 4090 Laptop GPU", "16.0 GiB", "6 models", "coding · 32K context", "Service status"} {
		if !strings.Contains(output.String(), wanted) {
			t.Fatalf("installation menu is missing %q:\n%s", wanted, output.String())
		}
	}
}

func TestTaskShowsCommandOutputAndCompletion(t *testing.T) {
	output := &bytes.Buffer{}
	terminal := ui.NewTerminal(bytes.NewBuffer(nil), output)

	err := terminal.RunTask(context.Background(), "Download model", "Model ready", func(_ context.Context, writer io.Writer) error {
		_, writeErr := io.WriteString(writer, "pulling qwen3.5:9b\nverified digest\n")
		return writeErr
	})
	if err != nil {
		t.Fatalf("run task: %v", err)
	}
	for _, wanted := range []string{"Download model", "pulling qwen3.5:9b", "verified digest", "Model ready"} {
		if !strings.Contains(output.String(), wanted) {
			t.Fatalf("task view is missing %q:\n%s", wanted, output.String())
		}
	}
}

func TestTaskDoesNotRenderTerminalControlSequences(t *testing.T) {
	output := &bytes.Buffer{}
	terminal := ui.NewTerminal(bytes.NewBuffer(nil), output)

	err := terminal.RunTask(context.Background(), "Container output", "Complete", func(_ context.Context, writer io.Writer) error {
		_, writeErr := io.WriteString(writer, "before\x1b]52;c;YXR0YWNr\x07after\n")
		return writeErr
	})
	if err != nil {
		t.Fatalf("run task: %v", err)
	}
	got := output.String()
	if strings.Contains(got, "\x1b]52") || strings.Contains(got, "YXR0YWNr") {
		t.Fatalf("task rendered terminal control sequence: %q", got)
	}
	if !strings.Contains(got, "beforeafter") {
		t.Fatalf("task removed surrounding text: %q", got)
	}
}

func TestTaskCancellationReachesRunningAction(t *testing.T) {
	output := &bytes.Buffer{}
	terminal := ui.NewTerminal(bytes.NewBufferString("\x03"), output)

	err := terminal.RunTask(context.Background(), "Follow logs", "Log stream closed", func(ctx context.Context, _ io.Writer) error {
		<-ctx.Done()
		return ctx.Err()
	})
	if err != nil {
		t.Fatalf("cancel task: %v", err)
	}
	if !strings.Contains(output.String(), "Cancelled") {
		t.Fatalf("task does not show cancelled state:\n%s", output.String())
	}
}

func TestTaskShowsFailureAndReturnsCause(t *testing.T) {
	output := &bytes.Buffer{}
	terminal := ui.NewTerminal(bytes.NewBuffer(nil), output)
	wantedErr := errors.New("container failed health check")

	err := terminal.RunTask(context.Background(), "Start workload", "Workload running", func(_ context.Context, _ io.Writer) error {
		return wantedErr
	})
	if !errors.Is(err, wantedErr) {
		t.Fatalf("task error = %v, want %v", err, wantedErr)
	}
	if !strings.Contains(output.String(), "Failed: container failed health check") {
		t.Fatalf("task does not show failure state:\n%s", output.String())
	}
}

func TestShowKeepsStructuredResultUntilReturn(t *testing.T) {
	output := &bytes.Buffer{}
	terminal := ui.NewTerminal(bytes.NewBufferString("\r"), output)

	err := terminal.Show("Local URLs", "Services bind to localhost", "Open WebUI  http://127.0.0.1:3000\nGrafana     http://127.0.0.1:3002")
	if err != nil {
		t.Fatalf("show result: %v", err)
	}
	for _, wanted := range []string{"Local URLs", "Services bind to localhost", "Open WebUI", "enter return"} {
		if !strings.Contains(output.String(), wanted) {
			t.Fatalf("result view is missing %q:\n%s", wanted, output.String())
		}
	}
}

func TestShowReturnsInterruptedForEscape(t *testing.T) {
	terminal := ui.NewTerminal(bytes.NewBufferString("\x1b"), &bytes.Buffer{})

	err := terminal.Show("Setup", "Review before continuing", "Nothing runs without confirmation.")

	if !errors.Is(err, ui.ErrInterrupted) {
		t.Fatalf("show error = %v, want interrupted", err)
	}
}

func TestShowDoesNotRenderTerminalControlSequences(t *testing.T) {
	output := &bytes.Buffer{}
	terminal := ui.NewTerminal(bytes.NewBufferString("\r"), output)

	err := terminal.Show("Result", "Safe output", "before\x1b]2;forged title\x07after")
	if err != nil {
		t.Fatalf("show result: %v", err)
	}
	got := output.String()
	if strings.Contains(got, "\x1b]2;forged title") || strings.Contains(got, "forged title") {
		t.Fatalf("result rendered terminal control sequence: %q", got)
	}
	if !strings.Contains(got, "beforeafter") {
		t.Fatalf("result removed surrounding text: %q", got)
	}
}

func TestReviewKeepsPlanVisibleWhileConfirming(t *testing.T) {
	output := &bytes.Buffer{}
	terminal := ui.NewTerminal(bytes.NewBufferString("\x1b[C\r"), output)

	confirmed, err := terminal.Review(
		"Installation plan", "Review before downloading",
		"Data      /home/alice/.local/share/local-ai-lab\nDownload  42.0 GiB",
		"Install", false,
	)
	if err != nil {
		t.Fatalf("review plan: %v", err)
	}
	if !confirmed {
		t.Fatal("review did not confirm selected action")
	}
	for _, wanted := range []string{"Installation plan", "42.0 GiB", "Install", "cancel"} {
		if !strings.Contains(output.String(), wanted) {
			t.Fatalf("review is missing %q:\n%s", wanted, output.String())
		}
	}
}
