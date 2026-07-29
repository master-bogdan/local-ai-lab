package terminalimage_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/master-bogdan/local-ai-lab/internal/terminalimage"
)

func TestDetectChoosesSafeProtocolForPopularTerminals(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want terminalimage.Protocol
	}{
		{name: "kitty", env: map[string]string{"KITTY_WINDOW_ID": "1"}, want: terminalimage.Kitty},
		{name: "ghostty", env: map[string]string{"TERM_PROGRAM": "ghostty"}, want: terminalimage.Kitty},
		{name: "wezterm", env: map[string]string{"TERM_PROGRAM": "WezTerm"}, want: terminalimage.Kitty},
		{name: "iterm", env: map[string]string{"TERM_PROGRAM": "iTerm.app"}, want: terminalimage.ITerm2},
		{name: "sixel terminfo", env: map[string]string{"TERM": "xterm-sixel"}, want: terminalimage.Sixel},
		{name: "apple terminal fallback", env: map[string]string{"TERM_PROGRAM": "Apple_Terminal"}, want: terminalimage.ANSI},
		{name: "ssh fallback", env: map[string]string{"TERM_PROGRAM": "WezTerm", "SSH_CONNECTION": "remote"}, want: terminalimage.ANSI},
		{name: "no color fallback", env: map[string]string{"TERM_PROGRAM": "WezTerm", "NO_COLOR": "1"}, want: terminalimage.ASCII},
		{name: "explicit override", env: map[string]string{"LOCAL_AI_IMAGE_PROTOCOL": "iterm2"}, want: terminalimage.ITerm2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := terminalimage.Detect(tt.env); got != tt.want {
				t.Fatalf("protocol = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenderUsesSelectedTerminalProtocol(t *testing.T) {
	path := writeImage(t)
	tests := []struct {
		protocol terminalimage.Protocol
		marker   string
	}{
		{protocol: terminalimage.Kitty, marker: "\x1b_G"},
		{protocol: terminalimage.ITerm2, marker: "\x1b]1337;"},
		{protocol: terminalimage.Sixel, marker: "\x1bP"},
		{protocol: terminalimage.ANSI, marker: "\x1b[38;2;"},
	}

	for _, tt := range tests {
		t.Run(string(tt.protocol), func(t *testing.T) {
			var output bytes.Buffer
			renderer := terminalimage.New(tt.protocol, false)
			if err := renderer.Render(&output, path, 12, 6); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output.String(), tt.marker) {
				t.Fatalf("%s output is missing protocol marker %q", tt.protocol, tt.marker)
			}
		})
	}
}

func TestASCIIRenderContainsNoControlSequences(t *testing.T) {
	var output bytes.Buffer

	err := terminalimage.New(terminalimage.ASCII, false).Render(&output, writeImage(t), 12, 6)

	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "\x1b") {
		t.Fatalf("ASCII preview contains terminal control sequence: %q", output.String())
	}
}

func TestRenderRejectsSymlink(t *testing.T) {
	target := writeImage(t)
	link := filepath.Join(t.TempDir(), "preview.png")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	err := terminalimage.New(terminalimage.ANSI, false).Render(&bytes.Buffer{}, link, 12, 6)

	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink rejection, got %v", err)
	}
}

func writeImage(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "preview.png")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := range 4 {
		for x := range 4 {
			img.Set(x, y, color.RGBA{R: uint8(50 * x), G: uint8(50 * y), B: 160, A: 255})
		}
	}
	if err := png.Encode(file, img); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}
