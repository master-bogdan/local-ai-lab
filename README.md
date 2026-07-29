# Local AI Lab

Local, zero-API-cost AI workstation by [bogdanlabs.dev](https://bogdanlabs.dev) and [masterbogdan](https://github.com/masterbogdan).

Local AI Lab provides:

- coding agents through Ollama and OpenCode
- browser chat through Open WebUI
- public web search through SearXNG
- local workspace search through Qdrant
- image generation through ComfyUI
- ready-to-use monitoring through Prometheus and Grafana

Services bind to `127.0.0.1`, and the Local AI Lab CLI stays in this repository. Required Docker or GPU runtime packages may be installed system-wide only after the control center shows the exact commands and receives confirmation.

## Requirements

- Go 1.26.5 or newer
- GNU Make
- internet access during installation and for public web search
- a supported GPU and its working host driver

Supported hosts:

| Host | Minimum | Runtime |
|---|---|---|
| Fedora, Ubuntu, or Arch with NVIDIA or AMD GPU | 6 GiB VRAM, 16 GiB RAM | CUDA or ROCm |
| Linux with supported AMD or Intel GPU | 6 GiB VRAM, 16 GiB RAM | experimental Vulkan |
| Apple Silicon macOS | 24 GiB unified memory | Metal |

CPU-only systems, Intel Macs, Windows, and WSL are unsupported. Linux dependency setup can install Docker and GPU container tools, but it does not install GPU drivers. macOS requires Homebrew and Docker Desktop.

## Use

From the repository, run:

```bash
make start
```

This opens the control center. On first run it checks the hardware, asks what the lab is for, recommends a small model set, and lets you customize it. Installation downloads only the confirmed services and models, then returns with all services stopped.

Use arrow keys or `j`/`k` to move, `Enter` to select, `Space` to toggle choices, and `Esc` to go back. Every system change, download, configuration update, and deletion is reviewed before it runs.

After installation, the menu provides:

- **Start or switch workload**: coding, images, infrastructure, or both
- **Service status and URLs**: health, local addresses, and Grafana credentials
- **Follow service logs**: live container output
- **Manage models**: list, download, or remove Ollama models
- **Optional setup**: OpenCode, monitoring, and the separate ComfyUI control flow
- **Index a workspace**: add a Git repository to local knowledge search
- **Stop services**: stop runtimes without deleting data
- **Delete data**: remove selected data or the complete installation

Services can keep running after the control center exits. The exit screen asks whether to leave them running or stop them. Run `make start` again whenever you need the menu.

## Models

The installer recommends models for the detected hardware and selected workload. The picker explains every choice and labels it:

- `FAST`: model and configured context stay GPU-resident
- `TIGHT`: fits with limited runtime headroom
- `HYBRID`: uses deliberate CPU and system-memory offload
- `UNSUPPORTED`: visible for comparison but cannot be selected

| Model | Use |
|---|---|
| `qwen3.5:4b` | lightweight work on smaller GPUs |
| `qwen3.5:9b` | fast coding and tool calls |
| `gpt-oss:20b` | multi-step agentic reasoning |
| `devstral-small-2:24b` | repository-scale coding agent |
| `qwen3.6:27b` / `qwen3.6:35b` | quality coding and review |
| `gemma3:12b` | screenshots, vision, and general chat |
| `qwen3-coder-next` | very large repository-scale coding |
| `gpt-oss:120b` | high-end local reasoning |
| `qwen3-embedding:0.6b` / `qwen3-embedding:4b` | workspace retrieval |

Context is sized from available memory instead of blindly using a model's advertised maximum. Presets stay small; the complete catalog is available for custom setups.

## Images

Choose **Optional setup** -> **Image generation** to configure, start, stop, or update ComfyUI separately from the coding runtime. The same menu previews the newest generated PNG or JPEG directly in the terminal.

Image preview supports Kitty graphics, iTerm2 images, Sixel, ANSI half-blocks, and ASCII fallback. Detection covers Kitty, Ghostty, WezTerm, iTerm2, tmux, SSH sessions, and ordinary native terminals. Override detection with `LOCAL_AI_IMAGE_PROTOCOL=kitty|iterm2|sixel|ansi|off`; set `LOCAL_AI_REDUCE_MOTION=1` to disable optional UI motion.

## OpenCode

Install OpenCode manually from its [official guide](https://opencode.ai/docs), then choose **Optional setup** -> **OpenCode**. The control center previews and backs up the OpenCode configuration before connecting it to local Ollama models, the configured context, web search, and workspace search.

Start a coding workload, change to any project directory, and run `opencode` normally.

## Local Data

Default data locations:

- Linux: `${XDG_DATA_HOME:-~/.local/share}/local-ai-lab`
- macOS: `~/Library/Application Support/local-ai-lab`

Models, indexes, cached search results, generated images, service data, and monitoring history stay there until removed through **Delete data**.

All ports are loopback-only, but localhost is not authentication: other processes running as your user can access local service APIs. Public web searches and downloads still use the internet. Review repositories before indexing them if tracked files may contain secrets.

## License

Repository source and configuration are MIT licensed. See [LICENSE](LICENSE). Downloaded models, applications, containers, and dependencies keep their upstream licenses.
