package distribution_test

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/master-bogdan/local-ai-lab/internal/distribution"
)

func TestReceiptStoresReinstallChoicesWithoutSecretsOrHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reinstall.json")
	uninstalledAt := time.Date(2026, time.July, 30, 20, 0, 0, 0, time.UTC)
	receipt := distribution.Receipt{
		Schema: 1, LastVersion: "v0.1.0",
		InstalledAt:   time.Date(2026, time.July, 29, 20, 0, 0, 0, time.UTC),
		UninstalledAt: &uninstalledAt,
		DataDir:       "/data/local-ai-lab",
		Platform:      "linux", Runtime: "cuda", Workload: "coding",
		Models:   []string{"qwen3.5:9b"},
		Services: []string{"search", "knowledge"},
		Modules:  []string{"opencode"},
	}

	if err := distribution.WriteReceipt(path, receipt); err != nil {
		t.Fatalf("write receipt: %v", err)
	}
	loaded, err := distribution.ReadReceipt(path)
	if err != nil {
		t.Fatalf("read receipt: %v", err)
	}
	if !reflect.DeepEqual(loaded, receipt) {
		t.Fatalf("receipt differs:\nwant: %#v\n got: %#v", receipt, loaded)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range [][]byte{
		[]byte("secret"), []byte("password"), []byte("prompt"),
		[]byte("searchHistory"), []byte("workspace"),
	} {
		if bytes.Contains(bytes.ToLower(payload), bytes.ToLower(forbidden)) {
			t.Fatalf("receipt contains forbidden metadata %q:\n%s", forbidden, payload)
		}
	}
}

func TestReceiptRejectsUnknownSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reinstall.json")
	if err := os.WriteFile(path, []byte(`{"schema":99}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := distribution.ReadReceipt(path); err == nil {
		t.Fatal("read receipt with unsupported schema")
	}
}

func TestReceiptOmitsUninstallTimeForActiveInstallation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reinstall.json")
	receipt := distribution.Receipt{
		Schema: 1, LastVersion: "v0.1.0",
		InstalledAt: time.Date(2026, time.August, 5, 20, 0, 0, 0, time.UTC),
	}

	if err := distribution.WriteReceipt(path, receipt); err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(payload, []byte("uninstalledAt")) {
		t.Fatalf("active install receipt contains uninstall timestamp:\n%s", payload)
	}
}
