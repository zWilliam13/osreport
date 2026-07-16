package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds everything needed to talk to OpenSearch. Never populate this
// from hardcoded values — always from environment variables (or an
// EnvironmentFile under systemd in production).
type Config struct {
	Endpoint           string // e.g. https://172.26.0.210:9200
	Username           string
	Password           string
	InsecureSkipVerify bool          // only true for self-signed certs on trusted internal networks — never default
	Timeout            time.Duration // overall deadline for one report run (covers every paginated request + retries), not a per-request timeout
}

// Load reads OS_ENDPOINT, OS_USERNAME, OS_PASSWORD, OS_INSECURE_SKIP_VERIFY
// and OS_TIMEOUT_SECONDS from the environment and validates them fail-fast:
// a report run should refuse to start rather than fail midway through a
// long-running scroll because of a missing credential.
func Load() (Config, error) {
	cfg := Config{
		Endpoint: os.Getenv("OS_ENDPOINT"),
		Username: os.Getenv("OS_USERNAME"),
		Password: os.Getenv("OS_PASSWORD"),
		Timeout:  2 * time.Minute,
	}

	if cfg.Endpoint == "" {
		return Config{}, fmt.Errorf("OS_ENDPOINT is required (e.g. https://host:9200)")
	}
	if !strings.HasPrefix(cfg.Endpoint, "http://") && !strings.HasPrefix(cfg.Endpoint, "https://") {
		return Config{}, fmt.Errorf("OS_ENDPOINT must start with http:// or https://, got %q", cfg.Endpoint)
	}
	if cfg.Username == "" {
		return Config{}, fmt.Errorf("OS_USERNAME is required")
	}
	if cfg.Password == "" {
		return Config{}, fmt.Errorf("OS_PASSWORD is required")
	}

	if v := os.Getenv("OS_INSECURE_SKIP_VERIFY"); v != "" {
		skip, err := strconv.ParseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("OS_INSECURE_SKIP_VERIFY must be true/false: %w", err)
		}
		cfg.InsecureSkipVerify = skip
	}

	if v := os.Getenv("OS_TIMEOUT_SECONDS"); v != "" {
		secs, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("OS_TIMEOUT_SECONDS must be an integer: %w", err)
		}
		if secs <= 0 {
			return Config{}, fmt.Errorf("OS_TIMEOUT_SECONDS must be positive, got %d", secs)
		}
		cfg.Timeout = time.Duration(secs) * time.Second
	}

	return cfg, nil
}
