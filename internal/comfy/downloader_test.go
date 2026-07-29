package comfy_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/master-bogdan/local-ai-lab/internal/comfy"
)

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestDownloaderDoesNotFollowPartialSymlinkOutsideModelRoot(t *testing.T) {
	root := t.TempDir()
	payload := []byte("verified model payload")
	digest := sha256.Sum256(payload)
	asset := comfy.Asset{
		Name: "model.safetensors", URL: "https://huggingface.co/example/model.safetensors",
		Path: "checkpoints/model.safetensors", SHA256: hex.EncodeToString(digest[:]), SizeBytes: uint64(len(payload)),
	}
	destination := filepath.Join(root, "checkpoints", "model.safetensors")
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(victim, []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, destination+".part"); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: responseTransport(payload)}

	if err := comfy.NewDownloader(client).Install(context.Background(), comfy.Pack{Assets: []comfy.Asset{asset}}, root, nil); err == nil {
		t.Fatal("download followed partial symlink outside model root")
	}
	got, err := os.ReadFile(victim)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "unchanged" {
		t.Fatalf("partial symlink target was overwritten: %q", got)
	}
}

func TestDownloaderRejectsResponseLargerThanCatalogSize(t *testing.T) {
	root := t.TempDir()
	payload := []byte("oversized model payload")
	digest := sha256.Sum256(payload)
	asset := comfy.Asset{
		Name: "model.safetensors", URL: "https://huggingface.co/example/model.safetensors",
		Path: "checkpoints/model.safetensors", SHA256: hex.EncodeToString(digest[:]), SizeBytes: 4,
	}

	err := comfy.NewDownloader(&http.Client{Transport: responseTransport(payload)}).
		Install(context.Background(), comfy.Pack{Assets: []comfy.Asset{asset}}, root, nil)
	if err == nil {
		t.Fatal("download accepted response larger than catalog size")
	}
	if _, statErr := os.Stat(filepath.Join(root, "checkpoints", "model.safetensors.part")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("oversized partial remains: %v", statErr)
	}
}

func TestDownloaderRejectsHTTPSDowngrade(t *testing.T) {
	payload := []byte("verified model payload")
	digest := sha256.Sum256(payload)
	asset := comfy.Asset{
		Name: "model.safetensors", URL: "https://huggingface.co/example/model.safetensors",
		Path: "checkpoints/model.safetensors", SHA256: hex.EncodeToString(digest[:]), SizeBytes: uint64(len(payload)),
	}
	insecureRequests := 0
	client := &http.Client{Transport: roundTripper(func(request *http.Request) (*http.Response, error) {
		if request.URL.Scheme == "http" {
			insecureRequests++
			return &http.Response{
				StatusCode: http.StatusOK, Status: "200 OK", Request: request,
				Body: io.NopCloser(bytes.NewReader(payload)), ContentLength: int64(len(payload)),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusFound, Status: "302 Found", Request: request,
			Header: http.Header{"Location": []string{"http://downloads.example/model"}},
			Body:   io.NopCloser(bytes.NewReader(nil)),
		}, nil
	})}

	err := comfy.NewDownloader(client).Install(context.Background(), comfy.Pack{Assets: []comfy.Asset{asset}}, t.TempDir(), nil)
	if err == nil {
		t.Fatal("download accepted HTTPS to HTTP downgrade")
	}
	if insecureRequests != 0 {
		t.Fatalf("download contacted insecure redirect target %d time(s)", insecureRequests)
	}
}

func responseTransport(payload []byte) roundTripper {
	return func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK, Status: "200 OK", Request: request,
			Body: io.NopCloser(bytes.NewReader(payload)), ContentLength: int64(len(payload)),
		}, nil
	}
}

func TestDownloaderPromotesVerifiedCompletePartialWithoutNetwork(t *testing.T) {
	root := t.TempDir()
	payload := []byte("verified model payload")
	digest := sha256.Sum256(payload)
	asset := comfy.Asset{
		Name: "model.safetensors", URL: "https://huggingface.co/example/model.safetensors",
		Path: "checkpoints/model.safetensors", SHA256: hex.EncodeToString(digest[:]), SizeBytes: uint64(len(payload)),
	}
	destination := filepath.Join(root, "checkpoints", "model.safetensors")
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination+".part", payload, 0o600); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: roundTripper(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("network must not be used for a complete verified partial")
	})}

	if err := comfy.NewDownloader(client).Install(context.Background(), comfy.Pack{Assets: []comfy.Asset{asset}}, root, nil); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("promoted payload = %q", got)
	}
	if _, err := os.Stat(destination + ".part"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial file remains after promotion: %v", err)
	}
}
