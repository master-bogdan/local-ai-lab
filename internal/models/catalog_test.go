package models_test

import (
	"testing"

	"github.com/master-bogdan/local-ai-lab/internal/hardware"
	"github.com/master-bogdan/local-ai-lab/internal/models"
)

func TestRecommendBuildsCodingPresetForRTX4090Laptop(t *testing.T) {
	report := discreteHost(16, 64)
	report.GPU.VRAMBytes = 16376 * 1024 * 1024

	catalog := models.Recommend(report, models.Coding)

	assertSelected(t, catalog, []string{
		"qwen3.5:9b",
		"devstral-small-2:24b",
		"qwen3-embedding:0.6b",
	})
	assertModel(t, catalog, "qwen3.5:9b", models.Fast, 32768)
	assertModel(t, catalog, "devstral-small-2:24b", models.Tight, 16384)
	assertModel(t, catalog, "qwen3-coder-next", models.Unsupported, 0)
}

func TestRecommendUsesSmallCapableModelOnSixGiBGPU(t *testing.T) {
	catalog := models.Recommend(discreteHost(6, 16), models.Minimal)

	assertSelected(t, catalog, []string{"qwen3.5:4b"})
	assertModel(t, catalog, "qwen3.5:4b", models.Fast, 16384)
	assertModel(t, catalog, "qwen3.5:9b", models.Unsupported, 0)
}

func TestRecommendRequiresTwentyFourGiBAppleUnifiedMemory(t *testing.T) {
	report := hardware.Report{
		OS:          hardware.MacOS,
		MemoryBytes: 16 * hardware.GiB,
		GPU: hardware.GPU{
			Vendor: hardware.Apple, Runtime: hardware.Metal, Usable: true,
		},
	}

	catalog := models.Recommend(report, models.General)

	for _, model := range catalog {
		if model.Fit != models.Unsupported || model.Selected {
			t.Fatalf("%s should be disabled on 16 GiB Apple Silicon: %#v", model.Name, model)
		}
	}
}

func TestRecommendCompletePresetStaysSmallAndPurposeful(t *testing.T) {
	catalog := models.Recommend(discreteHost(24, 96), models.Complete)

	assertSelected(t, catalog, []string{
		"qwen3.5:9b",
		"gpt-oss:20b",
		"gemma3:12b",
		"qwen3-embedding:4b",
	})
	if len(catalog) < 10 {
		t.Fatalf("expected broad curated catalog, got %d models", len(catalog))
	}
	for _, model := range catalog {
		if model.Context > model.NativeContext {
			t.Fatalf("%s configured context %d exceeds native context %d", model.Name, model.Context, model.NativeContext)
		}
	}
}

func TestRecommendCustomPresetSelectsNothing(t *testing.T) {
	catalog := models.Recommend(discreteHost(24, 96), models.Custom)

	assertSelected(t, catalog, nil)
}

func discreteHost(vramGiB, ramGiB uint64) hardware.Report {
	return hardware.Report{
		OS:          hardware.Linux,
		MemoryBytes: ramGiB * hardware.GiB,
		GPU: hardware.GPU{
			Vendor: hardware.NVIDIA, Runtime: hardware.CUDA,
			VRAMBytes: vramGiB * hardware.GiB, Usable: true,
		},
	}
}

func assertSelected(t *testing.T, catalog []models.Model, want []string) {
	t.Helper()
	got := make([]string, 0, len(want))
	for _, model := range catalog {
		if model.Selected {
			got = append(got, model.Name)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("selected models = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("selected models = %v, want %v", got, want)
		}
	}
}

func assertModel(t *testing.T, catalog []models.Model, name string, fit models.Fit, context int) {
	t.Helper()
	for _, model := range catalog {
		if model.Name != name {
			continue
		}
		if model.Fit != fit || model.Context != context {
			t.Fatalf("%s fit/context = %s/%d, want %s/%d", name, model.Fit, model.Context, fit, context)
		}
		if model.Reason == "" {
			t.Fatalf("%s has no recommendation reason", name)
		}
		return
	}
	t.Fatalf("model %s is missing", name)
}
