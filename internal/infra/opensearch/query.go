package opensearch

import (
	"time"

	"osreport/internal/domain"
)

// BuildQueryBody assembles the _search request body for one page.
//
// Field paths are confirmed against the real index-athonet mapping:
//   - record.time_event (type date) — NOT a top-level "time_event", which
//     doesn't exist in the mapping and fails hard on sort.
//   - ALERT_COMPONENT.keyword / ALERT_SEVERITY.keyword — top-level fields,
//     but only populated on ~52% of documents; the rest is unrelated infra
//     log noise (auditd, snmpd, docker, ...) that should never reach the app
//     layer, hence filtering here rather than in Go after fetching.
func BuildQueryBody(c domain.Criteria, size int, searchAfter []interface{}) map[string]interface{} {
	filters := []map[string]interface{}{
		{
			"range": map[string]interface{}{
				"record.time_event": map[string]interface{}{
					"gte": c.From.UTC().Format(time.RFC3339),
					"lte": c.To.UTC().Format(time.RFC3339),
				},
			},
		},
		{"exists": map[string]interface{}{"field": "ALERT_SEVERITY"}},
	}

	if len(c.Components) > 0 {
		filters = append(filters, map[string]interface{}{
			"terms": map[string]interface{}{"ALERT_COMPONENT.keyword": c.Components},
		})
	}
	if len(c.Severities) > 0 {
		filters = append(filters, map[string]interface{}{
			"terms": map[string]interface{}{"ALERT_SEVERITY.keyword": c.Severities},
		})
	}

	body := map[string]interface{}{
		"size": size,
		// Docs on this index carry ~979 fields (Fluent Bit re-wrapping,
		// see mapper.go) but MapHit only reads these — restricting
		// _source cuts the network payload substantially on large pulls.
		"_source": []string{
			"ALERT_COMPONENT", "ALERT_SEVERITY",
			"record.time_event", "record.HOSTNAME",
			"record.MESSAGE.raw", "record.MESSAGE.msg",
		},
		"query": map[string]interface{}{"bool": map[string]interface{}{"filter": filters}},
		// "_doc" (Lucene's internal segment order) is the tiebreaker, not
		// "_id": sorting on "_id" forces OpenSearch to build fielddata for
		// the whole _id field across the index to do the comparison — on
		// this 825M-doc/227GB single-shard index that blew the 6.3GB
		// fielddata circuit breaker in testing. "_doc" needs no fielddata
		// and is the tiebreaker documented for search_after pagination.
		"sort": []map[string]interface{}{
			{"record.time_event": "asc"},
			{"_doc": "asc"},
		},
	}
	if len(searchAfter) > 0 {
		body["search_after"] = searchAfter
	}
	return body
}
