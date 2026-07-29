package app_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/master-bogdan/local-ai-lab/internal/app"
	"github.com/master-bogdan/local-ai-lab/internal/config"
	labruntime "github.com/master-bogdan/local-ai-lab/internal/runtime"
)

type workloadPrompt struct {
	calls       int
	defaultSeen labruntime.Workload
	selected    labruntime.Workload
}

func (p *workloadPrompt) ChooseWorkload(_ context.Context, defaultWorkload labruntime.Workload) (labruntime.Workload, error) {
	p.calls++
	p.defaultSeen = defaultWorkload
	return p.selected, nil
}

type planExecutor struct {
	plan labruntime.Plan
}

func (e *planExecutor) Execute(_ context.Context, plan labruntime.Plan) error {
	e.plan = plan
	return nil
}

func TestStartAlwaysAsksForWorkloadWithPreviousChoicePreselected(t *testing.T) {
	repoDir := t.TempDir()
	store := config.NewStore(repoDir)
	installation := config.Installation{
		DataDir:  filepath.Join(t.TempDir(), "data"),
		Platform: "linux",
		Workload: string(labruntime.Coding),
	}
	if err := store.Save(installation); err != nil {
		t.Fatalf("save installation: %v", err)
	}
	prompt := &workloadPrompt{selected: labruntime.Images}
	executor := &planExecutor{}
	controller := app.NewController(store, prompt, executor)

	if err := controller.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}

	if prompt.calls != 1 || prompt.defaultSeen != labruntime.Coding {
		t.Fatalf("prompt calls=%d default=%q", prompt.calls, prompt.defaultSeen)
	}
	if executor.plan.Workload != labruntime.Images {
		t.Fatalf("expected images plan, got %q", executor.plan.Workload)
	}
}
