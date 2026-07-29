package comfy

import "github.com/master-bogdan/local-ai-lab/internal/hardware"

type Asset struct {
	Name      string
	URL       string
	Path      string
	SHA256    string
	SizeBytes uint64
}

type Pack struct {
	Name        string
	Description string
	License     string
	Assets      []Asset
}

var sd15 = Pack{
	Name: "sd15", Description: "Stable Diffusion 1.5 starter for 6 GiB GPUs",
	License: "CreativeML Open RAIL-M",
	Assets: []Asset{{
		Name:      "v1-5-pruned-emaonly-fp16.safetensors",
		URL:       "https://huggingface.co/Comfy-Org/stable-diffusion-v1-5-archive/resolve/main/v1-5-pruned-emaonly-fp16.safetensors",
		Path:      "checkpoints/v1-5-pruned-emaonly-fp16.safetensors",
		SHA256:    "e9476a13728cd75d8279f6ec8bad753a66a1957ca375a1464dc63b37db6e3916",
		SizeBytes: 2130 * 1000 * 1000,
	}},
}

var flux2Klein = Pack{
	Name: "flux2-klein-4b", Description: "FLUX.2 Klein 4B FP8 text-to-image and editing",
	License: "Apache-2.0",
	Assets: []Asset{
		{
			Name:      "flux-2-klein-4b-fp8.safetensors",
			URL:       "https://huggingface.co/black-forest-labs/FLUX.2-klein-4b-fp8/resolve/main/flux-2-klein-4b-fp8.safetensors",
			Path:      "diffusion_models/flux-2-klein-4b-fp8.safetensors",
			SHA256:    "97ed34fe0567e436200f2faee3939b88f2b5d99f8af2a4dc16532c4245c0ccb6",
			SizeBytes: 4070624520,
		},
		{
			Name:      "qwen_3_4b.safetensors",
			URL:       "https://huggingface.co/Comfy-Org/vae-text-encorder-for-flux-klein-4b/resolve/main/split_files/text_encoders/qwen_3_4b.safetensors",
			Path:      "text_encoders/qwen_3_4b.safetensors",
			SHA256:    "6c671498573ac2f7a5501502ccce8d2b08ea6ca2f661c458e708f36b36edfc5a",
			SizeBytes: 8040 * 1000 * 1000,
		},
		{
			Name:      "flux2-vae.safetensors",
			URL:       "https://huggingface.co/Comfy-Org/vae-text-encorder-for-flux-klein-4b/resolve/main/split_files/vae/flux2-vae.safetensors",
			Path:      "vae/flux2-vae.safetensors",
			SHA256:    "868fe7b343cc8f3a19dbcfcafbc3d5f888802be3f89bd81b65b3621a066ce8f3",
			SizeBytes: 336 * 1000 * 1000,
		},
	},
}

func StarterPack(report hardware.Report) Pack {
	if report.GPU.Vendor != hardware.Apple && report.GPU.VRAMBytes < 8*hardware.GiB {
		return sd15
	}
	return flux2Klein
}

func Catalog() []Pack {
	return []Pack{sd15, flux2Klein}
}

func (p Pack) TotalBytes() uint64 {
	var total uint64
	for _, asset := range p.Assets {
		total += asset.SizeBytes
	}
	return total
}
