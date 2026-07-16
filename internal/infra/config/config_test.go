package config

import (
	"testing"
	"time"
)

func setEnv(t *testing.T, vars map[string]string) {
	t.Helper()
	for k, v := range vars {
		t.Setenv(k, v)
	}
}

func TestLoad_Success(t *testing.T) {
	setEnv(t, map[string]string{
		"OS_ENDPOINT":             "https://172.26.0.210:9200",
		"OS_USERNAME":             "user",
		"OS_PASSWORD":             "pass",
		"OS_INSECURE_SKIP_VERIFY": "true",
		"OS_TIMEOUT_SECONDS":      "45",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Endpoint != "https://172.26.0.210:9200" || cfg.Username != "user" || cfg.Password != "pass" {
		t.Errorf("unexpected cfg = %+v", cfg)
	}
	if !cfg.InsecureSkipVerify {
		t.Error("InsecureSkipVerify = false, want true")
	}
	if cfg.Timeout != 45*time.Second {
		t.Errorf("Timeout = %v, want 45s", cfg.Timeout)
	}
}

func TestLoad_EndpointMissingScheme(t *testing.T) {
	setEnv(t, map[string]string{
		"OS_ENDPOINT": "172.26.0.210:9200",
		"OS_USERNAME": "u",
		"OS_PASSWORD": "p",
	})
	if _, err := Load(); err == nil {
		t.Error("expected error for OS_ENDPOINT without http(s):// scheme, got nil")
	}
}

func TestLoad_DefaultsWhenOptionalVarsUnset(t *testing.T) {
	setEnv(t, map[string]string{
		"OS_ENDPOINT": "https://172.26.0.210:9200",
		"OS_USERNAME": "user",
		"OS_PASSWORD": "pass",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.InsecureSkipVerify {
		t.Error("InsecureSkipVerify = true, want false by default")
	}
	if cfg.Timeout != 2*time.Minute {
		t.Errorf("Timeout = %v, want 2m default", cfg.Timeout)
	}
}

func TestLoad_MissingRequiredVars(t *testing.T) {
	tests := []struct {
		name string
		vars map[string]string
	}{
		{"missing endpoint", map[string]string{"OS_USERNAME": "u", "OS_PASSWORD": "p"}},
		{"missing username", map[string]string{"OS_ENDPOINT": "https://h:9200", "OS_PASSWORD": "p"}},
		{"missing password", map[string]string{"OS_ENDPOINT": "https://h:9200", "OS_USERNAME": "u"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setEnv(t, tt.vars)
			if _, err := Load(); err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestLoad_InvalidInsecureSkipVerify(t *testing.T) {
	setEnv(t, map[string]string{
		"OS_ENDPOINT":             "https://h:9200",
		"OS_USERNAME":             "u",
		"OS_PASSWORD":             "p",
		"OS_INSECURE_SKIP_VERIFY": "not-a-bool",
	})
	if _, err := Load(); err == nil {
		t.Error("expected error for invalid OS_INSECURE_SKIP_VERIFY, got nil")
	}
}

func TestLoad_InvalidTimeoutSeconds(t *testing.T) {
	setEnv(t, map[string]string{
		"OS_ENDPOINT":        "https://h:9200",
		"OS_USERNAME":        "u",
		"OS_PASSWORD":        "p",
		"OS_TIMEOUT_SECONDS": "not-an-int",
	})
	if _, err := Load(); err == nil {
		t.Error("expected error for invalid OS_TIMEOUT_SECONDS, got nil")
	}
}

func TestLoad_ZeroOrNegativeTimeoutSeconds(t *testing.T) {
	for _, v := range []string{"0", "-5"} {
		t.Run(v, func(t *testing.T) {
			setEnv(t, map[string]string{
				"OS_ENDPOINT":        "https://h:9200",
				"OS_USERNAME":        "u",
				"OS_PASSWORD":        "p",
				"OS_TIMEOUT_SECONDS": v,
			})
			if _, err := Load(); err == nil {
				t.Errorf("expected error for OS_TIMEOUT_SECONDS=%s (would produce an already-expired context), got nil", v)
			}
		})
	}
}
