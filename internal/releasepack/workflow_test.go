package releasepack_test

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestGitHubWorkflowsAreValidYAML(t *testing.T) {
	projectDir, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ci.yml", "release.yml"} {
		t.Run(name, func(t *testing.T) {
			payload, err := os.ReadFile(filepath.Join(projectDir, ".github", "workflows", name))
			if err != nil {
				t.Fatal(err)
			}
			var document yaml.Node
			if err := yaml.Unmarshal(payload, &document); err != nil {
				t.Fatalf("parse workflow: %v", err)
			}
		})
	}
}
