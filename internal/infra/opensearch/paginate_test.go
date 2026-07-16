package opensearch

import (
	"testing"
)

func hitWithSort(id string, sort []interface{}) hit {
	return hit{ID: id, Sort: sort}
}

func TestPaginateSearchAfter_SinglePageUnderPageSize(t *testing.T) {
	calls := 0
	got, err := paginateSearchAfter(10, func(searchAfter []interface{}) (searchResponse, error) {
		calls++
		var resp searchResponse
		resp.Hits.Hits = []hit{hitWithSort("1", []interface{}{1.0, "a"})}
		return resp, nil
	})
	if err != nil {
		t.Fatalf("paginateSearchAfter() error = %v", err)
	}
	if calls != 1 {
		t.Errorf("fetchPage called %d times, want 1 (page under pageSize should stop)", calls)
	}
	if len(got) != 1 {
		t.Errorf("got %d hits, want 1", len(got))
	}
}

func TestPaginateSearchAfter_MultiplePagesAdvanceSearchAfter(t *testing.T) {
	pages := [][]hit{
		{hitWithSort("1", []interface{}{1.0}), hitWithSort("2", []interface{}{2.0})},
		{hitWithSort("3", []interface{}{3.0})},
	}
	var seenSearchAfter [][]interface{}
	call := 0
	got, err := paginateSearchAfter(2, func(searchAfter []interface{}) (searchResponse, error) {
		seenSearchAfter = append(seenSearchAfter, searchAfter)
		var resp searchResponse
		resp.Hits.Hits = pages[call]
		call++
		return resp, nil
	})
	if err != nil {
		t.Fatalf("paginateSearchAfter() error = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d hits, want 3", len(got))
	}
	if seenSearchAfter[0] != nil {
		t.Errorf("first call searchAfter = %v, want nil", seenSearchAfter[0])
	}
	if len(seenSearchAfter) != 2 || seenSearchAfter[1] == nil {
		t.Errorf("second call should have received the first page's last sort value, got %v", seenSearchAfter)
	}
}

// TestPaginateSearchAfter_EmptySortOnFullPageErrors guards against the
// infinite-loop failure mode: a full page (n == pageSize) whose last hit
// has no sort values would otherwise make the next request equivalent to
// the very first one, re-fetching page 1 forever until the caller's
// context deadline instead of failing fast.
func TestPaginateSearchAfter_EmptySortOnFullPageErrors(t *testing.T) {
	calls := 0
	_, err := paginateSearchAfter(2, func(searchAfter []interface{}) (searchResponse, error) {
		calls++
		var resp searchResponse
		resp.Hits.Hits = []hit{hitWithSort("1", nil), hitWithSort("2", nil)} // full page, no sort
		return resp, nil
	})
	if err == nil {
		t.Fatal("expected an error for a full page with empty sort values, got nil")
	}
	if calls != 1 {
		t.Errorf("fetchPage called %d times, want 1 (should fail fast, not loop)", calls)
	}
}

func TestPaginateSearchAfter_NoHits(t *testing.T) {
	got, err := paginateSearchAfter(10, func(searchAfter []interface{}) (searchResponse, error) {
		return searchResponse{}, nil
	})
	if err != nil {
		t.Fatalf("paginateSearchAfter() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d hits, want 0", len(got))
	}
}
