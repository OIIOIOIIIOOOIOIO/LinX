package cdp

import (
	"context"
	"fmt"
	"sync"
)

type Bridge struct {
	discoverer  *Discoverer
	extensionID string
	selector    string

	mu       sync.Mutex
	client   *Client
	targetID string
}

func NewBridge(endpoint, extensionID, selector string) (*Bridge, error) {
	discoverer, err := NewDiscoverer(endpoint)
	if err != nil {
		return nil, err
	}
	if extensionID == "" {
		extensionID = DefaultExtensionID
	}
	return &Bridge{
		discoverer:  discoverer,
		extensionID: extensionID,
		selector:    selector,
	}, nil
}

func (b *Bridge) Targets(ctx context.Context) ([]Target, error) {
	return b.discoverer.Targets(ctx)
}

func (b *Bridge) Refresh(ctx context.Context) (Snapshot, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	target, client, err := b.connectLocked(ctx)
	if err != nil {
		return Snapshot{}, err
	}

	snapshot, err := Capture(ctx, client, target)
	if err != nil {
		b.closeLocked()
		return Snapshot{}, err
	}
	return snapshot, nil
}

func (b *Bridge) connectLocked(ctx context.Context) (Target, *Client, error) {
	targets, err := b.discoverer.Targets(ctx)
	if err != nil {
		b.closeLocked()
		return Target{}, nil, err
	}
	target, err := SelectTarget(targets, b.extensionID, b.selector)
	if err != nil {
		b.closeLocked()
		return Target{}, nil, fmt.Errorf("%w; open the LINE extension window first", err)
	}

	if b.client == nil || b.targetID != target.ID {
		b.closeLocked()
		b.client, err = Dial(ctx, target.WebSocketDebuggerURL)
		if err != nil {
			return Target{}, nil, err
		}
		b.targetID = target.ID
	}
	if target.Type == "page" {
		if err := b.client.Call(ctx, "Page.bringToFront", nil, nil); err != nil {
			b.closeLocked()
			return Target{}, nil, fmt.Errorf("activate LINE extension target: %w", err)
		}
	}
	return target, b.client, nil
}

func (b *Bridge) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.closeLocked()
}

func (b *Bridge) closeLocked() error {
	var err error
	if b.client != nil {
		err = b.client.Close()
	}
	b.client = nil
	b.targetID = ""
	return err
}
