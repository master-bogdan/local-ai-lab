package models

import "github.com/master-bogdan/local-ai-lab/internal/hardware"

type Workload string

const (
	Coding   Workload = "coding"
	General  Workload = "general"
	Vision   Workload = "vision"
	Complete Workload = "complete"
	Minimal  Workload = "minimal"
	Custom   Workload = "custom"
)

type Fit string

const (
	Fast        Fit = "FAST"
	Tight       Fit = "TIGHT"
	Hybrid      Fit = "HYBRID"
	Unsupported Fit = "UNSUPPORTED"
)

type Kind string

const (
	Chat        Kind = "chat"
	CodingModel Kind = "coding"
	VisionModel Kind = "vision"
	Embedding   Kind = "embedding"
)

type Model struct {
	Name          string
	Purpose       string
	Kind          Kind
	SizeBytes     uint64
	NativeContext int
	Context       int
	MinVRAMBytes  uint64
	MinRAMBytes   uint64
	MinAppleBytes uint64
	Fit           Fit
	Selected      bool
	Compatible    bool
	Reason        string
	Warning       string
}

var catalog = []Model{
	{
		Name: "qwen3.5:4b", Purpose: "lightweight coding and tool calls", Kind: CodingModel,
		SizeBytes: 3400 * 1024 * 1024, NativeContext: 256000,
		MinVRAMBytes: 6 * hardware.GiB, MinRAMBytes: 16 * hardware.GiB, MinAppleBytes: 24 * hardware.GiB,
	},
	{
		Name: "qwen3.5:9b", Purpose: "fast daily coding and tool calls", Kind: CodingModel,
		SizeBytes: 6600 * 1024 * 1024, NativeContext: 256000,
		MinVRAMBytes: 8 * hardware.GiB, MinRAMBytes: 16 * hardware.GiB, MinAppleBytes: 24 * hardware.GiB,
	},
	{
		Name: "gpt-oss:20b", Purpose: "reasoning and multi-step agents", Kind: Chat,
		SizeBytes: 14 * hardware.GiB, NativeContext: 128000,
		MinVRAMBytes: 12 * hardware.GiB, MinRAMBytes: 32 * hardware.GiB, MinAppleBytes: 32 * hardware.GiB,
	},
	{
		Name: "devstral-small-2:24b", Purpose: "specialized repository-scale coding agent", Kind: CodingModel,
		SizeBytes: 15 * hardware.GiB, NativeContext: 384000,
		MinVRAMBytes: 12 * hardware.GiB, MinRAMBytes: 32 * hardware.GiB, MinAppleBytes: 32 * hardware.GiB,
	},
	{
		Name: "qwen3.6:27b", Purpose: "high-quality coding and tool use", Kind: CodingModel,
		SizeBytes: 17 * hardware.GiB, NativeContext: 256000,
		MinVRAMBytes: 12 * hardware.GiB, MinRAMBytes: 48 * hardware.GiB, MinAppleBytes: 48 * hardware.GiB,
	},
	{
		Name: "qwen3.6:35b", Purpose: "heavy coding and reasoning", Kind: CodingModel,
		SizeBytes: 24 * hardware.GiB, NativeContext: 256000,
		MinVRAMBytes: 16 * hardware.GiB, MinRAMBytes: 64 * hardware.GiB, MinAppleBytes: 64 * hardware.GiB,
	},
	{
		Name: "gemma3:12b", Purpose: "vision, screenshots, and general chat", Kind: VisionModel,
		SizeBytes: 8100 * 1024 * 1024, NativeContext: 128000,
		MinVRAMBytes: 8 * hardware.GiB, MinRAMBytes: 24 * hardware.GiB, MinAppleBytes: 24 * hardware.GiB,
	},
	{
		Name: "qwen3-coder-next", Purpose: "very large repository-scale coding", Kind: CodingModel,
		SizeBytes: 52 * hardware.GiB, NativeContext: 256000,
		MinVRAMBytes: 24 * hardware.GiB, MinRAMBytes: 64 * hardware.GiB, MinAppleBytes: 64 * hardware.GiB,
	},
	{
		Name: "gpt-oss:120b", Purpose: "high-end local reasoning", Kind: Chat,
		SizeBytes: 65 * hardware.GiB, NativeContext: 128000,
		MinVRAMBytes: 48 * hardware.GiB, MinRAMBytes: 96 * hardware.GiB, MinAppleBytes: 96 * hardware.GiB,
	},
	{
		Name: "qwen3-embedding:0.6b", Purpose: "fast workspace retrieval", Kind: Embedding,
		SizeBytes: 639 * 1024 * 1024, NativeContext: 32000,
		MinVRAMBytes: 6 * hardware.GiB, MinRAMBytes: 16 * hardware.GiB, MinAppleBytes: 24 * hardware.GiB,
	},
	{
		Name: "qwen3-embedding:4b", Purpose: "higher-quality workspace retrieval", Kind: Embedding,
		SizeBytes: 2500 * 1024 * 1024, NativeContext: 40000,
		MinVRAMBytes: 8 * hardware.GiB, MinRAMBytes: 24 * hardware.GiB, MinAppleBytes: 24 * hardware.GiB,
	},
}

func Recommend(report hardware.Report, workload Workload) []Model {
	result := make([]Model, len(catalog))
	copy(result, catalog)
	for index := range result {
		classify(&result[index], report)
	}
	selectPreset(result, workload)
	return result
}

func classify(model *Model, report hardware.Report) {
	model.Fit = Unsupported
	model.Reason = "hardware does not meet this model's minimum memory"
	if !report.GPU.Usable {
		return
	}
	if report.GPU.Vendor == hardware.Apple {
		classifyApple(model, report)
		return
	}
	vram := hardware.NormalizedVRAM(report.GPU.VRAMBytes)
	if vram < model.MinVRAMBytes || report.MemoryBytes < model.MinRAMBytes {
		return
	}

	fastCapacity := vram - min(vram, hardware.GiB)
	for _, context := range contextCandidates(vram, model.NativeContext) {
		required := model.SizeBytes + contextReserve(context)
		switch {
		case required <= fastCapacity:
			setFit(model, Fast, context, "model and context stay GPU-resident")
			return
		case required <= vram:
			setFit(model, Tight, context, "fits VRAM with limited runtime headroom")
			return
		}
	}
	if model.SizeBytes+contextReserve(16384) <= report.MemoryBytes-8*hardware.GiB {
		setFit(model, Hybrid, min(16384, model.NativeContext), "runs with CPU and system-memory offload")
	}
}

func classifyApple(model *Model, report hardware.Report) {
	if report.MemoryBytes < model.MinAppleBytes || report.MemoryBytes < 24*hardware.GiB {
		return
	}
	usable := report.MemoryBytes - 8*hardware.GiB
	for _, context := range contextCandidates(usable, model.NativeContext) {
		required := model.SizeBytes + contextReserve(context)
		switch {
		case required <= usable*3/4:
			setFit(model, Fast, context, "fits comfortably in unified memory")
			return
		case required <= usable:
			setFit(model, Tight, context, "fits unified memory with limited system headroom")
			return
		}
	}
}

func setFit(model *Model, fit Fit, context int, reason string) {
	model.Fit = fit
	model.Context = context
	model.Compatible = true
	model.Reason = reason
	if fit == Tight || fit == Hybrid {
		model.Warning = reason
	}
}

func contextCandidates(capacity uint64, native int) []int {
	maximum := 16384
	switch {
	case capacity >= 24*hardware.GiB:
		maximum = 65536
	case capacity >= 8*hardware.GiB:
		maximum = 32768
	}
	candidates := []int{maximum}
	if maximum > 32768 {
		candidates = append(candidates, 32768)
	}
	if maximum > 16384 {
		candidates = append(candidates, 16384)
	}
	for index, context := range candidates {
		candidates[index] = min(context, native)
	}
	return candidates
}

func contextReserve(context int) uint64 {
	switch {
	case context > 32768:
		return 4 * hardware.GiB
	case context > 16384:
		return 2 * hardware.GiB
	default:
		return hardware.GiB
	}
}

func selectPreset(models []Model, workload Workload) {
	preferred := map[Workload][]string{
		Coding:   {"qwen3.5:9b", "devstral-small-2:24b", "qwen3-embedding:0.6b"},
		General:  {"qwen3.5:9b", "gpt-oss:20b", "qwen3-embedding:0.6b"},
		Vision:   {"gemma3:12b", "qwen3.5:9b"},
		Complete: {"qwen3.5:9b", "gpt-oss:20b", "gemma3:12b", "qwen3-embedding:4b"},
	}
	if workload == Minimal {
		selectMinimal(models)
		return
	}
	for _, name := range preferred[workload] {
		if model := find(models, name); model != nil && model.Compatible {
			model.Selected = true
		}
	}
	if workload != Custom && !hasSelectedChatModel(models) {
		selectMinimal(models)
	}
}

func selectMinimal(models []Model) {
	for _, name := range []string{"qwen3.5:9b", "qwen3.5:4b"} {
		if model := find(models, name); model != nil && model.Fit == Fast {
			model.Selected = true
			return
		}
	}
}

func hasSelectedChatModel(models []Model) bool {
	for _, model := range models {
		if model.Selected && model.Kind != Embedding {
			return true
		}
	}
	return false
}

func find(models []Model, name string) *Model {
	for index := range models {
		if models[index].Name == name {
			return &models[index]
		}
	}
	return nil
}
