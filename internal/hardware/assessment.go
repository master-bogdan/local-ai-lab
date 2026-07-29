package hardware

const GiB uint64 = 1024 * 1024 * 1024

type OS string

const (
	Linux   OS = "linux"
	MacOS   OS = "darwin"
	Windows OS = "windows"
)

type Vendor string
type Runtime string
type Tier string

const (
	NVIDIA   Vendor  = "nvidia"
	AMD      Vendor  = "amd"
	Intel    Vendor  = "intel"
	Apple    Vendor  = "apple"
	CUDA     Runtime = "cuda"
	Metal    Runtime = "metal"
	ROCm     Runtime = "rocm"
	Vulkan   Runtime = "vulkan"
	XPU      Runtime = "xpu"
	Limited  Tier    = "limited"
	Entry    Tier    = "entry"
	Standard Tier    = "standard"
	High     Tier    = "high"
)

type GPU struct {
	Vendor    Vendor
	Name      string
	VRAMBytes uint64
	Runtime   Runtime
	Usable    bool
	Driver    string
}

type Report struct {
	OS            OS
	Distro        string
	DistroVersion string
	MemoryBytes   uint64
	DiskBytes     uint64
	GPU           GPU
}

type Assessment struct {
	Supported bool
	Reason    string
	Tier      Tier
}

func Assess(report Report, allowExperimental bool) Assessment {
	if report.OS == MacOS && report.GPU.Vendor == Apple && report.GPU.Usable {
		if report.MemoryBytes >= 24*GiB {
			return Assessment{Supported: true, Tier: Standard}
		}
		return Assessment{Reason: "Apple Silicon requires at least 24 GiB unified memory"}
	}
	if !report.GPU.Usable {
		return Assessment{Reason: "no supported GPU detected; CPU-only operation is not supported"}
	}
	if (report.GPU.Runtime == Vulkan || report.GPU.Runtime == XPU) && !allowExperimental {
		return Assessment{Reason: "detected GPU requires experimental runtime; rerun with --experimental"}
	}
	if report.MemoryBytes < 16*GiB {
		return Assessment{Reason: "discrete GPU hosts require at least 16 GiB system memory"}
	}
	vram := NormalizedVRAM(report.GPU.VRAMBytes)
	if vram >= 24*GiB {
		return Assessment{Supported: true, Tier: High}
	}
	if vram >= 16*GiB {
		return Assessment{Supported: true, Tier: Standard}
	}
	if vram >= 8*GiB {
		return Assessment{Supported: true, Tier: Entry}
	}
	if vram >= 6*GiB {
		return Assessment{Supported: true, Tier: Limited}
	}
	return Assessment{Reason: "supported discrete GPU requires at least 6 GiB VRAM"}
}

func NormalizedVRAM(bytes uint64) uint64 {
	const tolerance = 256 * 1024 * 1024
	roundedUp := (bytes + GiB - 1) / GiB * GiB
	if roundedUp-bytes <= tolerance {
		return roundedUp
	}
	return bytes
}
