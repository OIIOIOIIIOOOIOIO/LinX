package ui

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"

	"linx/internal/cdp"
)

type logoutTestController struct {
	app         cdp.AppState
	logoutCalls int
}

func (c *logoutTestController) State(context.Context) (cdp.AppState, error) {
	return c.app, nil
}

func (c *logoutTestController) OpenRoom(context.Context, string) error {
	return nil
}

func (c *logoutTestController) SendMessage(context.Context, string) error {
	return nil
}

func (c *logoutTestController) RefreshQR(context.Context) error {
	return nil
}

func (c *logoutTestController) Logout(context.Context) error {
	c.logoutCalls++
	c.app = cdp.AppState{Login: cdp.LoginState{Required: true}}
	return nil
}

func (c *logoutTestController) QRCodePNG(context.Context) ([]byte, error) {
	return nil, nil
}

func TestRenderQRCodeDetectsModuleGrid(t *testing.T) {
	const modules = 57
	const scale = 2
	img := image.NewRGBA(image.Rect(0, 0, modules*scale+12, modules*scale+12))
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			img.Set(x, y, color.White)
		}
	}
	drawFinder := func(originX, originY int) {
		for y := 0; y < 7; y++ {
			for x := 0; x < 7; x++ {
				dark := x == 0 || x == 6 || y == 0 || y == 6 ||
					(x >= 2 && x <= 4 && y >= 2 && y <= 4)
				if !dark {
					continue
				}
				for pixelY := 0; pixelY < scale; pixelY++ {
					for pixelX := 0; pixelX < scale; pixelX++ {
						img.Set(6+(originX+x)*scale+pixelX, 6+(originY+y)*scale+pixelY, color.Black)
					}
				}
			}
		}
	}
	drawFinder(0, 0)
	drawFinder(modules-7, 0)
	drawFinder(0, modules-7)

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	lines, err := RenderQRCode(encoded.Bytes(), 80)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 33 {
		t.Fatalf("rendered %d rows, want 33", len(lines))
	}
	if got := visibleWidth(lines[0]); got != 66 {
		t.Fatalf("rendered width %d, want 66", got)
	}
}

func TestRenderQRCodeRejectsNarrowTerminal(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 42, 42))
	for y := 0; y < 42; y++ {
		for x := 0; x < 42; x++ {
			img.Set(x, y, color.White)
		}
	}
	draw := func(originX, originY int) {
		for y := 0; y < 7; y++ {
			for x := 0; x < 7; x++ {
				dark := x == 0 || x == 6 || y == 0 || y == 6 ||
					(x >= 2 && x <= 4 && y >= 2 && y <= 4)
				if dark {
					img.Set((originX+x)*2, (originY+y)*2, color.Black)
					img.Set((originX+x)*2+1, (originY+y)*2, color.Black)
					img.Set((originX+x)*2, (originY+y)*2+1, color.Black)
					img.Set((originX+x)*2+1, (originY+y)*2+1, color.Black)
				}
			}
		}
	}
	draw(0, 0)
	draw(14, 0)
	draw(0, 14)

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	_, err := RenderQRCode(encoded.Bytes(), 20)
	if err == nil || !strings.Contains(err.Error(), "too narrow") {
		t.Fatalf("got %v, want terminal width error", err)
	}
}

func TestRawTerminalTextUsesCRLF(t *testing.T) {
	got := rawTerminalText("first\nsecond\r\nthird")
	want := "first\r\nsecond\r\nthird"
	if got != want {
		t.Fatalf("rawTerminalText() = %q, want %q", got, want)
	}
}

func TestFitUsesTerminalCellWidth(t *testing.T) {
	tests := []struct {
		name  string
		value string
		width int
	}{
		{name: "Thai combining marks", value: "นุ่มนิ่ม", width: 12},
		{name: "CJK wide characters", value: "日本語", width: 12},
		{name: "emoji grapheme", value: "ครอบครัว 👨‍👩‍👧‍👦", width: 16},
		{name: "Thai truncation", value: "OTOP สมุนไพรด่านเกวียน", width: 14},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := fit(test.value, test.width)
			if width := visibleWidth(got); width != test.width {
				t.Fatalf("visibleWidth(fit(%q, %d)) = %d, want %d; output %q",
					test.value, test.width, width, test.width, got)
			}
		})
	}
}

func TestThaiTerminalWidths(t *testing.T) {
	tests := map[string]int{
		"นุ่มนิ่ม":                      4,
		"OTOP สมุนไพรด่านเกวียน (7)":    23,
		"BC67221 A ลูก อ.นุ่มนิ่ม (61)": 24,
		"ผญ.ไพโรจน์ รังพงษ์":            15,
	}
	for value, want := range tests {
		if got := runewidth.StringWidth(value); got != want {
			t.Errorf("StringWidth(%q) = %d, want %d", value, got, want)
		}
	}
}

func TestFitKeepsColumnSeparatorAligned(t *testing.T) {
	const columnWidth = 30
	values := []string{
		"LINE BK Alerts",
		"OTOP สมุนไพรด่านเกวียน (7)",
		"BC67221 A ลูก อ.นุ่มนิ่ม (61)",
		"日本語のルーム",
		"Family 👨‍👩‍👧‍👦",
	}
	for _, value := range values {
		left, _, ok := strings.Cut(fit(value, columnWidth)+" │ Messages", "│")
		if !ok {
			t.Fatalf("separator missing for %q", value)
		}
		if width := visibleWidth(left); width != columnWidth+1 {
			t.Fatalf("separator for %q starts at cell %d, want %d",
				value, width, columnWidth+1)
		}
	}
}

func TestWriteColumnsUsesAbsoluteDividerPosition(t *testing.T) {
	var out strings.Builder
	writeColumns(&out, "BC67221 A ลูก อ.นุ่มนิ่ม (61)", 30, "ข้อความภาษาไทย", 40)
	if !strings.Contains(out.String(), "\x1b[31G │ ") {
		t.Fatalf("column output does not position divider absolutely: %q", out.String())
	}
	if !strings.HasSuffix(out.String(), "\x1b[K") {
		t.Fatalf("column output does not erase stale cells: %q", out.String())
	}
}

func TestRenderExplainsEmptyRoomsWhileOpeningChat(t *testing.T) {
	output, err := os.CreateTemp(t.TempDir(), "linx-output")
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()

	ControlUI{Out: output}.render(controlState{
		app: cdp.AppState{
			LoggedIn:     true,
			ChatSelected: false,
			ChatReady:    false,
		},
		status: "opening LINE Chat tab…",
	})
	if _, err := output.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	rendered, err := os.ReadFile(output.Name())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rendered), "Opening Chat tab") {
		t.Fatalf("rendered output did not explain the empty room list: %q", rendered)
	}
}

func TestLogoutRequiresConfirmation(t *testing.T) {
	controller := &logoutTestController{
		app: cdp.AppState{LoggedIn: true},
	}
	ui := ControlUI{Controller: controller}
	state := controlState{app: controller.app}

	if quit := ui.handleKey(context.Background(), &state, keyEvent{kind: keyRune, r: 'l'}); quit {
		t.Fatal("logout prompt unexpectedly quit the UI")
	}
	if state.mode != modeConfirmLogout {
		t.Fatalf("mode = %v, want modeConfirmLogout", state.mode)
	}
	if controller.logoutCalls != 0 {
		t.Fatalf("Logout called %d times before confirmation", controller.logoutCalls)
	}

	ui.handleKey(context.Background(), &state, keyEvent{kind: keyRune, r: 'n'})
	if state.mode != modeNormal || controller.logoutCalls != 0 {
		t.Fatalf("cancel left mode %v with %d logout calls", state.mode, controller.logoutCalls)
	}

	ui.handleKey(context.Background(), &state, keyEvent{kind: keyRune, r: 'l'})
	ui.handleKey(context.Background(), &state, keyEvent{kind: keyRune, r: 'y'})
	if controller.logoutCalls != 1 {
		t.Fatalf("Logout called %d times after confirmation, want 1", controller.logoutCalls)
	}
	if state.mode != modeNormal || !state.app.Login.Required {
		t.Fatalf("state after logout = mode %v, app %#v", state.mode, state.app)
	}
}
