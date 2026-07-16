package opensearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"

	opensearchgo "github.com/opensearch-project/opensearch-go/v2"

	"osreport/internal/domain"
)

// Repository implements domain.EventRepository against a real OpenSearch
// cluster. The domain and app layers never import this package directly —
// only the domain.EventRepository interface.
type Repository struct {
	client   *opensearchgo.Client
	pageSize int
}

const (
	defaultPageSize = 1000
	// maxPageSize matches OpenSearch's default index.max_result_window.
	// search_after bypasses the from+size deep-pagination limit, but size
	// itself is still bounds-checked against max_result_window — asking
	// for more per page than that fails the request outright.
	maxPageSize = 10000
)

// NewRepository builds a Repository. pageSize <= 0 falls back to
// defaultPageSize; anything above maxPageSize is clamped rather than left
// to fail on the first request with an opaque 400 from OpenSearch.
func NewRepository(client *opensearchgo.Client, pageSize int) *Repository {
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		slog.Warn("page-size exceeds OpenSearch max_result_window, clamping", "requested", pageSize, "clamped_to", maxPageSize)
		pageSize = maxPageSize
	}
	return &Repository{client: client, pageSize: pageSize}
}

func (r *Repository) Search(ctx context.Context, c domain.Criteria) ([]domain.Event, int, error) {
	hits, err := paginateSearchAfter(r.pageSize, func(searchAfter []interface{}) (searchResponse, error) {
		return r.fetchPage(ctx, c, searchAfter)
	})
	if err != nil {
		return nil, 0, err
	}

	events := make([]domain.Event, 0, len(hits))
	skipped := 0
	for _, h := range hits {
		e, err := MapHit(h.ID, h.Source)
		if err != nil {
			// A single malformed document shouldn't sink the whole report
			// run — log it and keep going. If this fires a lot, that's a
			// signal to loosen/adjust the mapper, not to make it panic.
			slog.Warn("skipping unmappable hit", "hit_id", h.ID, "error", err)
			skipped++
			continue
		}
		events = append(events, e)
	}
	return events, skipped, nil
}

func (r *Repository) fetchPage(ctx context.Context, c domain.Criteria, searchAfter []interface{}) (searchResponse, error) {
	body := BuildQueryBody(c, r.pageSize, searchAfter)
	payload, err := json.Marshal(body)
	if err != nil {
		return searchResponse{}, fmt.Errorf("marshal query body: %w", err)
	}

	res, err := r.client.Search(
		r.client.Search.WithContext(ctx),
		r.client.Search.WithIndex(c.Index),
		r.client.Search.WithBody(bytes.NewReader(payload)),
	)
	if err != nil {
		return searchResponse{}, fmt.Errorf("search request: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return searchResponse{}, fmt.Errorf("opensearch returned error status: %s: %s", res.Status(), body)
	}

	var parsed searchResponse
	if err := json.NewDecoder(res.Body).Decode(&parsed); err != nil {
		return searchResponse{}, fmt.Errorf("decode search response: %w", err)
	}
	slog.Info("page fetched", "hits", len(parsed.Hits.Hits), "total_matching_gte", parsed.Hits.Total.Value)
	return parsed, nil
}
