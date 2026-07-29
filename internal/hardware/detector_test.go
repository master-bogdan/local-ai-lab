package hardware_test

import (
	"context"
	"errors"
	"testing"

	"github.com/master-bogdan/local-ai-lab/internal/hardware"
)

type probeSystem struct{}

func (probeSystem) OS() string { return "linux" }

func (probeSystem) ReadFile(path string) ([]byte, error) {
	switch path {
	case "/proc/meminfo":
		return []byte("MemTotal:       65536000 kB\n"), nil
	case "/etc/os-release":
		return []byte("ID=fedora\nVERSION_ID=43\n"), nil
	default:
		return nil, errors.New("not found")
	}
}

func (probeSystem) Glob(string) (map[string][]byte, error) { return nil, nil }

func (probeSystem) Run(_ context.Context, name string, _ ...string) ([]byte, error) {
	if name == "nvidia-smi" {
		return []byte("NVIDIA GeForce RTX 4090 Laptop GPU, 16376, 580.173.02\n"), nil
	}
	return nil, errors.New("command unavailable")
}

func (probeSystem) DiskFree(_ string) (uint64, error) { return 2 * 1024 * hardware.GiB, nil }

func TestDetectorReportsUsableLinuxNVIDIAGPU(t *testing.T) {
	detector := hardware.NewDetector(probeSystem{})

	report, err := detector.Detect(context.Background(), "/data")
	if err != nil {
		t.Fatalf("detect hardware: %v", err)
	}

	if report.Distro != "fedora" || report.DistroVersion != "43" {
		t.Fatalf("unexpected distribution: %s %s", report.Distro, report.DistroVersion)
	}
	if report.GPU.Vendor != hardware.NVIDIA || report.GPU.Runtime != hardware.CUDA || !report.GPU.Usable {
		t.Fatalf("unexpected GPU report: %#v", report.GPU)
	}
	if report.GPU.VRAMBytes != 16376*1024*1024 {
		t.Fatalf("unexpected VRAM: %d", report.GPU.VRAMBytes)
	}
}
