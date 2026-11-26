package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"detect-radar/internal/model"
)

type Adapter interface {
	Name() string

	Tier() string

	Lookup(ctx context.Context, ip string) (*model.IPIntel, error)
}

type Registry struct {
	local   Adapter
	remotes []Adapter
}

func NewRegistry() *Registry {
	return &Registry{
		remotes: make([]Adapter, 0),
	}
}

func (r *Registry) SetLocal(a Adapter) {
	r.local = a
}

func (r *Registry) AddRemote(a Adapter) {
	r.remotes = append(r.remotes, a)
}

func (r *Registry) Local() Adapter {
	return r.local
}

func (r *Registry) Remotes() []Adapter {
	return r.remotes
}

func (r *Registry) HasLocal() bool {
	return r.local != nil
}

func (r *Registry) RemoteCount() int {
	return len(r.remotes)
}

type RemoteBase struct {
	name       string
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func NewRemoteBase(name, baseURL, apiKey string) RemoteBase {
	return RemoteBase{
		name:    name,
		apiKey:  apiKey,
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (b *RemoteBase) Name() string { return b.name }

func (b *RemoteBase) Tier() string { return "L3" }

func (b *RemoteBase) DoGet(ctx context.Context, url string, headers map[string]string, result interface{}) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("create request failed: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "DetectRadar/1.0")

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("decode response failed: %w", err)
	}

	return nil
}

func (b *RemoteBase) APIKey() string {
	return b.apiKey
}

func (b *RemoteBase) BaseURL() string {
	return b.baseURL
}
