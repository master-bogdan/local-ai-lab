package app_test

import (
	"testing"

	"github.com/master-bogdan/local-ai-lab/internal/app"
	"github.com/master-bogdan/local-ai-lab/internal/config"
)

func TestFullDeletionRequiresTypedConfirmationAndNeverTargetsOpenCode(t *testing.T) {
	pointerPath := "/state/local-ai-lab/installation.json"
	installation := config.Installation{DataDir: "/data/local-ai-lab"}

	plan := app.FullDeletionPlan(pointerPath, installation)

	if plan.Confirmation != "DELETE" {
		t.Fatalf("expected DELETE confirmation, got %q", plan.Confirmation)
	}
	want := map[string]bool{
		"/data/local-ai-lab": true,
		pointerPath:          true,
	}
	for _, path := range plan.Paths {
		if !want[path] {
			t.Fatalf("unexpected deletion target %q", path)
		}
		delete(want, path)
	}
	if len(want) != 0 {
		t.Fatalf("missing deletion targets: %#v", want)
	}
}

func TestPartialDeletionRetainsSelectedCategories(t *testing.T) {
	plan := app.PartialDeletionPlan(
		config.Installation{DataDir: "/data/local-ai-lab"},
		[]app.DeletionCategory{app.DeleteModels, app.DeleteCache},
	)

	if !plan.Includes(app.DeleteModels) || !plan.Includes(app.DeleteCache) {
		t.Fatalf("partial deletion lost selected categories: %#v", plan)
	}
	if plan.Includes(app.DeleteIndexes) {
		t.Fatal("partial deletion included an unselected category")
	}
}
