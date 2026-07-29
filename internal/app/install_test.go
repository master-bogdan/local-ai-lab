package app_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/master-bogdan/local-ai-lab/internal/app"
	"github.com/master-bogdan/local-ai-lab/internal/config"
	"github.com/master-bogdan/local-ai-lab/internal/hardware"
	"github.com/master-bogdan/local-ai-lab/internal/models"
	labruntime "github.com/master-bogdan/local-ai-lab/internal/runtime"
)

type hardwareDetector struct {
	report hardware.Report
}

func (d hardwareDetector) Detect(context.Context, string) (hardware.Report, error) {
	return d.report, nil
}

type installPrompt struct {
	dataDir  string
	workload models.Workload
	services config.Services
	models   []string
	catalog  []models.Model
	preview  app.InstallPreview
}

func (p installPrompt) DataDirectory(string) (string, error) { return p.dataDir, nil }
func (p installPrompt) Workload(hardware.Report) (models.Workload, error) {
	if p.workload == "" {
		return models.Coding, nil
	}
	return p.workload, nil
}
func (p installPrompt) Services(models.Workload) (config.Services, error) { return p.services, nil }
func (p *installPrompt) Models(catalog []models.Model) ([]string, error) {
	p.catalog = catalog
	return p.models, nil
}
func (p *installPrompt) ConfirmInstall(preview app.InstallPreview) (bool, error) {
	p.preview = preview
	return true, nil
}

type countingExecutor struct {
	calls int
}

type recordingExecutor struct {
	plan labruntime.Plan
}

func (e *recordingExecutor) Execute(_ context.Context, plan labruntime.Plan) error {
	e.plan = plan
	return nil
}

type checkingExecutor struct {
	check func() error
}

func (e checkingExecutor) Execute(context.Context, labruntime.Plan) error {
	return e.check()
}

func TestInstallerAddsEmbeddingModelWhenKnowledgeIsSelected(t *testing.T) {
	repoDir := t.TempDir()
	executor := &recordingExecutor{}
	installer := app.NewInstaller(
		config.NewStore(repoDir),
		hardwareDetector{report: hardware.Report{
			OS:          hardware.Linux,
			MemoryBytes: 32 * hardware.GiB,
			DiskBytes:   100 * hardware.GiB,
			GPU: hardware.GPU{
				Vendor: hardware.NVIDIA, Runtime: hardware.CUDA, VRAMBytes: 24 * hardware.GiB, Usable: true,
			},
		}},
		&installPrompt{
			dataDir:  safeDataDir(t),
			services: config.Services{Knowledge: true},
			models:   []string{"qwen3.5:9b"},
		},
		executor,
	)

	if err := installer.Run(context.Background(), app.InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	if !executor.plan.PullsModel("qwen3-embedding:0.6b") {
		t.Fatalf("knowledge service missing embedding model: %#v", executor.plan.Steps)
	}
}

func TestInstallerRejectsBroadDataDirectory(t *testing.T) {
	executor := &countingExecutor{}
	installer := app.NewInstaller(
		config.NewStore(t.TempDir()),
		hardwareDetector{},
		&installPrompt{dataDir: "/tmp"},
		executor,
	)

	err := installer.Run(context.Background(), app.InstallOptions{})

	if !errors.Is(err, app.ErrUnsafeDataDir) {
		t.Fatalf("expected unsafe data directory error, got %v", err)
	}
	if executor.calls != 0 {
		t.Fatal("unsafe data directory reached runtime executor")
	}
}

func TestInstallerRejectsSystemDataDirectory(t *testing.T) {
	executor := &countingExecutor{}
	installer := app.NewInstaller(
		config.NewStore(t.TempDir()),
		hardwareDetector{},
		&installPrompt{dataDir: "/etc/local-ai-lab"},
		executor,
	)

	err := installer.Run(context.Background(), app.InstallOptions{})

	if !errors.Is(err, app.ErrUnsafeDataDir) {
		t.Fatalf("expected unsafe data directory error, got %v", err)
	}
	if executor.calls != 0 {
		t.Fatal("unsafe data directory reached runtime executor")
	}
}

func TestInstallerCreatesDataLayoutBeforeRuntime(t *testing.T) {
	dataDir := safeDataDir(t)
	executor := checkingExecutor{check: func() error {
		for _, path := range []string{
			"models/ollama", "models/comfyui", "cache/searxng", "qdrant",
			"services/open-webui", "services/comfyui/input", "services/comfyui/output",
		} {
			info, err := os.Stat(filepath.Join(dataDir, path))
			if err != nil || !info.IsDir() {
				return errors.New("missing data directory " + path)
			}
		}
		return nil
	}}
	installer := app.NewInstaller(
		config.NewStore(t.TempDir()),
		hardwareDetector{report: hardware.Report{
			OS: hardware.Linux, MemoryBytes: 32 * hardware.GiB, DiskBytes: 100 * hardware.GiB,
			GPU: hardware.GPU{Vendor: hardware.NVIDIA, Runtime: hardware.CUDA, VRAMBytes: 24 * hardware.GiB, Usable: true},
		}},
		&installPrompt{dataDir: dataDir, models: []string{"qwen3.5:9b"}},
		executor,
	)

	if err := installer.Run(context.Background(), app.InstallOptions{}); err != nil {
		t.Fatal(err)
	}
}

func TestInstallerEnablesOpenWebUIDependencies(t *testing.T) {
	repoDir := t.TempDir()
	dataDir := safeDataDir(t)
	installer := app.NewInstaller(
		config.NewStore(repoDir),
		hardwareDetector{report: hardware.Report{
			OS: hardware.Linux, MemoryBytes: 32 * hardware.GiB, DiskBytes: 100 * hardware.GiB,
			GPU: hardware.GPU{Vendor: hardware.NVIDIA, Runtime: hardware.CUDA, VRAMBytes: 24 * hardware.GiB, Usable: true},
		}},
		&installPrompt{dataDir: dataDir, services: config.Services{WebUI: true}, models: []string{"qwen3.5:9b"}},
		&recordingExecutor{},
	)

	if err := installer.Run(context.Background(), app.InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	installation, err := config.NewStore(repoDir).Load()
	if err != nil {
		t.Fatal(err)
	}
	if !installation.Services.Search || !installation.Services.Knowledge {
		t.Fatalf("Open WebUI dependencies are disabled: %#v", installation.Services)
	}
}

func TestPrepareDataLayoutRestoresModelDirectories(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "local-ai-lab")
	if err := app.PrepareDataLayout(dataDir); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(dataDir, "models")); err != nil {
		t.Fatal(err)
	}

	if err := app.PrepareDataLayout(dataDir); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"models/ollama", "models/comfyui"} {
		if info, err := os.Stat(filepath.Join(dataDir, path)); err != nil || !info.IsDir() {
			t.Fatalf("model directory %s was not restored: %v", path, err)
		}
	}
}

func (e *countingExecutor) Execute(context.Context, labruntime.Plan) error {
	e.calls++
	return nil
}

func TestInstallerRefusesUnsupportedHardwareWithoutChanges(t *testing.T) {
	repoDir := t.TempDir()
	dataDir := safeDataDir(t)
	executor := &countingExecutor{}
	installer := app.NewInstaller(
		config.NewStore(repoDir),
		hardwareDetector{report: hardware.Report{OS: hardware.Linux, MemoryBytes: 32 * hardware.GiB}},
		&installPrompt{dataDir: dataDir},
		executor,
	)

	err := installer.Run(context.Background(), app.InstallOptions{})

	if !errors.Is(err, app.ErrUnsupportedHardware) {
		t.Fatalf("expected unsupported hardware error, got %v", err)
	}
	if executor.calls != 0 {
		t.Fatalf("executor called %d times", executor.calls)
	}
	if _, err := os.Stat(filepath.Join(repoDir, config.PointerFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("repository pointer was written: %v", err)
	}
	if _, err := os.Stat(dataDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("data directory was written: %v", err)
	}
}

func TestInstallerPersistsRecommendationRuntimeChoices(t *testing.T) {
	repoDir := t.TempDir()
	prompt := &installPrompt{
		dataDir:  safeDataDir(t),
		workload: models.General,
		services: config.Services{Knowledge: true},
		models:   []string{"qwen3.5:9b", "gpt-oss:20b", "qwen3-embedding:0.6b"},
	}
	installer := app.NewInstaller(
		config.NewStore(repoDir),
		hardwareDetector{report: hardware.Report{
			OS: hardware.Linux, MemoryBytes: 64 * hardware.GiB, DiskBytes: 100 * hardware.GiB,
			GPU: hardware.GPU{Vendor: hardware.NVIDIA, Runtime: hardware.CUDA, VRAMBytes: 16 * hardware.GiB, Usable: true},
		}},
		prompt,
		&recordingExecutor{},
	)

	if err := installer.Run(context.Background(), app.InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	installation, err := config.NewStore(repoDir).Load()
	if err != nil {
		t.Fatal(err)
	}
	if installation.ModelProfile != string(models.General) || installation.Workload != "" {
		t.Fatalf("model profile/workload = %q/%q", installation.ModelProfile, installation.Workload)
	}
	if installation.EmbeddingModel != "qwen3-embedding:0.6b" {
		t.Fatalf("embedding model = %q", installation.EmbeddingModel)
	}
	if installation.ContextLength != 32768 {
		t.Fatalf("runtime context = %d", installation.ContextLength)
	}
	if len(prompt.catalog) < 10 {
		t.Fatalf("prompt received incomplete recommendation catalog: %d", len(prompt.catalog))
	}
}

func TestInstallerRejectsUnsupportedModelSelection(t *testing.T) {
	executor := &countingExecutor{}
	installer := app.NewInstaller(
		config.NewStore(t.TempDir()),
		hardwareDetector{report: hardware.Report{
			OS: hardware.Linux, MemoryBytes: 16 * hardware.GiB, DiskBytes: 100 * hardware.GiB,
			GPU: hardware.GPU{Vendor: hardware.NVIDIA, Runtime: hardware.CUDA, VRAMBytes: 6 * hardware.GiB, Usable: true},
		}},
		&installPrompt{
			dataDir:  safeDataDir(t),
			workload: models.Custom,
			models:   []string{"qwen3-coder-next"},
		},
		executor,
	)

	err := installer.Run(context.Background(), app.InstallOptions{})

	if !errors.Is(err, app.ErrUnsupportedModel) {
		t.Fatalf("expected unsupported model error, got %v", err)
	}
	if executor.calls != 0 {
		t.Fatal("unsupported model reached runtime executor")
	}
}

func TestInstallerKeepsSelectedQualityEmbeddingWithoutAddingFallback(t *testing.T) {
	repoDir := t.TempDir()
	installer := app.NewInstaller(
		config.NewStore(repoDir),
		hardwareDetector{report: hardware.Report{
			OS: hardware.Linux, MemoryBytes: 96 * hardware.GiB, DiskBytes: 200 * hardware.GiB,
			GPU: hardware.GPU{Vendor: hardware.NVIDIA, Runtime: hardware.CUDA, VRAMBytes: 24 * hardware.GiB, Usable: true},
		}},
		&installPrompt{
			dataDir:  safeDataDir(t),
			workload: models.Complete,
			services: config.Services{Knowledge: true},
			models:   []string{"qwen3.5:9b", "qwen3-embedding:4b"},
		},
		&recordingExecutor{},
	)

	if err := installer.Run(context.Background(), app.InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	installation, err := config.NewStore(repoDir).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(installation.Models) != 2 || installation.EmbeddingModel != "qwen3-embedding:4b" {
		t.Fatalf("embedding selection changed: %#v", installation)
	}
}

func TestInstallerUsesContextThatFitsEverySelectedChatModel(t *testing.T) {
	repoDir := t.TempDir()
	prompt := &installPrompt{
		dataDir:  safeDataDir(t),
		workload: models.Coding,
		models:   []string{"qwen3.5:9b", "devstral-small-2:24b"},
	}
	installer := app.NewInstaller(
		config.NewStore(repoDir),
		hardwareDetector{report: hardware.Report{
			OS: hardware.Linux, MemoryBytes: 64 * hardware.GiB, DiskBytes: 100 * hardware.GiB,
			GPU: hardware.GPU{
				Vendor: hardware.NVIDIA, Runtime: hardware.CUDA,
				VRAMBytes: 16376 * 1024 * 1024, Usable: true,
			},
		}},
		prompt,
		&recordingExecutor{},
	)

	if err := installer.Run(context.Background(), app.InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	installation, err := config.NewStore(repoDir).Load()
	if err != nil {
		t.Fatal(err)
	}
	if installation.ContextLength != 16384 {
		t.Fatalf("shared context = %d, want 16384", installation.ContextLength)
	}
	for _, model := range prompt.preview.Models {
		if model.Kind != models.Embedding && model.Context != 16384 {
			t.Fatalf("preview context for %s = %d, want shared 16384", model.Name, model.Context)
		}
	}
}

func safeDataDir(t *testing.T) string {
	t.Helper()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	return filepath.Join(homeDir, "local-ai-lab")
}
