package ui

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	"golang.org/x/term"

	"github.com/master-bogdan/local-ai-lab/internal/app"
	"github.com/master-bogdan/local-ai-lab/internal/config"
	"github.com/master-bogdan/local-ai-lab/internal/hardware"
	"github.com/master-bogdan/local-ai-lab/internal/models"
	labruntime "github.com/master-bogdan/local-ai-lab/internal/runtime"
	"github.com/master-bogdan/local-ai-lab/internal/terminalimage"
)

type Terminal struct {
	input  *bufio.Reader
	source io.Reader
	output io.Writer
	color  bool
}

var ErrInterrupted = errors.New("terminal input interrupted")

const asciiLogo = ` _                    _      _    ___   _          _
| |    ___   ___ __ _| |    / \  |_ _| | |    __ _| |__
| |   / _ \ / __/ _  | |   / _ \  | |  | |   / _  | '_ \
| |__| (_) | (_| (_| | |  / ___ \ | |  | |__| (_| | |_) |
|_____\___/ \___\__,_|_| /_/   \_\___| |_____\__,_|_.__/`

type Option struct {
	Label       string
	Description string
	Shortcut    string
	Value       string
	Selected    bool
}

type InstallationSummary struct {
	Status, DataDir, Platform, GPU, Runtime   string
	Services, Modules, Workload, ModelProfile string
	Application                               string
	DataBytes, FreeDiskBytes                  uint64
	VRAMBytes, RAMBytes                       uint64
	Models, ContextLength                     int
}

func NewTerminal(input io.Reader, output io.Writer) *Terminal {
	return &Terminal{input: bufio.NewReader(input), source: input, output: output, color: os.Getenv("NO_COLOR") == ""}
}

func (t *Terminal) Welcome() {
	fmt.Fprintf(t.output, "\n%s\n", t.style("1;36", asciiLogo))
	fmt.Fprintln(t.output, t.style("1", "LOCAL AI LAB"))
	fmt.Fprintln(t.output, "Private AI workstation")
	fmt.Fprintln(t.output, "Local models | Coding agents | Search | Images")
	fmt.Fprintln(t.output)
	fmt.Fprintln(t.output, "bogdanlabs.dev | github.com/master-bogdan")
}

func (t *Terminal) Heading(text string) {
	fmt.Fprintf(t.output, "\n%s\n", t.style("1;36", safeSingleLine(text)))
}

func (t *Terminal) Info(format string, args ...any) {
	fmt.Fprintln(t.output, safeText(fmt.Sprintf(format, args...)))
}

func (t *Terminal) Warn(format string, args ...any) {
	fmt.Fprintln(t.output, t.style("1;33", safeText(fmt.Sprintf(format, args...))))
}

func (t *Terminal) Success(format string, args ...any) {
	fmt.Fprintln(t.output, t.style("1;32", safeText(fmt.Sprintf(format, args...))))
}

func (t *Terminal) Input(label, defaultValue string) (string, error) {
	return t.runInput(label, defaultValue)
}

func (t *Terminal) Pause(label string) error {
	fmt.Fprintf(t.output, "%s: ", safeSingleLine(label))
	_, err := t.readLine()
	return err
}

func (t *Terminal) Confirm(label string, defaultValue bool) (bool, error) {
	defaultChoice := "no"
	if defaultValue {
		defaultChoice = "yes"
	}
	choice, err := t.Select(label, []Option{
		{Label: "No", Description: "Return without making changes", Shortcut: "n", Value: "no"},
		{Label: "Yes", Description: "Confirm and continue", Shortcut: "y", Value: "yes"},
	}, defaultChoice)
	if err != nil {
		return false, err
	}
	return choice == "yes", nil
}

func (t *Terminal) Select(label string, options []Option, defaultValue string) (string, error) {
	return t.runSelect(label, options, defaultValue)
}

func (t *Terminal) DataDirectory(defaultPath string) (string, error) {
	t.Heading("Installation path")
	return t.Input("Data directory", defaultPath)
}

func (t *Terminal) Workload(report hardware.Report) (models.Workload, error) {
	hardwareSummary := report.GPU.Name
	if report.GPU.VRAMBytes > 0 {
		hardwareSummary += " · " + formatBytes(report.GPU.VRAMBytes) + " VRAM"
	} else {
		hardwareSummary += " · " + formatBytes(report.MemoryBytes) + " unified memory"
	}
	choice, err := t.Select("Primary workload · "+hardwareSummary, []Option{
		{Label: "Coding agent", Description: "Fast coding plus repository-scale agent and retrieval", Value: string(models.Coding)},
		{Label: "General and reasoning", Description: "Daily assistant plus multi-step reasoning and retrieval", Value: string(models.General)},
		{Label: "Vision", Description: "Screenshot understanding and general chat; ComfyUI remains separate", Value: string(models.Vision)},
		{Label: "Complete lab", Description: "Coding, reasoning, vision, and high-quality retrieval", Value: string(models.Complete)},
		{Label: "Minimal", Description: "One fast model with the smallest practical footprint", Value: string(models.Minimal)},
		{Label: "Custom", Description: "Start with nothing selected and build your own set", Value: string(models.Custom)},
	}, string(models.Coding))
	return models.Workload(choice), err
}

func (t *Terminal) Services(workload models.Workload) (config.Services, error) {
	coreSelected := workload != models.Minimal && workload != models.Custom
	selected, err := t.MultiSelect("Core services", []Option{
		{Label: "SearXNG", Description: "Private local web search", Value: "search", Selected: coreSelected},
		{Label: "Qdrant", Description: "Workspace knowledge and vector search", Value: "knowledge", Selected: coreSelected},
		{Label: "Open WebUI", Description: "Browser chat with local RAG", Value: "webui", Selected: coreSelected},
		{Label: "Monitoring", Description: "Prometheus, Grafana, alerts, and predefined Grafana dashboard", Value: "monitoring", Selected: workload == models.Complete},
	})
	if err != nil {
		return config.Services{}, err
	}
	search := containsValue(selected, "search")
	knowledge := containsValue(selected, "knowledge")
	webUI := containsValue(selected, "webui")
	if webUI && (!search || !knowledge) {
		t.Info("Open WebUI enables SearXNG and Qdrant because its local RAG configuration requires them.")
		search, knowledge = true, true
	}
	return config.Services{
		Search: search, Knowledge: knowledge, WebUI: webUI,
		Monitoring: containsValue(selected, "monitoring"),
	}, nil
}

func (t *Terminal) Models(catalog []models.Model) ([]string, error) {
	safeCatalog := append([]models.Model(nil), catalog...)
	for index := range safeCatalog {
		safeCatalog[index].Name = safeSingleLine(safeCatalog[index].Name)
		safeCatalog[index].Purpose = safeSingleLine(safeCatalog[index].Purpose)
		safeCatalog[index].Reason = safeSingleLine(safeCatalog[index].Reason)
	}
	return t.runModelPicker(safeCatalog)
}

func (t *Terminal) ImageProtocol() string {
	return string(terminalimage.Detect(environment()))
}

func (t *Terminal) PreviewImage(path string) error {
	protocol := terminalimage.Detect(environment())
	t.Heading("Generated image")
	t.Info("%s · %s", path, protocol)
	renderer := terminalimage.New(protocol, os.Getenv("TMUX") != "")
	if err := renderer.Render(t.output, path, 80, 28); err != nil {
		return err
	}
	fmt.Fprintln(t.output)
	return t.Pause("Press Enter to return")
}

func environment() map[string]string {
	values := make(map[string]string)
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			values[key] = value
		}
	}
	return values
}

func containsValue(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func (t *Terminal) ConfirmInstall(preview app.InstallPreview) (bool, error) {
	var body strings.Builder
	fmt.Fprintf(&body, "Platform  %s %s\n", preview.Hardware.OS, preview.Hardware.Distro)
	fmt.Fprintf(&body, "GPU       %s\n", preview.Hardware.GPU.Name)
	fmt.Fprintf(&body, "Runtime   %s\n", preview.Hardware.GPU.Runtime)
	fmt.Fprintf(&body, "VRAM      %s\n", formatBytes(preview.Hardware.GPU.VRAMBytes))
	fmt.Fprintf(&body, "Tier      %s\n", preview.Assessment.Tier)
	fmt.Fprintf(&body, "Workload  %s\n", preview.Workload)
	fmt.Fprintf(&body, "Context   %s shared runtime default\n", formatContext(preview.ContextLength))
	fmt.Fprintf(&body, "Data      %s\n", preview.DataDir)
	fmt.Fprintf(&body, "Download  %s including safety margin\n\n", formatBytes(preview.RequiredBytes))
	body.WriteString("MODELS\n")
	for _, model := range preview.Models {
		fmt.Fprintf(&body, "  %-28s %s · %s context\n", model.Name, model.Fit, formatContext(model.Context))
	}
	return t.Review("Installation plan", "Review all downloads and storage before continuing", body.String(), "Install", false)
}

func (t *Terminal) ChooseWorkload(_ context.Context, defaultWorkload labruntime.Workload) (labruntime.Workload, error) {
	choice, err := t.Select("Choose workload", []Option{
		{Label: "Coding", Description: "Ollama plus core services", Value: string(labruntime.Coding)},
		{Label: "Images", Description: "ComfyUI plus infrastructure", Value: string(labruntime.Images)},
		{Label: "Infrastructure", Description: "Search, knowledge, Web UI, and monitoring only", Value: string(labruntime.Infrastructure)},
		{Label: "Both engines", Description: "Ollama and ComfyUI; highest resource usage", Value: string(labruntime.Both)},
	}, string(defaultWorkload))
	return labruntime.Workload(choice), err
}

func (t *Terminal) readLine() (string, error) {
	if file, ok := t.inputSource(); ok && term.IsTerminal(int(file.Fd())) {
		return t.readRawLine(file)
	}
	line, err := t.input.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	if errors.Is(err, io.EOF) && line == "" {
		return "", io.EOF
	}
	line = strings.TrimSpace(line)
	if line == "\x03" {
		return "", ErrInterrupted
	}
	return line, nil
}

func (t *Terminal) inputSource() (*os.File, bool) {
	file, ok := t.source.(*os.File)
	return file, ok
}

func (t *Terminal) isInteractive() bool {
	input, inputOK := t.source.(*os.File)
	output, outputOK := t.output.(*os.File)
	return inputOK && outputOK && term.IsTerminal(int(input.Fd())) && term.IsTerminal(int(output.Fd()))
}

func (t *Terminal) readRawLine(file *os.File) (string, error) {
	state, err := term.MakeRaw(int(file.Fd()))
	if err != nil {
		return "", err
	}
	defer term.Restore(int(file.Fd()), state)

	line := make([]byte, 0, 64)
	buffer := make([]byte, 1)
	for {
		if _, err := file.Read(buffer); err != nil {
			return "", err
		}
		switch buffer[0] {
		case '\r', '\n':
			fmt.Fprint(t.output, "\r\n")
			return strings.TrimSpace(string(line)), nil
		case 3:
			fmt.Fprint(t.output, "^C\r\n")
			return "", ErrInterrupted
		case 4:
			if len(line) == 0 {
				fmt.Fprint(t.output, "\r\n")
				return "", io.EOF
			}
			fmt.Fprint(t.output, "\r\n")
			return strings.TrimSpace(string(line)), nil
		case 8, 127:
			if len(line) > 0 {
				line = line[:len(line)-1]
				fmt.Fprint(t.output, "\b \b")
			}
		default:
			if buffer[0] >= 32 {
				line = append(line, buffer[0])
				fmt.Fprintf(t.output, "%c", buffer[0])
			}
		}
	}
}

func (t *Terminal) style(code, text string) string {
	if !t.color {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func safeText(value string) string {
	stripped := ansi.Strip(value)
	return strings.Map(func(character rune) rune {
		switch character {
		case '\n', '\t':
			return character
		case '\r':
			return '\n'
		}
		if unicode.IsControl(character) {
			return -1
		}
		return character
	}, stripped)
}

func safeSingleLine(value string) string {
	return strings.Map(func(character rune) rune {
		if character == '\n' || character == '\t' {
			return ' '
		}
		return character
	}, safeText(value))
}

func safeOptions(options []Option) []Option {
	safe := append([]Option(nil), options...)
	for index := range safe {
		safe[index].Label = safeSingleLine(safe[index].Label)
		safe[index].Description = safeSingleLine(safe[index].Description)
		safe[index].Shortcut = safeSingleLine(safe[index].Shortcut)
	}
	return safe
}

func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	divisor, exponent := uint64(unit), 0
	for value := bytes / unit; value >= unit && exponent < 3; value /= unit {
		divisor *= unit
		exponent++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(divisor), "KMGT"[exponent])
}
