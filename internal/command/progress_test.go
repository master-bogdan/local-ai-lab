package command

import (
	"testing"

	"github.com/master-bogdan/local-ai-lab/internal/config"
	labruntime "github.com/master-bogdan/local-ai-lab/internal/runtime"
)

func TestPlanPresentationNamesUserOperation(t *testing.T) {
	installation := config.Installation{Platform: "linux"}
	tests := []struct {
		name, wantTitle, wantSuccess string
		plan                         labruntime.Plan
	}{
		{name: "install", plan: labruntime.InstallPlan(config.Installation{Platform: "linux", Models: []string{"qwen3.5:9b"}}), wantTitle: "Install Local AI Lab", wantSuccess: "Installation complete"},
		{name: "model", plan: labruntime.ModelPullPlan(installation, "qwen3.5:9b"), wantTitle: "Download qwen3.5:9b", wantSuccess: "Model ready"},
		{name: "start", plan: labruntime.StartPlan(installation, labruntime.Coding), wantTitle: "Start coding workload", wantSuccess: "Coding workload running"},
		{name: "status", plan: labruntime.StatusPlan(installation), wantTitle: "Service status", wantSuccess: "Status refreshed"},
		{name: "logs", plan: labruntime.LogsPlan(installation), wantTitle: "Service logs", wantSuccess: "Log stream closed"},
		{name: "stop", plan: labruntime.StopPlan(installation), wantTitle: "Stop services", wantSuccess: "Services stopped"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			title, success := planPresentation(test.plan)
			if title != test.wantTitle || success != test.wantSuccess {
				t.Fatalf("presentation = %q / %q, want %q / %q", title, success, test.wantTitle, test.wantSuccess)
			}
		})
	}
}
