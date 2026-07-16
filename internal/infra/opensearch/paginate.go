package opensearch

import (
	"encoding/json"
	"fmt"
)

type hit struct {
	ID     string          `json:"_id"`
	Source json.RawMessage `json:"_source"`
	Sort   []interface{}   `json:"sort"`
}

type searchResponse struct {
	Hits struct {
		Total struct {
			Value int `json:"value"`
		} `json:"total"`
		Hits []hit `json:"hits"`
	} `json:"hits"`
}

// fetchPageFunc executes one page of a search_after query. searchAfter is
// nil for the first page.
type fetchPageFunc func(searchAfter []interface{}) (searchResponse, error)

// paginateSearchAfter drives fetchPage until a page comes back with fewer
// hits than pageSize, using search_after (not scroll) — stateless, so a
// report run that's interrupted doesn't leave a scroll context open on the
// cluster. Most runs against index-athonet resolve in a single page: a
// component+severity filtered day is on the order of thousands of hits,
// well under pageSize.
func paginateSearchAfter(pageSize int, fetchPage fetchPageFunc) ([]hit, error) {
	var all []hit
	var searchAfter []interface{}

	for {
		resp, err := fetchPage(searchAfter)
		if err != nil {
			return nil, err
		}

		n := len(resp.Hits.Hits)
		if n == 0 {
			break
		}
		all = append(all, resp.Hits.Hits...)

		if n < pageSize {
			break
		}

		// A full page means there's more to fetch, which requires a
		// non-empty sort tiebreaker on the last hit. If that's ever
		// missing (malformed document, mapping surprise), advancing with
		// an empty searchAfter would make the next request equivalent to
		// the first page — an infinite loop re-fetching page 1 until the
		// caller's context deadline, rather than a fast, clear failure.
		next := resp.Hits.Hits[n-1].Sort
		if len(next) == 0 {
			return nil, fmt.Errorf("search_after: last hit on a full page has no sort values, cannot page further")
		}
		searchAfter = next
	}

	return all, nil
}
