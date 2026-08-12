package cdp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestStateOpensChatTabAfterLogin(t *testing.T) {
	var chatOpened atomic.Bool
	var targetActivated atomic.Bool
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	mux.HandleFunc("/json/list", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]Target{{
			ID:                   "line",
			Type:                 "page",
			Title:                "LINE",
			URL:                  "chrome-extension://" + DefaultExtensionID + "/index.html#/friends",
			WebSocketDebuggerURL: "ws://" + r.Host + "/devtools/page/line",
		}})
	})
	mux.HandleFunc("/devtools/page/line", func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()

		for {
			var request struct {
				ID     int    `json:"id"`
				Method string `json:"method"`
				Params struct {
					Expression string `json:"expression"`
				} `json:"params"`
			}
			if err := wsjson.Read(r.Context(), conn, &request); err != nil {
				return
			}

			value := ""
			switch {
			case request.Method == "Page.bringToFront":
				targetActivated.Store(true)
			case strings.Contains(request.Params.Expression, "button.click()"):
				chatOpened.Store(true)
				value = `{"found":true,"clicked":true}`
			case chatOpened.Load():
				value = `{"loggedIn":true,"chatSelected":true,"chatReady":true,` +
					`"rooms":[{"id":"room-1","name":"Alice"}],"messages":[],` +
					`"login":{"required":false}}`
			default:
				value = `{"loggedIn":true,"chatSelected":false,"chatReady":false,` +
					`"rooms":[],"messages":[],"login":{"required":false}}`
			}
			if err := wsjson.Write(r.Context(), conn, map[string]any{
				"id": request.ID,
				"result": map[string]any{
					"result": map[string]any{"type": "string", "value": value},
				},
			}); err != nil {
				return
			}
		}
	})

	bridge, err := NewBridge(server.URL, DefaultExtensionID, "")
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	state, err := bridge.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !chatOpened.Load() {
		t.Fatal("State did not click the LINE Chat tab")
	}
	if !targetActivated.Load() {
		t.Fatal("State did not bring the LINE target to the foreground")
	}
	if !state.ChatSelected || !state.ChatReady {
		t.Fatalf("chat state = selected:%v ready:%v", state.ChatSelected, state.ChatReady)
	}
	if len(state.Rooms) != 1 || state.Rooms[0].Name != "Alice" {
		t.Fatalf("rooms = %#v, want Alice", state.Rooms)
	}
}

func TestLogoutUsesLINEMoreActionsMenu(t *testing.T) {
	var evaluated atomic.Bool
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	mux.HandleFunc("/json/list", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]Target{{
			ID:                   "line",
			Type:                 "page",
			Title:                "LINE",
			URL:                  "chrome-extension://" + DefaultExtensionID + "/index.html#/chats/room-1",
			WebSocketDebuggerURL: "ws://" + r.Host + "/devtools/page/line",
		}})
	})
	mux.HandleFunc("/devtools/page/line", func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()

		for {
			var request struct {
				ID     int    `json:"id"`
				Method string `json:"method"`
				Params struct {
					Expression string `json:"expression"`
				} `json:"params"`
			}
			if err := wsjson.Read(r.Context(), conn, &request); err != nil {
				return
			}

			result := map[string]any{}
			if request.Method == "Runtime.evaluate" {
				if !strings.Contains(request.Params.Expression, `button[aria-label="Popover more actions"]`) ||
					!strings.Contains(request.Params.Expression, "logoutButton.click()") {
					t.Errorf("logout expression does not use the LINE logout menu:\n%s",
						request.Params.Expression)
				}
				evaluated.Store(true)
				result = map[string]any{
					"result": map[string]any{
						"type":  "string",
						"value": `{"ok":true}`,
					},
				}
			}
			if err := wsjson.Write(r.Context(), conn, map[string]any{
				"id":     request.ID,
				"result": result,
			}); err != nil {
				return
			}
		}
	})

	bridge, err := NewBridge(server.URL, DefaultExtensionID, "")
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := bridge.Logout(ctx); err != nil {
		t.Fatal(err)
	}
	if !evaluated.Load() {
		t.Fatal("Logout did not evaluate the LINE logout action")
	}
}
