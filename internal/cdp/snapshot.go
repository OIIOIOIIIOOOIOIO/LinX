package cdp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Snapshot struct {
	Target    Target    `json:"target"`
	Lines     []string  `json:"lines"`
	Source    string    `json:"source"`
	Captured  time.Time `json:"captured"`
	NodeCount int       `json:"nodeCount"`
}

type AXValue struct {
	Type  string `json:"type"`
	Value any    `json:"value"`
}

type AXNode struct {
	NodeID  string  `json:"nodeId"`
	Ignored bool    `json:"ignored"`
	Role    AXValue `json:"role"`
	Name    AXValue `json:"name"`
	Value   AXValue `json:"value"`
}

type axTreeResult struct {
	Nodes []AXNode `json:"nodes"`
}

type runtimeResult struct {
	Result struct {
		Type        string `json:"type"`
		Value       any    `json:"value"`
		Description string `json:"description"`
	} `json:"result"`
}

type domCaptureResult struct {
	Mode  string   `json:"mode"`
	Lines []string `json:"lines"`
}

const domCaptureExpression = `(() => {
	const normalize = value => String(value || "").replace(/\s+/g, " ").trim();
	const messageList = document.querySelector(".message_list");
	const messageElements = messageList
		? Array.from(messageList.querySelectorAll("[data-message-content]"))
		: [];

	if (messageElements.length > 0) {
		const messages = messageElements.map((element, index) => {
			if (String(element.className).includes("messageDate-module__date")) {
				return null;
			}
			const timestamp = Number(element.dataset.timestamp || 0);
			const content = normalize(element.dataset.messageContent);
			const prefix = normalize(element.dataset.messageContentPrefix);
			const sender = prefix.replace(/^\d{1,2}:\d{2}\s+/, "").trim();
			return { timestamp, content, sender, index };
		}).filter(message => message && message.content);

		messages.sort((left, right) =>
			(left.timestamp || Number.MAX_SAFE_INTEGER) -
			(right.timestamp || Number.MAX_SAFE_INTEGER) ||
			left.index - right.index
		);

		const pad = value => String(value).padStart(2, "0");
		const formatTime = timestamp => {
			if (!timestamp) return "unknown time";
			const date = new Date(timestamp);
			return date.getFullYear() + "-" +
				pad(date.getMonth() + 1) + "-" +
				pad(date.getDate()) + " " +
				pad(date.getHours()) + ":" +
				pad(date.getMinutes());
		};
		const roomElement = document.querySelector('[class*="chatroomHeader-module__name"]');
		const room = normalize(roomElement ? roomElement.innerText : "");
		const lines = messages.map(message =>
			"[" + formatTime(message.timestamp) + "] " +
			(message.sender ? message.sender + ": " : "") +
			message.content
		);
		if (room) lines.unshift("# " + room);
		return JSON.stringify({ mode: "messages", lines });
	}

	const text = document.body ? document.body.innerText : "";
	return JSON.stringify({
		mode: "page",
		lines: text.split(/\r?\n/).map(normalize).filter(Boolean)
	});
})()`

func Capture(ctx context.Context, client *Client, target Target) (Snapshot, error) {
	var evaluated runtimeResult
	domErr := client.Call(ctx, "Runtime.evaluate", map[string]any{
		"expression":    domCaptureExpression,
		"returnByValue": true,
		"awaitPromise":  true,
	}, &evaluated)
	if domErr == nil {
		captured, err := decodeDOMCapture(evaluated.Result.Value)
		if err == nil && len(captured.Lines) > 0 {
			source := "dom"
			if captured.Mode == "messages" {
				source = "dom-messages"
			}
			return Snapshot{
				Target:   target,
				Lines:    captured.Lines,
				Source:   source,
				Captured: time.Now(),
			}, nil
		}
	}

	// Accessibility is a fallback for pages whose DOM does not expose text.
	// Enabling the domain is idempotent and keeps Capture usable after a
	// reconnect without maintaining extra state.
	if err := client.Call(ctx, "Accessibility.enable", map[string]any{}, nil); err != nil {
		if domErr != nil {
			return Snapshot{}, fmt.Errorf("capture DOM text: %v; enable accessibility: %w", domErr, err)
		}
		return Snapshot{}, err
	}

	var tree axTreeResult
	if err := client.Call(ctx, "Accessibility.getFullAXTree", map[string]any{}, &tree); err != nil {
		if domErr != nil {
			return Snapshot{}, fmt.Errorf("capture DOM text: %v; capture accessibility tree: %w", domErr, err)
		}
		return Snapshot{}, err
	}

	lines := ExtractText(tree.Nodes)
	if len(lines) == 0 {
		if domErr != nil {
			return Snapshot{}, fmt.Errorf("capture DOM text: %v; accessibility tree contained no text", domErr)
		}
		return Snapshot{}, fmt.Errorf("page contained no visible text")
	}
	return Snapshot{
		Target:    target,
		Lines:     lines,
		Source:    "accessibility",
		Captured:  time.Now(),
		NodeCount: len(tree.Nodes),
	}, nil
}

func decodeDOMCapture(value any) (domCaptureResult, error) {
	encoded, ok := value.(string)
	if !ok {
		return domCaptureResult{}, fmt.Errorf("DOM capture returned %T instead of string", value)
	}

	var captured domCaptureResult
	if err := json.Unmarshal([]byte(encoded), &captured); err != nil {
		return domCaptureResult{}, fmt.Errorf("decode DOM capture: %w", err)
	}
	for i, line := range captured.Lines {
		captured.Lines[i] = strings.TrimSpace(line)
	}
	return captured, nil
}

func ExtractText(nodes []AXNode) []string {
	var lines []string

	for _, node := range nodes {
		if node.Ignored {
			continue
		}
		role := valueString(node.Role.Value)
		if role == "InlineTextBox" {
			// StaticText parents contain the same value and are less fragmented.
			continue
		}
		if !textBearingRole(role) {
			continue
		}

		text := valueString(node.Name.Value)
		if text == "" {
			text = valueString(node.Value.Value)
		}
		for _, line := range splitVisibleText(text) {
			lines = append(lines, line)
		}
	}
	return lines
}

func textBearingRole(role string) bool {
	switch role {
	case "StaticText", "heading", "paragraph", "button", "link", "textbox", "listitem", "LabelText":
		return true
	default:
		return false
	}
}

func valueString(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	default:
		return ""
	}
}

func splitVisibleText(text string) []string {
	raw := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		line = strings.Join(strings.Fields(line), " ")
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
