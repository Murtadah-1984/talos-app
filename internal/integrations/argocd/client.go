// Package argocd implements the ports.ArgoCDClient adapter (ADR-0003). This
// file is the Phase 4 real implementation: a plain REST client against Argo
// CD's documented HTTP API (https://<server>/api/v1/...), authenticated with
// a bearer token. A hand-rolled client is used deliberately instead of Argo
// CD's own Go SDK, which pulls in most of k8s client-go and Argo CD's own
// server packages — far more than a read-mostly status/sync client needs.
// See mock.go for the deterministic stand-in used elsewhere in local
// development.
package argocd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/talos-platform/talos-platform/internal/application/ports"
	"github.com/talos-platform/talos-platform/internal/domain/gitops"
	"github.com/talos-platform/talos-platform/internal/domain/shared"
)

// Client is the real ports.ArgoCDClient adapter.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// NewClient builds a Client against an Argo CD server's base URL (e.g.
// "https://argocd.example.com"), authenticated with token — a static API
// token created via `argocd account generate-token`, or a session JWT.
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: 30 * time.Second, Transport: otelhttp.NewTransport(http.DefaultTransport)},
	}
}

func (c *Client) do(ctx context.Context, method, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("calling Argo CD API %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return shared.ErrNotFound
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("argo cd api %s %s returned %d", method, path, resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// argoApplication mirrors the subset of Argo CD's Application resource this
// client actually reads. Argo CD's real type has many more fields.
type argoApplication struct {
	Metadata struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Spec struct {
		Project string `json:"project"`
	} `json:"spec"`
	Status struct {
		Sync struct {
			Status   string `json:"status"`
			Revision string `json:"revision"`
		} `json:"sync"`
		Health struct {
			Status string `json:"status"`
		} `json:"health"`
		Resources []struct {
			Kind      string `json:"kind"`
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
			Health    struct {
				Status string `json:"status"`
			} `json:"health"`
		} `json:"resources"`
		History []struct {
			Revision   string `json:"revision"`
			DeployedAt string `json:"deployedAt"`
		} `json:"history"`
	} `json:"status"`
}

type argoApplicationList struct {
	Items []argoApplication `json:"items"`
}

func toApplicationStatus(a argoApplication) ports.ApplicationStatus {
	tree := make([]ports.ResourceNode, 0, len(a.Status.Resources))
	for _, r := range a.Status.Resources {
		tree = append(tree, ports.ResourceNode{
			Kind:      r.Kind,
			Name:      r.Name,
			Namespace: r.Namespace,
			Health:    gitops.ArgoCDHealthStatus(r.Health.Status),
		})
	}
	return ports.ApplicationStatus{
		Name:         a.Metadata.Name,
		Namespace:    a.Metadata.Namespace,
		Project:      a.Spec.Project,
		SyncStatus:   gitops.ArgoCDSyncStatus(a.Status.Sync.Status),
		HealthStatus: gitops.ArgoCDHealthStatus(a.Status.Health.Status),
		Revision:     a.Status.Sync.Revision,
		ResourceTree: tree,
	}
}

func (c *Client) ListApplications(ctx context.Context, project string) ([]ports.ApplicationStatus, error) {
	path := "/api/v1/applications"
	if project != "" {
		path += "?project=" + project
	}
	var list argoApplicationList
	if err := c.do(ctx, http.MethodGet, path, &list); err != nil {
		return nil, fmt.Errorf("listing Argo CD applications: %w", err)
	}
	out := make([]ports.ApplicationStatus, 0, len(list.Items))
	for _, item := range list.Items {
		out = append(out, toApplicationStatus(item))
	}
	return out, nil
}

func (c *Client) GetApplication(ctx context.Context, name string) (ports.ApplicationStatus, error) {
	var app argoApplication
	if err := c.do(ctx, http.MethodGet, "/api/v1/applications/"+name, &app); err != nil {
		return ports.ApplicationStatus{}, fmt.Errorf("getting Argo CD application %s: %w", name, err)
	}
	return toApplicationStatus(app), nil
}

func (c *Client) Sync(ctx context.Context, name string) error {
	if err := c.do(ctx, http.MethodPost, "/api/v1/applications/"+name+"/sync", nil); err != nil {
		return fmt.Errorf("syncing Argo CD application %s: %w", name, err)
	}
	return nil
}

func (c *Client) GetHistory(ctx context.Context, name string) ([]ports.SyncHistoryEntry, error) {
	var app argoApplication
	if err := c.do(ctx, http.MethodGet, "/api/v1/applications/"+name, &app); err != nil {
		return nil, fmt.Errorf("getting history for Argo CD application %s: %w", name, err)
	}
	out := make([]ports.SyncHistoryEntry, 0, len(app.Status.History))
	for _, h := range app.Status.History {
		out = append(out, ports.SyncHistoryEntry{Revision: h.Revision, DeployedAt: h.DeployedAt, Status: app.Status.Sync.Status})
	}
	return out, nil
}

// argoApplicationSet mirrors the subset of Argo CD's ApplicationSet resource
// this client reads.
type argoApplicationSet struct {
	Metadata struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Status struct {
		Resources []struct {
			Name string `json:"name"`
		} `json:"resources"`
		Conditions []struct {
			Type    string `json:"type"`
			Status  string `json:"status"`
			Message string `json:"message"`
		} `json:"conditions"`
	} `json:"status"`
}

type argoApplicationSetList struct {
	Items []argoApplicationSet `json:"items"`
}

func toApplicationSetStatus(a argoApplicationSet) ports.ApplicationSetStatus {
	resources := make([]string, 0, len(a.Status.Resources))
	for _, r := range a.Status.Resources {
		resources = append(resources, r.Name)
	}
	conditions := make([]ports.ApplicationSetCondition, 0, len(a.Status.Conditions))
	for _, cond := range a.Status.Conditions {
		conditions = append(conditions, ports.ApplicationSetCondition{Type: cond.Type, Status: cond.Status, Message: cond.Message})
	}
	return ports.ApplicationSetStatus{
		Name:       a.Metadata.Name,
		Namespace:  a.Metadata.Namespace,
		Resources:  resources,
		Conditions: conditions,
	}
}

func (c *Client) ListApplicationSets(ctx context.Context, project string) ([]ports.ApplicationSetStatus, error) {
	path := "/api/v1/applicationsets"
	if project != "" {
		path += "?projects=" + project
	}
	var list argoApplicationSetList
	if err := c.do(ctx, http.MethodGet, path, &list); err != nil {
		return nil, fmt.Errorf("listing Argo CD application sets: %w", err)
	}
	out := make([]ports.ApplicationSetStatus, 0, len(list.Items))
	for _, item := range list.Items {
		out = append(out, toApplicationSetStatus(item))
	}
	return out, nil
}

func (c *Client) Capability() shared.CapabilityState {
	return shared.CapabilityAvailable
}

var _ ports.ArgoCDClient = (*Client)(nil)
