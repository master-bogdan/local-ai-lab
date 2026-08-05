package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/master-bogdan/local-ai-lab/internal/config"
	"github.com/master-bogdan/local-ai-lab/internal/hardware"
	"github.com/master-bogdan/local-ai-lab/internal/models"
	labruntime "github.com/master-bogdan/local-ai-lab/internal/runtime"
)

var (
	ErrUnsupportedHardware = errors.New("unsupported hardware")
	ErrInstallCancelled    = errors.New("installation cancelled")
	ErrInsufficientDisk    = errors.New("insufficient disk space")
	ErrUnsafeDataDir       = errors.New("unsafe data directory")
	ErrUnsupportedModel    = errors.New("unsupported model")
)

type HardwareDetector interface {
	Detect(context.Context, string) (hardware.Report, error)
}

type InstallPrompt interface {
	DataDirectory(string) (string, error)
	Workload(hardware.Report) (models.Workload, error)
	Services(models.Workload) (config.Services, error)
	Models([]models.Model) ([]string, error)
	ConfirmInstall(InstallPreview) (bool, error)
}

type InstallOptions struct {
	DefaultDataDir    string
	AllowExperimental bool
	Preset            *InstallPreset
}

type InstallPreset struct {
	Workload models.Workload
	Models   []string
	Services config.Services
}

type InstallPreview struct {
	DataDir        string
	Hardware       hardware.Report
	Assessment     hardware.Assessment
	Services       config.Services
	Workload       models.Workload
	Models         []models.Model
	ContextLength  int
	RequiredBytes  uint64
	AvailableBytes uint64
}

type Installer struct {
	store    config.Store
	detector HardwareDetector
	prompt   InstallPrompt
	executor PlanExecutor
}

func NewInstaller(store config.Store, detector HardwareDetector, prompt InstallPrompt, executor PlanExecutor) Installer {
	return Installer{store: store, detector: detector, prompt: prompt, executor: executor}
}

func (i Installer) Run(ctx context.Context, options InstallOptions) error {
	dataDir, err := i.chooseDataDirectory(options.DefaultDataDir)
	if err != nil {
		return err
	}
	report, err := i.detector.Detect(ctx, dataDir)
	if err != nil {
		return fmt.Errorf("detect hardware: %w", err)
	}
	assessment := hardware.Assess(report, options.AllowExperimental)
	if !assessment.Supported {
		return fmt.Errorf("%w: %s", ErrUnsupportedHardware, assessment.Reason)
	}
	workload, services, selectedModels, err := i.choosePayload(report, options.Preset)
	if err != nil {
		return err
	}
	preview := buildInstallPreview(dataDir, report, assessment, workload, services, selectedModels)
	if preview.RequiredBytes > preview.AvailableBytes {
		return fmt.Errorf("%w: need %d bytes, have %d", ErrInsufficientDisk, preview.RequiredBytes, preview.AvailableBytes)
	}
	confirmed, err := i.prompt.ConfirmInstall(preview)
	if err != nil {
		return fmt.Errorf("confirm installation: %w", err)
	}
	if !confirmed {
		return ErrInstallCancelled
	}
	installation, err := installationFromPreview(preview, options.AllowExperimental)
	if err != nil {
		return err
	}
	if err := PrepareDataLayout(installation.DataDir); err != nil {
		return err
	}
	if err := i.executor.Execute(ctx, labruntime.InstallPlan(installation)); err != nil {
		return fmt.Errorf("install runtime: %w", err)
	}
	if err := i.store.Save(installation); err != nil {
		return fmt.Errorf("save installation: %w", err)
	}
	return nil
}

func PrepareDataLayout(dataDir string) error {
	directories := []string{
		"models/ollama", "models/comfyui",
		"cache/searxng", "qdrant",
		"services/open-webui", "services/grafana", "services/prometheus",
		"services/comfyui/input", "services/comfyui/output",
	}
	for _, directory := range directories {
		path := filepath.Join(dataDir, filepath.FromSlash(directory))
		if err := os.MkdirAll(path, 0o700); err != nil {
			return fmt.Errorf("create data directory %s: %w", path, err)
		}
	}
	return nil
}

func (i Installer) chooseDataDirectory(defaultPath string) (string, error) {
	dataDir, err := i.prompt.DataDirectory(defaultPath)
	if err != nil {
		return "", fmt.Errorf("choose data directory: %w", err)
	}
	if dataDir == "" {
		return "", errors.New("data directory is required")
	}
	if !isSafeDataRoot(dataDir) {
		return "", fmt.Errorf("%w: choose a dedicated directory below your home, /data, or /mnt", ErrUnsafeDataDir)
	}
	return dataDir, nil
}

func (i Installer) choosePayload(
	report hardware.Report,
	preset *InstallPreset,
) (models.Workload, config.Services, []models.Model, error) {
	if preset != nil {
		return selectPayload(report, preset.Workload, preset.Services, preset.Models)
	}
	workload, err := i.prompt.Workload(report)
	if err != nil {
		return "", config.Services{}, nil, fmt.Errorf("choose workload: %w", err)
	}
	catalog := models.Recommend(report, workload)
	names, err := i.prompt.Models(catalog)
	if err != nil {
		return "", config.Services{}, nil, fmt.Errorf("choose models: %w", err)
	}
	services, err := i.prompt.Services(workload)
	if err != nil {
		return "", config.Services{}, nil, fmt.Errorf("choose services: %w", err)
	}
	return finalizePayload(workload, catalog, services, names)
}

func selectPayload(
	report hardware.Report,
	workload models.Workload,
	services config.Services,
	names []string,
) (models.Workload, config.Services, []models.Model, error) {
	catalog := models.Recommend(report, workload)
	return finalizePayload(workload, catalog, services, append([]string(nil), names...))
}

func finalizePayload(
	workload models.Workload,
	catalog []models.Model,
	services config.Services,
	names []string,
) (models.Workload, config.Services, []models.Model, error) {
	if services.WebUI {
		services.Search = true
		services.Knowledge = true
	}
	if services.Knowledge && !containsKind(catalog, names, models.Embedding) {
		names = append(names, "qwen3-embedding:0.6b")
	}
	selected, err := selectedModels(catalog, names)
	if err != nil {
		return "", config.Services{}, nil, err
	}
	if len(selected) == 0 {
		return "", config.Services{}, nil, errors.New("at least one model is required")
	}
	return workload, services, selected, nil
}

func containsModel(names []string, wanted string) bool {
	for _, name := range names {
		if name == wanted {
			return true
		}
	}
	return false
}

func containsKind(catalog []models.Model, names []string, kind models.Kind) bool {
	for _, candidate := range catalog {
		if candidate.Kind == kind && containsModel(names, candidate.Name) {
			return true
		}
	}
	return false
}

func selectedModels(catalog []models.Model, names []string) ([]models.Model, error) {
	selected := make([]models.Model, 0, len(names))
	for _, name := range names {
		found := false
		for _, candidate := range catalog {
			if candidate.Name == name {
				found = true
				if !candidate.Compatible {
					return nil, fmt.Errorf("%w: %s", ErrUnsupportedModel, name)
				}
				candidate.Selected = true
				selected = append(selected, candidate)
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("%w: unknown catalog entry %s", ErrUnsupportedModel, name)
		}
	}
	return selected, nil
}

func buildInstallPreview(dataDir string, report hardware.Report, assessment hardware.Assessment, workload models.Workload, services config.Services, selected []models.Model) InstallPreview {
	selected = append([]models.Model(nil), selected...)
	contextLength := sharedContext(selected)
	required := uint64(12 * hardware.GiB)
	for index, model := range selected {
		required += model.SizeBytes
		if model.Kind != models.Embedding {
			selected[index].Context = contextLength
		}
	}
	required += required / 5
	return InstallPreview{
		DataDir: dataDir, Hardware: report, Assessment: assessment, Workload: workload, Services: services,
		Models: selected, ContextLength: contextLength,
		RequiredBytes: required, AvailableBytes: report.DiskBytes,
	}
}

func installationFromPreview(preview InstallPreview, experimental bool) (config.Installation, error) {
	names := make([]string, 0, len(preview.Models))
	embeddingModel := ""
	for _, model := range preview.Models {
		names = append(names, model.Name)
		if model.Kind == models.Embedding {
			embeddingModel = model.Name
			continue
		}
	}
	searXNGSecret, err := newSecret(32)
	if err != nil {
		return config.Installation{}, err
	}
	grafanaPassword, err := newSecret(18)
	if err != nil {
		return config.Installation{}, err
	}
	return config.Installation{
		DataDir: preview.DataDir, Platform: string(preview.Hardware.OS),
		Runtime: string(preview.Hardware.GPU.Runtime), GPUVendor: string(preview.Hardware.GPU.Vendor), Experimental: experimental,
		Models: names, ContextLength: preview.ContextLength, EmbeddingModel: embeddingModel,
		Services: preview.Services, ModelProfile: string(preview.Workload),
		Secrets: config.Secrets{SearXNG: searXNGSecret, Grafana: grafanaPassword},
	}, nil
}

func sharedContext(selected []models.Model) int {
	contextLength := 0
	for _, model := range selected {
		if model.Kind == models.Embedding {
			continue
		}
		if contextLength == 0 || model.Context < contextLength {
			contextLength = model.Context
		}
	}
	return contextLength
}

func newSecret(bytes int) (string, error) {
	payload := make([]byte, bytes)
	if _, err := rand.Read(payload); err != nil {
		return "", fmt.Errorf("generate local service secret: %w", err)
	}
	return hex.EncodeToString(payload), nil
}
