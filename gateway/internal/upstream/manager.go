package upstream

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"ai-coding-interview/gateway/internal/config"
)

type Endpoint struct {
	URL     *url.URL
	healthy atomic.Bool
}

func (e *Endpoint) Healthy() bool {
	return e.healthy.Load()
}

type Group struct {
	id        string
	endpoints []*Endpoint
	health    config.HealthConfig
	next      atomic.Uint64
	client    *http.Client
}

type Manager struct {
	groups map[string]*Group
}

type GroupStatus struct {
	ID      string `json:"id"`
	Healthy int    `json:"healthy"`
	Total   int    `json:"total"`
}

func New(configs []config.UpstreamConfig, transport http.RoundTripper) (*Manager, error) {
	groups := make(map[string]*Group, len(configs))
	for _, upstreamConfig := range configs {
		group := &Group{
			id:     upstreamConfig.ID,
			health: upstreamConfig.Health,
			client: &http.Client{
				Transport: transport,
				Timeout:   upstreamConfig.Health.Timeout,
				CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
					return http.ErrUseLastResponse
				},
			},
		}
		for _, raw := range upstreamConfig.Endpoints {
			parsed, err := url.Parse(raw)
			if err != nil {
				return nil, fmt.Errorf("parse endpoint %q: %w", raw, err)
			}
			parsed.Path = ""
			parsed.RawPath = ""
			group.endpoints = append(group.endpoints, &Endpoint{URL: parsed})
		}
		groups[group.id] = group
	}
	return &Manager{groups: groups}, nil
}

func (m *Manager) Start(ctx context.Context) {
	var wait sync.WaitGroup
	for _, group := range m.groups {
		wait.Add(1)
		go func(group *Group) {
			defer wait.Done()
			group.check(ctx)
		}(group)
	}
	wait.Wait()

	for _, group := range m.groups {
		go group.run(ctx)
	}
}

func (m *Manager) Pick(id string) (*Endpoint, bool) {
	group, exists := m.groups[id]
	if !exists {
		return nil, false
	}
	healthy := group.healthyEndpoints()
	if len(healthy) == 0 {
		return nil, false
	}
	index := group.next.Add(1) - 1
	return healthy[index%uint64(len(healthy))], true
}

func (m *Manager) MarkUnhealthy(id string, endpoint *Endpoint) {
	group, exists := m.groups[id]
	if !exists {
		return
	}
	for _, candidate := range group.endpoints {
		if candidate == endpoint {
			candidate.healthy.Store(false)
			return
		}
	}
}

func (m *Manager) Ready(ids []string) bool {
	for _, id := range ids {
		group, exists := m.groups[id]
		if !exists || len(group.healthyEndpoints()) == 0 {
			return false
		}
	}
	return true
}

func (m *Manager) Status(ids []string) []GroupStatus {
	statuses := make([]GroupStatus, 0, len(ids))
	for _, id := range ids {
		group, exists := m.groups[id]
		if !exists {
			statuses = append(statuses, GroupStatus{ID: id})
			continue
		}
		statuses = append(statuses, GroupStatus{
			ID:      id,
			Healthy: len(group.healthyEndpoints()),
			Total:   len(group.endpoints),
		})
	}
	return statuses
}

func (g *Group) run(ctx context.Context) {
	ticker := time.NewTicker(g.health.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			g.check(ctx)
		}
	}
}

func (g *Group) check(ctx context.Context) {
	var wait sync.WaitGroup
	for _, endpoint := range g.endpoints {
		wait.Add(1)
		go func(endpoint *Endpoint) {
			defer wait.Done()
			endpoint.healthy.Store(g.probe(ctx, endpoint))
		}(endpoint)
	}
	wait.Wait()
}

func (g *Group) probe(ctx context.Context, endpoint *Endpoint) bool {
	healthURL := *endpoint.URL
	healthURL.Path = g.health.Path
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL.String(), nil)
	if err != nil {
		return false
	}
	response, err := g.client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	return response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices
}

func (g *Group) healthyEndpoints() []*Endpoint {
	healthy := make([]*Endpoint, 0, len(g.endpoints))
	for _, endpoint := range g.endpoints {
		if endpoint.Healthy() {
			healthy = append(healthy, endpoint)
		}
	}
	return healthy
}
