package cdp

import (
	"errors"
	"testing"
)

func TestSelectTargetPrefersMainPage(t *testing.T) {
	targets := []Target{
		{
			ID:                   "index",
			Type:                 "page",
			URL:                  "chrome-extension://" + DefaultExtensionID + "/index.html#popout",
			WebSocketDebuggerURL: "ws://127.0.0.1:9222/devtools/page/index",
		},
		{
			ID:                   "sandbox",
			Type:                 "page",
			URL:                  "chrome-extension://" + DefaultExtensionID + "/ltsmSandbox.html?sandboxId=abc",
			WebSocketDebuggerURL: "ws://127.0.0.1:9222/devtools/page/sandbox",
		},
	}

	got, err := SelectTarget(targets, DefaultExtensionID, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "index" {
		t.Fatalf("selected %q, want index", got.ID)
	}
}

func TestSelectTargetUsesSelector(t *testing.T) {
	targets := []Target{
		{
			ID:                   "one",
			Type:                 "page",
			Title:                "First",
			URL:                  "chrome-extension://" + DefaultExtensionID + "/one.html",
			WebSocketDebuggerURL: "ws://127.0.0.1:9222/one",
		},
		{
			ID:                   "two",
			Type:                 "page",
			Title:                "Second",
			URL:                  "chrome-extension://" + DefaultExtensionID + "/two.html",
			WebSocketDebuggerURL: "ws://127.0.0.1:9222/two",
		},
	}

	got, err := SelectTarget(targets, DefaultExtensionID, "Second")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "two" {
		t.Fatalf("selected %q, want two", got.ID)
	}
}

func TestSelectTargetAcceptsExplicitExtensionSandboxIframe(t *testing.T) {
	targets := []Target{
		{
			ID:                   "index",
			Type:                 "page",
			URL:                  "chrome-extension://" + DefaultExtensionID + "/index.html#/chats/room",
			WebSocketDebuggerURL: "ws://127.0.0.1:9222/devtools/page/index",
		},
		{
			ID:                   "sandbox",
			Type:                 "iframe",
			URL:                  "chrome-extension://" + DefaultExtensionID + "/ltsmSandbox.html?sandboxId=abc",
			WebSocketDebuggerURL: "ws://127.0.0.1:9222/devtools/page/sandbox",
		},
	}

	got, err := SelectTarget(targets, DefaultExtensionID, "ltsmSandbox.html")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "sandbox" {
		t.Fatalf("selected %q, want sandbox iframe", got.ID)
	}
}

func TestSelectTargetRejectsOtherExtensions(t *testing.T) {
	_, err := SelectTarget([]Target{{
		ID:                   "other",
		Type:                 "page",
		URL:                  "chrome-extension://aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/index.html",
		WebSocketDebuggerURL: "ws://127.0.0.1:9222/other",
	}}, DefaultExtensionID, "")
	if !errors.Is(err, ErrTargetNotFound) {
		t.Fatalf("got %v, want ErrTargetNotFound", err)
	}
}

func TestNewDiscovererOnlyAcceptsLoopback(t *testing.T) {
	if _, err := NewDiscoverer("http://127.0.0.1:9222"); err != nil {
		t.Fatalf("loopback endpoint rejected: %v", err)
	}
	if _, err := NewDiscoverer("http://localhost:9222"); err != nil {
		t.Fatalf("localhost endpoint rejected: %v", err)
	}
	if _, err := NewDiscoverer("http://example.com:9222"); err == nil {
		t.Fatal("non-loopback endpoint accepted")
	}
}
