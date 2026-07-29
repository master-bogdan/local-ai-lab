package command

import "testing"

func TestUbuntuNVIDIADependencyPlanConfiguresOfficialRepository(t *testing.T) {
	steps, err := linuxDependencySteps("ubuntu", dependencyState{
		dockerMissing:  true,
		dockerNotReady: true,
		nvidia:         true,
		toolkitMissing: true,
		username:       "alice",
	})
	if err != nil {
		t.Fatal(err)
	}

	assertStep(t, steps, "refresh Ubuntu packages")
	assertStep(t, steps, "grant Docker access to alice")
	assertStep(t, steps, "configure NVIDIA apt repository")
	assertStep(t, steps, "install NVIDIA container toolkit")
	assertStep(t, steps, "configure NVIDIA Docker runtime")
}

func TestArchDependencyPlanIsNonInteractive(t *testing.T) {
	steps, err := linuxDependencySteps("arch", dependencyState{dockerMissing: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range steps {
		if step.command == "sudo" && containsString(step.args, "pacman") && !containsString(step.args, "--noconfirm") {
			t.Fatalf("pacman step is interactive: %#v", step)
		}
	}
}

func assertStep(t *testing.T, steps []dependencyStep, description string) {
	t.Helper()
	for _, step := range steps {
		if step.description == description {
			return
		}
	}
	t.Fatalf("missing dependency step %q: %#v", description, steps)
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
