package cdp

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestLiveLINXDOMDiagnostics(t *testing.T) {
	if os.Getenv("LINX_LIVE_DIAGNOSTICS") == "" {
		t.Skip("set LINX_LIVE_DIAGNOSTICS=1 to inspect the local LINE target")
	}
	bridge, err := NewBridge("http://127.0.0.1:9222", DefaultExtensionID, "")
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	_, client, err := bridge.connectLocked(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("LINX_LIVE_VIEWPORT") != "" {
		if err := client.Call(ctx, "Page.bringToFront", nil, nil); err != nil {
			t.Fatal(err)
		}
		if err := client.Call(ctx, "Emulation.setDeviceMetricsOverride", map[string]any{
			"width":             1280,
			"height":            10000,
			"deviceScaleFactor": 1,
			"mobile":            false,
		}, nil); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Second)
	} else if os.Getenv("LINX_LIVE_CLEAR_VIEWPORT") != "" {
		if err := client.Call(ctx, "Emulation.setDeviceMetricsOverride", map[string]any{
			"width":             800,
			"height":            513,
			"deviceScaleFactor": 1,
			"mobile":            false,
		}, nil); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Second)
	}

	const expression = `(() => {
		const metrics = element => element ? {
			tag: element.tagName,
			className: String(element.className),
			clientHeight: element.clientHeight,
			scrollHeight: element.scrollHeight,
			scrollTop: element.scrollTop,
			childCount: element.children.length
		} : null;
		const classMetrics = pattern => Array.from(
			document.querySelectorAll('[class*="' + pattern + '"]')
		).map(metrics);
		const chatroom = document.querySelector('[class*="chatroom-module__chatroom"]');
		return JSON.stringify({
			url: location.href,
			visibility: document.visibilityState,
			viewport: {width: innerWidth, height: innerHeight},
			roomRows: document.querySelectorAll('[data-mid]').length,
			roomButtons: document.querySelectorAll(
				'[class*="chatlistItem-module__button_chatlist_item"]'
			).length,
			goChatroomButtons: document.querySelectorAll(
				'button[aria-label="Go chatroom"]'
			).length,
			chatlist: classMetrics("chatlist-module__"),
			scrollable: Array.from(document.querySelectorAll("*"))
				.filter(element => element.scrollHeight > element.clientHeight + 1)
				.map(metrics),
			roomAncestors: (() => {
				const button = document.querySelector(
					'[class*="chatlistItem-module__button_chatlist_item"]'
				);
				const result = [];
				for (let element = button; element && result.length < 8; element = element.parentElement) {
					result.push(metrics(element));
				}
				return result;
			})(),
			messageList: metrics(document.querySelector(".message_list")),
			messageIDs: document.querySelectorAll("[data-message-id]").length,
			messageContents: document.querySelectorAll("[data-message-content]").length,
			chatroom: metrics(chatroom),
			chatroomText: chatroom ? chatroom.innerText.slice(0, 500) : "",
			navigation: Array.from(document.querySelectorAll("button[aria-label]"))
				.slice(0, 20)
				.map(button => ({
					label: button.getAttribute("aria-label"),
					current: button.getAttribute("aria-current")
				}))
		});
	})()`
	var diagnostics any
	if err := evaluateJSON(ctx, client, expression, &diagnostics); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.MarshalIndent(diagnostics, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(encoded))
}

func TestLiveOpenVirtualizedRoom(t *testing.T) {
	if os.Getenv("LINX_LIVE_OPEN_ROOM") == "" {
		t.Skip("set LINX_LIVE_OPEN_ROOM=1 to open an off-screen local LINE room")
	}
	bridge, err := NewBridge("http://127.0.0.1:9222", DefaultExtensionID, "")
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	state, err := bridge.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Rooms) < 2 {
		t.Fatalf("need at least two rooms, got %d", len(state.Rooms))
	}
	roomIndex := len(state.Rooms) - 2
	if os.Getenv("LINX_LIVE_OPEN_ROOM") == "first" {
		roomIndex = 0
	}
	room := state.Rooms[roomIndex]
	if err := bridge.OpenRoom(ctx, room.ID); err != nil {
		t.Fatal(err)
	}
	state, err = bridge.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.ActiveRoom != room.ID {
		t.Fatalf("active room = %q, want %q", state.ActiveRoom, room.ID)
	}
	t.Logf("opened %q from %d rooms; loaded %d messages",
		room.Name, len(state.Rooms), len(state.Messages))
}
