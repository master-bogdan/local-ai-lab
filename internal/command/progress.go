package command

import (
	"context"
	"io"
	"strings"

	labruntime "github.com/master-bogdan/local-ai-lab/internal/runtime"
	"github.com/master-bogdan/local-ai-lab/internal/ui"
)

type planExecutor interface {
	Execute(context.Context, labruntime.Plan) error
}

type interactivePlanExecutor struct {
	appRoot  string
	terminal *ui.Terminal
}

func (e interactivePlanExecutor) Execute(ctx context.Context, plan labruntime.Plan) error {
	title, success := planPresentation(plan)
	return e.terminal.RunTask(ctx, title, success, func(taskContext context.Context, output io.Writer) error {
		executor := labruntime.CommandExecutor{Dir: e.appRoot, Stdout: output, Stderr: output}
		return executor.Execute(taskContext, plan)
	})
}

func planPresentation(plan labruntime.Plan) (string, string) {
	if planHasKind(plan, labruntime.StopAll) && (planPullsImages(plan) || pulledModel(plan) != "") {
		return "Install Local AI Lab", "Installation complete"
	}
	if model := pulledModel(plan); model != "" {
		return "Download " + model, "Model ready"
	}
	if plan.Workload != "" && plan.LeavesServicesRunning() {
		name := string(plan.Workload)
		return "Start " + name + " workload", strings.ToUpper(name[:1]) + name[1:] + " workload running"
	}
	if planContains(plan, "logs") {
		return "Service logs", "Log stream closed"
	}
	if planContains(plan, "ps") {
		return "Service status", "Status refreshed"
	}
	if planContains(plan, "down") {
		return "Stop services", "Services stopped"
	}
	if planContains(plan, "rm") {
		return "Remove model", "Model removed"
	}
	if planContains(plan, "list") {
		return "Installed models", "Model list refreshed"
	}
	if planContains(plan, "build") {
		return "Build image runtime", "Image runtime ready"
	}
	if planPullsImages(plan) {
		return "Download service images", "Service images ready"
	}
	return "Apply local AI changes", "Changes complete"
}

func planHasKind(plan labruntime.Plan, kind labruntime.StepKind) bool {
	for _, step := range plan.Steps {
		if step.Kind == kind {
			return true
		}
	}
	return false
}

func pulledModel(plan labruntime.Plan) string {
	for _, step := range plan.Steps {
		if step.Kind == labruntime.PullModel {
			return step.Model
		}
	}
	return ""
}

func planPullsImages(plan labruntime.Plan) bool {
	for _, step := range plan.Steps {
		if step.Kind == labruntime.PullImages {
			return true
		}
	}
	return false
}

func planContains(plan labruntime.Plan, argument string) bool {
	for _, step := range plan.Steps {
		for _, candidate := range step.Args {
			if candidate == argument {
				return true
			}
		}
	}
	return false
}
