package comfy_test

import (
	"testing"

	"github.com/master-bogdan/local-ai-lab/internal/comfy"
	"github.com/master-bogdan/local-ai-lab/internal/hardware"
)

func TestStarterPackMatchesAvailableAcceleratorMemory(t *testing.T) {
	limited := comfy.StarterPack(hardware.Report{GPU: hardware.GPU{VRAMBytes: 6 * hardware.GiB}})
	if limited.Name != "sd15" {
		t.Fatalf("expected sd15 for 6 GiB GPU, got %q", limited.Name)
	}
	standard := comfy.StarterPack(hardware.Report{GPU: hardware.GPU{VRAMBytes: 16 * hardware.GiB}})
	if standard.Name != "flux2-klein-4b" {
		t.Fatalf("expected Flux.2 Klein for 16 GiB GPU, got %q", standard.Name)
	}
}
