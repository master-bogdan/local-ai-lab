package app

import (
	"context"
	"fmt"

	"github.com/master-bogdan/local-ai-lab/internal/config"
	labruntime "github.com/master-bogdan/local-ai-lab/internal/runtime"
)

type WorkloadPrompt interface {
	ChooseWorkload(context.Context, labruntime.Workload) (labruntime.Workload, error)
}

type PlanExecutor interface {
	Execute(context.Context, labruntime.Plan) error
}

type Controller struct {
	store    config.Store
	prompt   WorkloadPrompt
	executor PlanExecutor
}

func NewController(store config.Store, prompt WorkloadPrompt, executor PlanExecutor) Controller {
	return Controller{store: store, prompt: prompt, executor: executor}
}

func (c Controller) Start(ctx context.Context) error {
	installation, err := c.store.Load()
	if err != nil {
		return err
	}
	defaultWorkload := labruntime.Workload(installation.Workload)
	if defaultWorkload == "" {
		defaultWorkload = labruntime.Coding
	}
	workload, err := c.prompt.ChooseWorkload(ctx, defaultWorkload)
	if err != nil {
		return fmt.Errorf("choose workload: %w", err)
	}
	return c.startWorkload(ctx, installation, workload)
}

func (c Controller) StartWorkload(ctx context.Context, workload labruntime.Workload) error {
	installation, err := c.store.Load()
	if err != nil {
		return err
	}
	return c.startWorkload(ctx, installation, workload)
}

func (c Controller) startWorkload(ctx context.Context, installation config.Installation, workload labruntime.Workload) error {
	previous := labruntime.Workload(installation.Workload)
	if err := c.executor.Execute(ctx, labruntime.SwitchPlan(installation, previous, workload)); err != nil {
		return fmt.Errorf("start workload: %w", err)
	}
	installation.Workload = string(workload)
	if err := c.store.Save(installation); err != nil {
		return fmt.Errorf("save workload choice: %w", err)
	}
	return nil
}
