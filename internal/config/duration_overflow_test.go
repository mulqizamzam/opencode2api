package config

import (
	"testing"
	"time"
)

// validConfig is the smallest configuration Normalize accepts, copied from the
// other Normalize tests in this package.
func validConfig() Config {
	return Config{
		Listen:      "127.0.0.1:8080",
		ServerKeys:  []string{"local-key"},
		Anonymous:   true,
		Upstream:    UpstreamConfig{Zen: "https://opencode.ai/zen", Go: "https://opencode.ai/zen/go", Anonymous: "https://opencode.ai/inference"},
		Retry:       RetryConfig{MaxAttempts: 3, TimeoutSeconds: 300},
		Models:      ModelsConfig{RefreshSeconds: 300, Protocols: map[string]string{}},
		Performance: PerformanceConfig{MaxIdleConns: 10, MaxIdleConnsPerHost: 10, IdleConnTimeoutSeconds: 30, ConnectTimeoutSeconds: 5, FailureCooldownSeconds: 15},
		Logging:     LoggingConfig{Level: "info", RingSize: 100},
		Prefer:      TierGo,
	}
}

// A *_seconds value is multiplied by time.Second at the conversion sites. Above
// 9223372036 the int64 nanosecond product wraps negative, so the ticker built
// from models.refresh_seconds panics the process and every context deadline
// built from retry.timeout_seconds is already expired. Normalize is the only
// trust boundary both config.json and the management API pass through, so it
// must reject the value instead of persisting a configuration that cannot be
// turned into a duration.
func TestNormalizeRejectsSecondsThatOverflowDuration(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Config)
	}{
		{"models.refresh_seconds", func(cfg *Config) { cfg.Models.RefreshSeconds = 9223372037 }},
		{"retry.timeout_seconds", func(cfg *Config) { cfg.Retry.TimeoutSeconds = 10000000000 }},
		{"performance.idle_conn_timeout_seconds", func(cfg *Config) { cfg.Performance.IdleConnTimeoutSeconds = 9223372037 }},
		{"performance.connect_timeout_seconds", func(cfg *Config) { cfg.Performance.ConnectTimeoutSeconds = 9223372037 }},
		{"performance.failure_cooldown_seconds", func(cfg *Config) { cfg.Performance.FailureCooldownSeconds = 9223372037 }},
		{"performance.attempt_timeout_seconds", func(cfg *Config) { cfg.Performance.AttemptTimeoutSeconds = 9223372037 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validConfig()
			tc.mutate(&cfg)
			if _, err := Normalize("config.json", cfg); err == nil {
				t.Fatalf("Normalize accepted an out-of-range %s: the value overflows time.Duration and bricks the service", tc.name)
			}
		})
	}
}

// The conversion the validator protects: this is the exact expression the
// ticker in gateway.StartModelRefresh evaluates, and it must never be reached
// with a value Normalize would have accepted.
func TestUnboundedSecondsWrapsDurationNegative(t *testing.T) {
	// A variable, not a constant: the compiler rejects the literal product,
	// while the runtime expression the gateway evaluates wraps silently.
	seconds := 9223372037
	overflowed := time.Duration(seconds) * time.Second
	if overflowed > 0 {
		t.Fatalf("%d * time.Second = %v, expected an int64 nanosecond overflow", seconds, overflowed)
	}
	if 86400 <= 0 || time.Duration(86400)*time.Second <= 0 {
		t.Fatalf("86400=%d does not convert to a positive duration", 86400)
	}
}

// The ceiling must not reject anything an operator could legitimately run: the
// documented maximum for a comparable field is one day.
func TestNormalizeAcceptsDurationSecondsAtTheCeiling(t *testing.T) {
	cfg := validConfig()
	cfg.Models.RefreshSeconds = 86400
	cfg.Retry.TimeoutSeconds = 86400
	cfg.Performance.IdleConnTimeoutSeconds = 86400
	cfg.Performance.ConnectTimeoutSeconds = 86400
	cfg.Performance.FailureCooldownSeconds = 86400
	cfg.Performance.AttemptTimeoutSeconds = 86400
	if _, err := Normalize("config.json", cfg); err != nil {
		t.Fatalf("Normalize rejected the documented ceiling: %v", err)
	}
}
