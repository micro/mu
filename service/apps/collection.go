package apps

import (
	"context"
	"fmt"
	"mu/internal/service"
	"sort"
	"time"
)

type CollectionRequest struct{}
type SavedApp struct {
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Source      string    `json:"source,omitempty"`
	Updated     time.Time `json:"updated"`
}
type CollectionResponse struct {
	Items []SavedApp `json:"items"`
}

func (Server) Collection(ctx context.Context, _ *CollectionRequest, rsp *CollectionResponse) error {
	owner := service.AccountFrom(ctx)
	if owner == "" {
		return fmt.Errorf("authentication required")
	}
	origins := map[string]string{}
	buildMu.Lock()
	for _, job := range buildJobs {
		if job.Account == owner && job.App != nil {
			origins[job.App.Slug] = job.Thread
		}
	}
	buildMu.Unlock()
	mutex.RLock()
	for _, a := range apps {
		if a.AuthorID != owner {
			continue
		}
		source := a.Source
		if source == "" {
			source = origins[a.Slug]
		}
		rsp.Items = append(rsp.Items, SavedApp{Slug: a.Slug, Name: a.Name, Description: a.Description, Source: source, Updated: a.UpdatedAt})
	}
	mutex.RUnlock()
	sort.Slice(rsp.Items, func(i, j int) bool { return rsp.Items[i].Updated.After(rsp.Items[j].Updated) })
	return nil
}

type BuildsResponse struct {
	Items []BuildStatusResponse `json:"items"`
}

func (Server) Builds(ctx context.Context, _ *CollectionRequest, rsp *BuildsResponse) error {
	owner := service.AccountFrom(ctx)
	buildMu.Lock()
	defer buildMu.Unlock()
	for _, j := range buildJobs {
		if owner != "" && j.Account == owner {
			rsp.Items = append(rsp.Items, buildStatus(j))
		}
	}
	sort.Slice(rsp.Items, func(i, j int) bool { return rsp.Items[i].Created.After(rsp.Items[j].Created) })
	return nil
}
