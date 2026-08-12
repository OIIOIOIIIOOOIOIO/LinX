package cdp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

type protocolError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type envelope struct {
	ID     int             `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *protocolError  `json:"error,omitempty"`
}

type Client struct {
	conn   *websocket.Conn
	mu     sync.Mutex
	nextID int
}

func Dial(ctx context.Context, websocketURL string) (*Client, error) {
	if _, err := validateLocalURL(websocketURL, "ws", "wss", "http", "https"); err != nil {
		return nil, fmt.Errorf("invalid target WebSocket URL: %w", err)
	}

	conn, resp, err := websocket.Dial(ctx, websocketURL, nil)
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("connect to target WebSocket (%s): %w", resp.Status, err)
		}
		return nil, fmt.Errorf("connect to target WebSocket: %w", err)
	}
	conn.SetReadLimit(32 << 20)
	return &Client{conn: conn}, nil
}

func (c *Client) Close() error {
	return c.conn.Close(websocket.StatusNormalClosure, "linx closed")
}

func (c *Client) Call(ctx context.Context, method string, params any, result any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.nextID++
	id := c.nextID
	request := struct {
		ID     int    `json:"id"`
		Method string `json:"method"`
		Params any    `json:"params,omitempty"`
	}{
		ID:     id,
		Method: method,
		Params: params,
	}

	if err := wsjson.Write(ctx, c.conn, request); err != nil {
		return fmt.Errorf("%s write request: %w", method, err)
	}

	for {
		var response envelope
		if err := wsjson.Read(ctx, c.conn, &response); err != nil {
			return fmt.Errorf("%s read response: %w", method, err)
		}
		if response.ID != id {
			// CDP events and responses from unrelated sessions are intentionally
			// ignored. This client issues one command at a time.
			continue
		}
		if response.Error != nil {
			return fmt.Errorf("%s failed (%d): %s", method, response.Error.Code, response.Error.Message)
		}
		if result == nil || len(response.Result) == 0 {
			return nil
		}
		if err := json.Unmarshal(response.Result, result); err != nil {
			return fmt.Errorf("%s decode response: %w", method, err)
		}
		return nil
	}
}
