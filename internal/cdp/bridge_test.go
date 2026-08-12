package cdp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestBridgeRefresh(t *testing.T) {
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	mux.HandleFunc("/json/list", func(w http.ResponseWriter, r *http.Request) {
		wsURL := "ws://" + r.Host + "/devtools/page/line"
		_ = json.NewEncoder(w).Encode([]Target{{
			ID:                   "line",
			Type:                 "page",
			Title:                "LINE",
			URL:                  "chrome-extension://" + DefaultExtensionID + "/ltsmSandbox.html?sandboxId=test",
			WebSocketDebuggerURL: wsURL,
		}})
	})
	mux.HandleFunc("/devtools/page/line", serveFakeCDP)

	bridge, err := NewBridge(server.URL, DefaultExtensionID, "")
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	snapshot, err := bridge.Refresh(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if snapshot.Source != "accessibility" {
		t.Fatalf("source = %q, want accessibility", snapshot.Source)
	}
	if got := strings.Join(snapshot.Lines, "|"); got != "Alice|สวัสดีจาก LINE" {
		t.Fatalf("lines = %q", got)
	}
}

func serveFakeCDP(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()

	for {
		var request struct {
			ID     int             `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := wsjson.Read(r.Context(), conn, &request); err != nil {
			return
		}

		result := any(map[string]any{})
		if request.Method == "Accessibility.getFullAXTree" {
			result = map[string]any{
				"nodes": []map[string]any{
					{"nodeId": "1", "role": map[string]any{"value": "StaticText"}, "name": map[string]any{"value": "Alice"}},
					{"nodeId": "2", "role": map[string]any{"value": "StaticText"}, "name": map[string]any{"value": "สวัสดีจาก LINE"}},
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
}
