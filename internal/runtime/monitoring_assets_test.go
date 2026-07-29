package runtime_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestMonitoringAssetsProvideReadyToUseDashboard(t *testing.T) {
	root := filepath.Join("..", "..", "deploy", "monitoring")
	for _, path := range []string{
		"prometheus/prometheus.yml",
		"prometheus/rules/local-ai-lab.yml",
		"grafana/provisioning/datasources/datasource.yml",
		"grafana/provisioning/dashboards/dashboard.yml",
	} {
		contents, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var document any
		if err := yaml.Unmarshal(contents, &document); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
	}

	contents, err := os.ReadFile(filepath.Join(root, "grafana", "dashboards", "local-ai-lab.json"))
	if err != nil {
		t.Fatalf("read dashboard: %v", err)
	}
	var dashboard struct {
		Title  string `json:"title"`
		UID    string `json:"uid"`
		Panels []struct {
			Title string `json:"title"`
		} `json:"panels"`
	}
	if err := json.Unmarshal(contents, &dashboard); err != nil {
		t.Fatalf("parse dashboard: %v", err)
	}
	if dashboard.Title != "Local AI Lab Overview" || dashboard.UID != "local-ai-lab" {
		t.Fatalf("unexpected dashboard identity: %#v", dashboard)
	}
	wantedPanels := map[string]bool{
		"CPU by Service": false, "Memory by Service": false,
		"Network by Service": false, "Disk I/O by Service": false,
		"OOM Events (5m)": false, "Firing Alerts": false,
	}
	for _, panel := range dashboard.Panels {
		if _, ok := wantedPanels[panel.Title]; ok {
			wantedPanels[panel.Title] = true
		}
	}
	for title, found := range wantedPanels {
		if !found {
			t.Errorf("dashboard is missing %q panel", title)
		}
	}
}
