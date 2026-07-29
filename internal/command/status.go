package command

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/master-bogdan/local-ai-lab/internal/config"
	labruntime "github.com/master-bogdan/local-ai-lab/internal/runtime"
)

func serviceStatus(ctx context.Context, installation config.Installation) string {
	expected := statusEndpoints(installation)
	endpoints := statusProbeEndpoints(installation)
	if len(endpoints) == 0 {
		return "Ready"
	}
	probeContext, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	responses := make(chan endpointResponse, len(endpoints))
	client := &http.Client{Timeout: time.Second}
	for name, endpoint := range endpoints {
		go probeEndpoint(probeContext, client, name, endpoint, responses)
	}
	responding, expectedResponding := 0, 0
	for range endpoints {
		response := <-responses
		if response.responding {
			responding++
			if _, ok := expected[response.name]; ok {
				expectedResponding++
			}
		}
	}
	switch {
	case responding == 0:
		return "Ready"
	case len(expected) == 0 || expectedResponding == len(expected):
		return "Running"
	default:
		return fmt.Sprintf("Partial (%d/%d services responding)", responding, len(endpoints))
	}
}

type endpointResponse struct {
	name       string
	responding bool
}

func probeEndpoint(ctx context.Context, client *http.Client, name, endpoint string, responses chan<- endpointResponse) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		responses <- endpointResponse{name: name}
		return
	}
	response, err := client.Do(request)
	if err != nil {
		responses <- endpointResponse{name: name}
		return
	}
	response.Body.Close()
	responses <- endpointResponse{name: name, responding: response.StatusCode < http.StatusInternalServerError}
}

func statusProbeEndpoints(installation config.Installation) map[string]string {
	endpoints := statusEndpoints(installation)
	endpoints["Ollama"] = "http://127.0.0.1:11434/api/version"
	if installation.Modules.ComfyUI {
		endpoints["ComfyUI"] = "http://127.0.0.1:8188/system_stats"
	}
	if installation.Services.WebUI {
		endpoints["Open WebUI"] = "http://127.0.0.1:3000/"
	}
	return endpoints
}

func statusEndpoints(installation config.Installation) map[string]string {
	endpoints := make(map[string]string)
	workload := labruntime.Workload(installation.Workload)
	if workload == labruntime.Coding || workload == labruntime.Both {
		endpoints["Ollama"] = "http://127.0.0.1:11434/api/version"
		if installation.Services.WebUI {
			endpoints["Open WebUI"] = "http://127.0.0.1:3000/"
		}
	}
	if installation.Modules.ComfyUI && (workload == labruntime.Images || workload == labruntime.Both) {
		endpoints["ComfyUI"] = "http://127.0.0.1:8188/system_stats"
	}
	if installation.Services.Search {
		endpoints["SearXNG"] = "http://127.0.0.1:8088/"
	}
	if installation.Services.Knowledge {
		endpoints["Qdrant"] = "http://127.0.0.1:6333/readyz"
	}
	if installation.Services.Monitoring {
		endpoints["Grafana"] = "http://127.0.0.1:3002/api/health"
		endpoints["Prometheus"] = "http://127.0.0.1:9090/-/ready"
		endpoints["cAdvisor"] = "http://127.0.0.1:8080/healthz"
	}
	return endpoints
}

func shouldConfirmExit(status string) bool {
	return status == "Running" || strings.HasPrefix(status, "Partial")
}
