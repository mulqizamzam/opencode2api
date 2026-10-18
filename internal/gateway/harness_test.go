package gateway

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"opencode2api/internal/config"
)

// upstreamCall is one request the gateway sent to the fake upstream.
type upstreamCall struct {
	Path    string
	Header  http.Header
	Body    []byte
	Decoded map[string]any
}

// fakeUpstream is a stand-in for the OpenCode Zen origin. It records every
// request the gateway makes and answers with a handler the test supplies.
type fakeUpstream struct {
	server *httptest.Server
	mu     sync.Mutex
	calls  []upstreamCall
	handle func(w http.ResponseWriter, r *http.Request, body []byte, call int)
}

func newFakeUpstream(t *testing.T, handle func(w http.ResponseWriter, r *http.Request, body []byte, call int)) *fakeUpstream {
	t.Helper()
	upstream := &fakeUpstream{handle: handle}
	upstream.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		call := upstream.record(r, body)
		upstream.handle(w, r, body, call)
	}))
	t.Cleanup(upstream.server.Close)
	return upstream
}

func (u *fakeUpstream) record(r *http.Request, body []byte) int {
	u.mu.Lock()
	defer u.mu.Unlock()
	decoded := map[string]any{}
	_ = json.Unmarshal(body, &decoded)
	u.calls = append(u.calls, upstreamCall{Path: r.URL.Path, Header: r.Header.Clone(), Body: body, Decoded: decoded})
	return len(u.calls)
}

func (u *fakeUpstream) requestCount() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.calls)
}

// jsonResponse writes one JSON body with the given status.
func jsonResponse(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	encoded, _ := json.Marshal(body)
	_, _ = w.Write(encoded)
}

// testConfig builds the smallest configuration the gateway needs.
func testConfig(upstreamURL string, mutate func(*config.Config)) config.Config {
	cfg := config.Config{
		Listen:     "127.0.0.1:0",
		ServerKeys: []string{"test-server-key"},
		ZenKeys:    []string{"zen-key-one"},
		GoKeys:     []string{"go-key-one"},
		Upstream:   config.UpstreamConfig{Zen: upstreamURL, Go: upstreamURL, Anonymous: upstreamURL},
		Retry:      config.RetryConfig{MaxAttempts: 3, TimeoutSeconds: 30},
		// The protocol override is what marks the model as exposed: without it
		// the catalog has no capability snapshot and rejects the request.
		Models:      config.ModelsConfig{RefreshSeconds: 300, Protocols: map[string]string{"test-model": "chat", "test-model-go": "chat"}},
		Performance: config.PerformanceConfig{MaxIdleConns: 8, MaxIdleConnsPerHost: 8, IdleConnTimeoutSeconds: 30, ConnectTimeoutSeconds: 5, FailureCooldownSeconds: 15},
		Logging:     config.LoggingConfig{Level: "error", RingSize: 64},
		Prefer:      config.TierZen,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	return cfg
}

// newTestGateway wires a Gateway against the fake upstream with one Zen model
// named "test-model".
func newTestGateway(t *testing.T, upstreamURL string, mutate func(*config.Config)) *Gateway {
	t.Helper()
	cfg := testConfig(upstreamURL, mutate)
	gateway, err := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	if err != nil {
		t.Fatalf("gateway.New: %v", err)
	}
	gateway.catalog.Replace([]string{"test-model"}, []string{"test-model-go"})
	return gateway
}

// postJSON drives one request through the gateway's real HTTP handler.
func postJSON(t *testing.T, gateway *Gateway, path string, payload map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, path, bytesReader(encoded))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer test-server-key")
	recorder := httptest.NewRecorder()
	gateway.Handler().ServeHTTP(recorder, request)
	return recorder
}

func bytesReader(body []byte) io.Reader {
	return &sliceReader{body: body}
}

type sliceReader struct {
	body []byte
}

func (r *sliceReader) Read(p []byte) (int, error) {
	if len(r.body) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.body)
	r.body = r.body[n:]
	return n, nil
}

func decodeJSON(t *testing.T, body []byte) map[string]any {
	t.Helper()
	decoded := map[string]any{}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode response %q: %v", string(body), err)
	}
	return decoded
}
