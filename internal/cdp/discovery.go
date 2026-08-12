package cdp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const DefaultExtensionID = "ophjlpahpchlmihnnnihgmmeilfjmjjc"

var ErrTargetNotFound = errors.New("LINE extension target not found")

type Target struct {
	ID                   string `json:"id"`
	Type                 string `json:"type"`
	Title                string `json:"title"`
	URL                  string `json:"url"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

type Discoverer struct {
	endpoint string
	client   *http.Client
}

func NewDiscoverer(endpoint string) (*Discoverer, error) {
	u, err := validateLocalURL(endpoint, "http", "https")
	if err != nil {
		return nil, fmt.Errorf("invalid CDP endpoint: %w", err)
	}

	return &Discoverer{
		endpoint: strings.TrimRight(u.String(), "/"),
		client:   &http.Client{Timeout: 3 * time.Second},
	}, nil
}

func (d *Discoverer) Targets(ctx context.Context) ([]Target, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.endpoint+"/json/list", nil)
	if err != nil {
		return nil, fmt.Errorf("create target request: %w", err)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connect to Chrome at %s: %w", d.endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Chrome target endpoint returned %s", resp.Status)
	}

	var targets []Target
	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		return nil, fmt.Errorf("decode Chrome targets: %w", err)
	}
	return targets, nil
}

func SelectTarget(targets []Target, extensionID, selector string) (Target, error) {
	if extensionID == "" {
		extensionID = DefaultExtensionID
	}
	prefix := "chrome-extension://" + extensionID + "/"

	var candidates []Target
	for _, target := range targets {
		if (target.Type != "page" && target.Type != "iframe") || target.WebSocketDebuggerURL == "" {
			continue
		}
		if !strings.HasPrefix(target.URL, prefix) {
			continue
		}
		if selector != "" && !strings.Contains(target.URL, selector) && !strings.Contains(target.Title, selector) {
			continue
		}
		candidates = append(candidates, target)
	}
	if len(candidates) == 0 {
		return Target{}, ErrTargetNotFound
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		return targetPriority(candidates[i]) < targetPriority(candidates[j])
	})
	return candidates[0], nil
}

func targetPriority(target Target) int {
	switch {
	case strings.Contains(target.URL, "/index.html"):
		return 0
	case strings.Contains(target.URL, "/ltsmSandbox.html"):
		return 1
	default:
		return 2
	}
}

func validateLocalURL(raw string, schemes ...string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if u.User != nil {
		return nil, errors.New("credentials are not allowed in debugger URLs")
	}

	validScheme := false
	for _, scheme := range schemes {
		if u.Scheme == scheme {
			validScheme = true
			break
		}
	}
	if !validScheme {
		return nil, fmt.Errorf("scheme %q is not allowed", u.Scheme)
	}
	if u.Hostname() == "" {
		return nil, errors.New("missing hostname")
	}
	if !isLoopbackHost(u.Hostname()) {
		return nil, fmt.Errorf("refusing non-loopback debugger host %q", u.Hostname())
	}
	return u, nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
