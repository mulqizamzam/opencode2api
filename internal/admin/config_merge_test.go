package admin

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"opencode2api/internal/config"
	"opencode2api/internal/gateway"
	"opencode2api/internal/telemetry"
)

// customAnonymous is the operator's own keyless endpoint. It is not edited by
// the console, so nothing the console saves may change it.
const customAnonymous = "https://anonymous.example.test/inference"

func testRuntimeConfig() config.Config {
	return config.Config{
		Listen:      "127.0.0.1:8080",
		ServerKeys:  []string{"local-key"},
		Anonymous:   true,
		Upstream:    config.UpstreamConfig{Zen: "https://opencode.ai/zen", Go: "https://opencode.ai/zen/go", Anonymous: customAnonymous},
		Retry:       config.RetryConfig{MaxAttempts: 3, TimeoutSeconds: 300},
		Models:      config.ModelsConfig{RefreshSeconds: 300, Protocols: map[string]string{}},
		Performance: config.PerformanceConfig{MaxIdleConns: 10, MaxIdleConnsPerHost: 10, IdleConnTimeoutSeconds: 30, ConnectTimeoutSeconds: 5, FailureCooldownSeconds: 15},
		Logging:     config.LoggingConfig{Level: "error", RingSize: 100},
		WebUI:       config.WebUIConfig{Enabled: false, SessionTTLMinutes: 720},
		Prefer:      config.TierGo,
	}
}

// newTestAdmin wires the real runtime manager over a temporary config file. The
// root context is already cancelled so the background catalog refreshers stop
// before their first fetch.
func newTestAdmin(t *testing.T, cfg config.Config) (*Server, *gateway.RuntimeManager) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.SaveAtomic(path, cfg); err != nil {
		t.Fatalf("write config: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	level := new(slog.LevelVar)
	hub := telemetry.NewLogHub(64)
	redactor := config.NewSecretRedactor()
	redactor.Replace(cfg)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	manager, err := gateway.NewRuntimeManager(ctx, path, cfg, logger, nil, hub, redactor, level)
	if err != nil {
		t.Fatalf("NewRuntimeManager: %v", err)
	}
	t.Cleanup(manager.Shutdown)
	return New(manager, nil, hub, logger), manager
}

// consolePayload mirrors the object the configuration page sends: the upstream
// object carries zen and go only, never anonymous.
func consolePayload() map[string]any {
	return map[string]any{
		"listen":      "127.0.0.1:8080",
		"server_keys": []map[string]any{{"id": config.Fingerprint("local-key")}},
		"zen_keys":    []map[string]any{},
		"go_keys":     []map[string]any{},
		"anonymous":   true,
		"proxies":     []map[string]any{},
		"proxyfile":   "",
		"prefer":      "go",
		"upstream":    map[string]any{"zen": "https://opencode.ai/zen", "go": "https://opencode.ai/zen/go"},
		"retry":       map[string]any{"max_attempts": 3, "timeout_seconds": 300},
		"models":      map[string]any{"refresh_seconds": 300, "protocols": map[string]any{}, "free_only": false},
		"reasoning":   map[string]any{"effort": "", "effort_by_model": map[string]any{}},
		"performance": map[string]any{"max_idle_conns": 10, "max_idle_conns_per_host": 10, "max_conns_per_host": 0,
			"idle_conn_timeout_seconds": 30, "connect_timeout_seconds": 5, "failure_cooldown_seconds": 15,
			"attempt_timeout_seconds": 0, "first_event_timeout_seconds": 0},
		"logging": map[string]any{"level": "error", "ring_size": 100, "dump_request_bodies": false},
		"webui":   map[string]any{"enabled": false, "listen": "0.0.0.0:8081", "session_ttl_minutes": 720},
	}
}

// A configuration save from the console must not touch a field the console does
// not edit. Normalize replaces an empty anonymous endpoint with the built-in
// default, so passing the console's payload straight through silently moved the
// keyless lane back to the default host on every save.
func TestHandlePutConfigPreservesStoredAnonymousUpstream(t *testing.T) {
	server, manager := newTestAdmin(t, testRuntimeConfig())
	if got := manager.Config().Upstream.Anonymous; got != customAnonymous {
		t.Fatalf("test setup lost the custom anonymous endpoint: %q", got)
	}
	body, err := json.Marshal(consolePayload())
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/config", bytesReader(body))
	request.Header.Set("Content-Type", "application/json")
	server.handlePutConfig(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("PUT /api/config = %d: %s", recorder.Code, recorder.Body.String())
	}
	if got := manager.Config().Upstream.Anonymous; got != customAnonymous {
		t.Fatalf("saving the console page reset upstream.anonymous to %q, want %q", got, customAnonymous)
	}
}

func bytesReader(body []byte) io.Reader {
	return &byteReader{body: body}
}

type byteReader struct {
	body []byte
}

func (r *byteReader) Read(p []byte) (int, error) {
	if len(r.body) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.body)
	r.body = r.body[n:]
	return n, nil
}
