package ui

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"linx/internal/cdp"
)

func TestRenderSnapshot(t *testing.T) {
	var out bytes.Buffer
	dashboard := Dashboard{Out: &out, ANSI: false, MaxLines: 10}
	dashboard.render(viewState{
		Snapshot: cdp.Snapshot{
			Target: cdp.Target{
				Title: "LINE",
				URL:   "chrome-extension://" + cdp.DefaultExtensionID + "/ltsmSandbox.html",
			},
			Lines:  []string{"Alice", "hello"},
			Source: "accessibility",
		},
		Updated: time.Date(2026, 7, 20, 12, 30, 0, 0, time.Local),
	})

	rendered := out.String()
	for _, want := range []string{"status  connected", "target  LINE", "Alice", "hello"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered output does not contain %q:\n%s", want, rendered)
		}
	}
}

func TestRenderError(t *testing.T) {
	var out bytes.Buffer
	dashboard := Dashboard{Out: &out, ANSI: false, MaxLines: 10}
	dashboard.render(viewState{Err: errors.New("Chrome unavailable")})
	if !strings.Contains(out.String(), "status  waiting: Chrome unavailable") {
		t.Fatalf("unexpected output:\n%s", out.String())
	}
}
