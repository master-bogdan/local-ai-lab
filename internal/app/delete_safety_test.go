package app

import (
	"os"
	"testing"
)

func TestSafeDataRootRejectsBroadSystemAndHomePaths(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/", "/tmp", "/home", homeDir, "relative/path"} {
		if isSafeDataRoot(path) {
			t.Errorf("isSafeDataRoot(%q) = true", path)
		}
	}
	if !isSafeDataRoot("/data/local-ai-lab") {
		t.Fatal("normal dedicated data directory was rejected")
	}
}

func TestDeletionTargetMustStayInsideDataRoot(t *testing.T) {
	if isSafeDeletionTarget("/data/local-ai-lab", "/data/other") {
		t.Fatal("deletion escaped the installation data root")
	}
	if !isSafeDeletionTarget("/data/local-ai-lab", "/data/local-ai-lab/models") {
		t.Fatal("model directory inside data root was rejected")
	}
}
