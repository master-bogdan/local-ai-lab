package hardware

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

type System interface {
	OS() string
	Arch() string
	ReadFile(string) ([]byte, error)
	Glob(string) (map[string][]byte, error)
	Run(context.Context, string, ...string) ([]byte, error)
	DiskFree(string) (uint64, error)
}

type Detector struct {
	system System
}

func NewDetector(system System) Detector {
	return Detector{system: system}
}

func (d Detector) Detect(ctx context.Context, dataPath string) (Report, error) {
	report := Report{OS: OS(d.system.OS()), Architecture: d.system.Arch()}
	var err error
	switch report.OS {
	case Linux:
		report, err = d.detectLinux(ctx, report)
	case MacOS:
		report, err = d.detectMac(ctx, report)
	case Windows:
		return report, nil
	default:
		return report, fmt.Errorf("unsupported operating system %q", report.OS)
	}
	if err != nil {
		return report, err
	}
	report.DiskBytes, err = d.system.DiskFree(dataPath)
	if err != nil {
		return report, fmt.Errorf("measure free disk at %s: %w", dataPath, err)
	}
	return report, nil
}

func (d Detector) detectLinux(ctx context.Context, report Report) (Report, error) {
	memory, err := d.system.ReadFile("/proc/meminfo")
	if err != nil {
		return report, fmt.Errorf("read Linux memory: %w", err)
	}
	report.MemoryBytes, err = parseLinuxMemory(string(memory))
	if err != nil {
		return report, err
	}
	if release, readErr := d.system.ReadFile("/etc/os-release"); readErr == nil {
		report.Distro, report.DistroVersion = parseOSRelease(string(release))
	}
	report.GPU = d.detectLinuxGPU(ctx)
	return report, nil
}

func (d Detector) detectLinuxGPU(ctx context.Context) GPU {
	output, err := d.system.Run(ctx, "nvidia-smi", "--query-gpu=name,memory.total,driver_version", "--format=csv,noheader,nounits")
	if err == nil {
		if gpu, parseErr := parseNVIDIA(string(output)); parseErr == nil {
			return gpu
		}
	}
	output, err = d.system.Run(ctx, "rocm-smi", "--showproductname", "--showmeminfo", "vram", "--json")
	if err == nil {
		if gpu, parseErr := parseROCm(output); parseErr == nil {
			return gpu
		}
	}
	return d.detectExperimentalGPU(ctx)
}

func (d Detector) detectExperimentalGPU(ctx context.Context) GPU {
	vendors, err := d.system.Glob("/sys/class/drm/card*/device/vendor")
	if err != nil {
		return GPU{}
	}
	var selected GPU
	for vendorPath, rawVendor := range vendors {
		vendor := vendorFromPCI(strings.TrimSpace(string(rawVendor)))
		if vendor == "" {
			continue
		}
		memoryPath := filepath.Join(filepath.Dir(vendorPath), "mem_info_vram_total")
		memory, err := d.system.ReadFile(memoryPath)
		if err != nil {
			continue
		}
		vram, _ := strconv.ParseUint(strings.TrimSpace(string(memory)), 10, 64)
		if vram <= selected.VRAMBytes {
			continue
		}
		selected = GPU{Vendor: vendor, Name: string(vendor) + " GPU", VRAMBytes: vram}
	}
	if selected.VRAMBytes == 0 {
		return GPU{}
	}
	if selected.Vendor == Intel {
		if _, err := d.system.Run(ctx, "xpu-smi", "--version"); err == nil {
			selected.Runtime, selected.Usable = XPU, true
			return selected
		}
	}
	if _, err := d.system.Run(ctx, "vulkaninfo", "--summary"); err == nil {
		selected.Runtime, selected.Usable = Vulkan, true
	}
	return selected
}

func vendorFromPCI(identifier string) Vendor {
	switch identifier {
	case "0x1002":
		return AMD
	case "0x8086":
		return Intel
	default:
		return ""
	}
}

func (d Detector) detectMac(ctx context.Context, report Report) (Report, error) {
	memory, err := d.system.Run(ctx, "sysctl", "-n", "hw.memsize")
	if err != nil {
		return report, fmt.Errorf("read macOS unified memory: %w", err)
	}
	report.MemoryBytes, err = strconv.ParseUint(strings.TrimSpace(string(memory)), 10, 64)
	if err != nil {
		return report, fmt.Errorf("parse macOS unified memory: %w", err)
	}
	report.Distro = "macos"
	if report.Architecture != "arm64" {
		report.GPU = GPU{Name: "Intel Mac"}
		return report, nil
	}
	report.GPU = GPU{Vendor: Apple, Name: "Apple Silicon", Runtime: Metal, Usable: true}
	return report, nil
}

func parseLinuxMemory(content string) (uint64, error) {
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "MemTotal:" {
			kilobytes, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				return 0, fmt.Errorf("parse Linux memory: %w", err)
			}
			return kilobytes * 1024, nil
		}
	}
	return 0, errors.New("missing Linux memory total")
}

func parseOSRelease(content string) (string, string) {
	values := make(map[string]string)
	for _, line := range strings.Split(content, "\n") {
		key, value, found := strings.Cut(line, "=")
		if found {
			values[key] = strings.Trim(value, `"`)
		}
	}
	return values["ID"], values["VERSION_ID"]
}

func parseNVIDIA(output string) (GPU, error) {
	fields := strings.SplitN(strings.TrimSpace(output), ",", 3)
	if len(fields) != 3 {
		return GPU{}, errors.New("unexpected nvidia-smi output")
	}
	megabytes, err := strconv.ParseUint(strings.TrimSpace(fields[1]), 10, 64)
	if err != nil {
		return GPU{}, fmt.Errorf("parse NVIDIA VRAM: %w", err)
	}
	return GPU{
		Vendor: NVIDIA, Name: strings.TrimSpace(fields[0]), VRAMBytes: megabytes * 1024 * 1024,
		Runtime: CUDA, Usable: true, Driver: strings.TrimSpace(fields[2]),
	}, nil
}

func parseROCm(output []byte) (GPU, error) {
	var cards map[string]map[string]string
	if err := json.Unmarshal(output, &cards); err != nil {
		return GPU{}, fmt.Errorf("parse rocm-smi output: %w", err)
	}
	for _, properties := range cards {
		gpu := GPU{Vendor: AMD, Runtime: ROCm, Usable: true}
		for key, value := range properties {
			switch {
			case strings.Contains(key, "Card series"):
				gpu.Name = value
			case strings.Contains(key, "Total Memory"):
				gpu.VRAMBytes, _ = strconv.ParseUint(value, 10, 64)
			}
		}
		if gpu.VRAMBytes > 0 {
			return gpu, nil
		}
	}
	return GPU{}, errors.New("ROCm GPU memory not found")
}
