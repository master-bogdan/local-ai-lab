package command

import (
	"context"
	"strings"
	"testing"

	"github.com/master-bogdan/local-ai-lab/internal/distribution"
)

func TestBootstrapApplicationRejectsDevelopmentBuildBeforeInteraction(t *testing.T) {
	err := BootstrapApplication(context.Background(), distribution.Layout{}, "dev", nil)
	if err == nil || !strings.Contains(err.Error(), "not built for a release") {
		t.Fatalf("bootstrap error = %v", err)
	}
}
