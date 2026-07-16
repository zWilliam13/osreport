package opensearch

import (
	"crypto/tls"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"time"

	opensearchgo "github.com/opensearch-project/opensearch-go/v2"

	"osreport/internal/infra/config"
)

// NewClient builds an opensearch-go client from Config. TLS is required by
// this cluster — plain HTTP gets an "empty reply from server" from
// OpenSearch's Security plugin, confirmed against the real cluster.
// InsecureSkipVerify must come from Config (i.e. from an explicit env var),
// never hardcoded true.
func NewClient(cfg config.Config) (*opensearchgo.Client, error) {
	// Clone http.DefaultTransport rather than building &http.Transport{}
	// from scratch — a bare struct literal silently drops all of Go's
	// default dial/handshake/idle-connection timeouts, so a hung TCP
	// connection to the cluster would block forever instead of erroring.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{
		InsecureSkipVerify: cfg.InsecureSkipVerify, //nolint:gosec // explicit opt-in via OS_INSECURE_SKIP_VERIFY
	}

	client, err := opensearchgo.NewClient(opensearchgo.Config{
		Addresses:            []string{cfg.Endpoint},
		Username:             cfg.Username,
		Password:             cfg.Password,
		Transport:            transport,
		RetryOnStatus:        []int{502, 503, 504},
		EnableRetryOnTimeout: true,
		MaxRetries:           5,
		RetryBackoff:         exponentialBackoffWithJitter,
	})
	if err != nil {
		return nil, fmt.Errorf("build opensearch client: %w", err)
	}
	return client, nil
}

// exponentialBackoffWithJitter caps at 8s so a flaky cluster doesn't turn a
// report run into a multi-minute hang across 5 retries.
func exponentialBackoffWithJitter(attempt int) time.Duration {
	base := math.Min(float64(attempt*attempt), 8) * float64(time.Second)
	jitter := time.Duration(rand.Int63n(int64(500 * time.Millisecond)))
	return time.Duration(base) + jitter
}
