package terminalimage

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/iterm2"
	"github.com/charmbracelet/x/ansi/kitty"
	"github.com/charmbracelet/x/ansi/sixel"
)

const (
	maxImageBytes  = 64 * 1024 * 1024
	maxImagePixels = 24_000_000
)

type Protocol string

const (
	Kitty  Protocol = "kitty"
	ITerm2 Protocol = "iterm2"
	Sixel  Protocol = "sixel"
	ANSI   Protocol = "ansi"
	ASCII  Protocol = "ascii"
)

type Renderer struct {
	protocol Protocol
	tmux     bool
}

func Detect(env map[string]string) Protocol {
	if override := parseProtocol(env["LOCAL_AI_IMAGE_PROTOCOL"]); override != "" {
		return override
	}
	if env["NO_COLOR"] != "" {
		return ASCII
	}
	if env["SSH_CONNECTION"] != "" || env["SSH_TTY"] != "" {
		return ANSI
	}
	program := strings.ToLower(env["TERM_PROGRAM"])
	term := strings.ToLower(env["TERM"])
	switch {
	case env["KITTY_WINDOW_ID"] != "",
		env["GHOSTTY_RESOURCES_DIR"] != "",
		strings.Contains(program, "ghostty"),
		strings.Contains(program, "wezterm"),
		strings.Contains(term, "kitty"):
		return Kitty
	case strings.Contains(program, "iterm"):
		return ITerm2
	case strings.Contains(term, "sixel"):
		return Sixel
	default:
		return ANSI
	}
}

func parseProtocol(value string) Protocol {
	switch Protocol(strings.ToLower(strings.TrimSpace(value))) {
	case Kitty:
		return Kitty
	case ITerm2:
		return ITerm2
	case Sixel:
		return Sixel
	case ANSI:
		return ANSI
	case ASCII, "off":
		return ASCII
	default:
		return ""
	}
}

func New(protocol Protocol, tmux bool) Renderer {
	if parseProtocol(string(protocol)) == "" {
		protocol = ANSI
	}
	return Renderer{protocol: protocol, tmux: tmux}
}

func (r Renderer) Protocol() Protocol {
	return r.protocol
}

func (r Renderer) Render(output io.Writer, path string, columns, rows int) error {
	source, payload, err := load(path)
	if err != nil {
		return err
	}
	columns = min(max(columns, 8), 120)
	rows = min(max(rows, 4), 60)

	switch r.protocol {
	case Kitty:
		err = r.renderKitty(output, source, columns, rows)
	case ITerm2:
		err = r.renderITerm2(output, payload, columns, rows)
	case Sixel:
		err = r.renderSixel(output, source, columns, rows)
	case ASCII:
		err = renderASCII(output, source, columns, rows)
	default:
		err = renderANSI(output, source, columns, rows)
	}
	if err != nil {
		return fmt.Errorf("render %s terminal image: %w", r.protocol, err)
	}
	return nil
}

func load(path string) (image.Image, []byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, nil, fmt.Errorf("inspect image: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, nil, errors.New("image preview refuses symlink")
	}
	if !info.Mode().IsRegular() {
		return nil, nil, errors.New("image preview requires a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open image: %w", err)
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil {
		return nil, nil, fmt.Errorf("inspect opened image: %w", err)
	}
	if !os.SameFile(info, openedInfo) {
		return nil, nil, errors.New("image changed while opening")
	}
	if openedInfo.Size() > maxImageBytes {
		return nil, nil, fmt.Errorf("image exceeds %d MiB preview limit", maxImageBytes/(1024*1024))
	}
	payload, err := io.ReadAll(io.LimitReader(file, maxImageBytes+1))
	if err != nil {
		return nil, nil, fmt.Errorf("read image: %w", err)
	}
	if len(payload) > maxImageBytes {
		return nil, nil, fmt.Errorf("image exceeds %d MiB preview limit", maxImageBytes/(1024*1024))
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(payload))
	if err != nil {
		return nil, nil, fmt.Errorf("decode image metadata: %w", err)
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > maxImagePixels/config.Height {
		return nil, nil, fmt.Errorf("image exceeds %d megapixel preview limit", maxImagePixels/1_000_000)
	}
	decoded, _, err := image.Decode(bytes.NewReader(payload))
	if err != nil {
		return nil, nil, fmt.Errorf("decode image: %w", err)
	}
	return decoded, payload, nil
}

func (r Renderer) renderKitty(output io.Writer, source image.Image, columns, rows int) error {
	var encoded bytes.Buffer
	err := kitty.EncodeGraphics(&encoded, source, &kitty.Options{
		Action: kitty.TransmitAndPut, Format: kitty.PNG,
		Chunk: true, Columns: columns, Rows: rows, Quite: 2,
	})
	if err != nil {
		return err
	}
	_, err = io.WriteString(output, r.wrap(encoded.String()))
	return err
}

func (r Renderer) renderITerm2(output io.Writer, payload []byte, columns, rows int) error {
	encoded := make([]byte, base64.StdEncoding.EncodedLen(len(payload)))
	base64.StdEncoding.Encode(encoded, payload)
	sequence := ansi.ITerm2(iterm2.File{
		Size: int64(len(payload)), Width: iterm2.Cells(columns), Height: iterm2.Cells(rows),
		Inline: true, Content: encoded,
	})
	_, err := io.WriteString(output, r.wrap(sequence))
	return err
}

func (r Renderer) renderSixel(output io.Writer, source image.Image, columns, rows int) error {
	scaled := resize(source, columns*8, rows*12)
	var payload bytes.Buffer
	if err := (&sixel.Encoder{}).Encode(&payload, scaled); err != nil {
		return err
	}
	_, err := io.WriteString(output, r.wrap(ansi.SixelGraphics(0, 1, 0, payload.Bytes())))
	return err
}

func (r Renderer) wrap(sequence string) string {
	if !r.tmux {
		return sequence
	}
	escaped := strings.ReplaceAll(sequence, "\x1b", "\x1b\x1b")
	return "\x1bPtmux;\x1b" + escaped + "\x1b\\"
}

func renderANSI(output io.Writer, source image.Image, columns, rows int) error {
	scaled := resize(source, columns, rows*2)
	bounds := scaled.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y += 2 {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			topR, topG, topB := rgb(scaled.At(x, y))
			bottomY := min(y+1, bounds.Max.Y-1)
			bottomR, bottomG, bottomB := rgb(scaled.At(x, bottomY))
			if _, err := fmt.Fprintf(
				output, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm▀",
				topR, topG, topB, bottomR, bottomG, bottomB,
			); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(output, "\x1b[0m\n"); err != nil {
			return err
		}
	}
	return nil
}

func renderASCII(output io.Writer, source image.Image, columns, rows int) error {
	const shades = " .:-=+*#%@"
	scaled := resize(source, columns, rows)
	bounds := scaled.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			red, green, blue := rgb(scaled.At(x, y))
			luminance := (299*red + 587*green + 114*blue) / 1000
			index := luminance * (len(shades) - 1) / 255
			if _, err := fmt.Fprintf(output, "%c", shades[index]); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(output, "\n"); err != nil {
			return err
		}
	}
	return nil
}

func resize(source image.Image, maximumWidth, maximumHeight int) image.Image {
	sourceBounds := source.Bounds()
	width, height := sourceBounds.Dx(), sourceBounds.Dy()
	scale := min(float64(maximumWidth)/float64(width), float64(maximumHeight)/float64(height))
	if scale > 1 {
		scale = 1
	}
	targetWidth := max(int(float64(width)*scale), 1)
	targetHeight := max(int(float64(height)*scale), 1)
	target := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	for y := range targetHeight {
		sourceY := sourceBounds.Min.Y + y*height/targetHeight
		for x := range targetWidth {
			sourceX := sourceBounds.Min.X + x*width/targetWidth
			target.Set(x, y, source.At(sourceX, sourceY))
		}
	}
	return target
}

func rgb(value color.Color) (int, int, int) {
	red, green, blue, alpha := value.RGBA()
	if alpha == 0 {
		return 0, 0, 0
	}
	return int(red >> 8), int(green >> 8), int(blue >> 8)
}
