package hardware_test

import (
	"testing"

	"github.com/master-bogdan/local-ai-lab/internal/hardware"
)

func TestAssessRejectsHostWithoutSupportedAccelerator(t *testing.T) {
	report := hardware.Report{
		OS:          hardware.Linux,
		MemoryBytes: 32 * hardware.GiB,
		DiskBytes:   100 * hardware.GiB,
	}

	assessment := hardware.Assess(report, false)

	if assessment.Supported {
		t.Fatal("expected host without accelerator to be rejected")
	}
	if assessment.Reason != "no supported GPU detected; CPU-only operation is not supported" {
		t.Fatalf("unexpected reason: %q", assessment.Reason)
	}
}

func TestAssessAcceptsSixGiBSupportedDiscreteGPU(t *testing.T) {
	report := hardware.Report{
		OS:          hardware.Linux,
		MemoryBytes: 16 * hardware.GiB,
		DiskBytes:   100 * hardware.GiB,
		GPU: hardware.GPU{
			Vendor:    hardware.NVIDIA,
			Name:      "RTX 2060",
			VRAMBytes: 6 * hardware.GiB,
			Runtime:   hardware.CUDA,
			Usable:    true,
		},
	}

	assessment := hardware.Assess(report, false)

	if !assessment.Supported {
		t.Fatalf("expected supported host, got %q", assessment.Reason)
	}
	if assessment.Tier != hardware.Limited {
		t.Fatalf("expected limited tier, got %q", assessment.Tier)
	}
}

func TestAssessRequiresTwentyFourGiBForAppleSilicon(t *testing.T) {
	tests := []struct {
		name        string
		memoryBytes uint64
		supported   bool
	}{
		{name: "sixteen GiB is rejected", memoryBytes: 16 * hardware.GiB},
		{name: "twenty-four GiB is accepted", memoryBytes: 24 * hardware.GiB, supported: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := hardware.Report{
				OS:          hardware.MacOS,
				MemoryBytes: tt.memoryBytes,
				DiskBytes:   100 * hardware.GiB,
				GPU: hardware.GPU{
					Vendor:  hardware.Apple,
					Runtime: hardware.Metal,
					Usable:  true,
				},
			}

			assessment := hardware.Assess(report, false)

			if assessment.Supported != tt.supported {
				t.Fatalf("expected supported=%v, got reason %q", tt.supported, assessment.Reason)
			}
		})
	}
}

func TestAssessClassifiesDiscreteGPUAndRequiresExperimentalOptIn(t *testing.T) {
	tests := []struct {
		name         string
		vramGiB      uint64
		runtime      hardware.Runtime
		experimental bool
		supported    bool
		tier         hardware.Tier
	}{
		{name: "eight GiB", vramGiB: 8, runtime: hardware.CUDA, supported: true, tier: hardware.Entry},
		{name: "sixteen GiB", vramGiB: 16, runtime: hardware.CUDA, supported: true, tier: hardware.Standard},
		{name: "twenty-four GiB", vramGiB: 24, runtime: hardware.CUDA, supported: true, tier: hardware.High},
		{name: "Vulkan refused by default", vramGiB: 16, runtime: hardware.Vulkan},
		{name: "Vulkan accepted with opt-in", vramGiB: 16, runtime: hardware.Vulkan, experimental: true, supported: true, tier: hardware.Standard},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := hardware.Report{
				OS:          hardware.Linux,
				MemoryBytes: 32 * hardware.GiB,
				DiskBytes:   100 * hardware.GiB,
				GPU: hardware.GPU{
					Vendor:    hardware.NVIDIA,
					VRAMBytes: tt.vramGiB * hardware.GiB,
					Runtime:   tt.runtime,
					Usable:    true,
				},
			}

			assessment := hardware.Assess(report, tt.experimental)

			if assessment.Supported != tt.supported || assessment.Tier != tt.tier {
				t.Fatalf("got supported=%v tier=%q reason=%q", assessment.Supported, assessment.Tier, assessment.Reason)
			}
		})
	}
}

func TestAssessTreatsMarketedSixteenGiBGPUAsSixteenGiB(t *testing.T) {
	report := hardware.Report{
		OS: hardware.Linux, MemoryBytes: 64 * hardware.GiB,
		GPU: hardware.GPU{
			Vendor: hardware.NVIDIA, Runtime: hardware.CUDA,
			VRAMBytes: 16376 * 1024 * 1024, Usable: true,
		},
	}

	assessment := hardware.Assess(report, false)

	if assessment.Tier != hardware.Standard {
		t.Fatalf("tier = %q, want standard", assessment.Tier)
	}
}
