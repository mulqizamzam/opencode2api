package gateway

import (
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"opencode2api/internal/config"
	wire "opencode2api/internal/protocol"
	"opencode2api/internal/rotation"
)

// rotationGateway wires a Gateway whose "sota" group owns the given models, all
// served over the Go tier.
func rotationGateway(t *testing.T, upstreamURL string, firstOutputSeconds int, models []string) (*Gateway, *rotation.Manager) {
	t.Helper()
	cfg := testConfig(upstreamURL, nil)
	cfg.Rotation = config.RotationConfig{
		SOTA:               config.RotationGroup{Alias: "sota", Order: append([]string{}, models...)},
		Sweet:              config.RotationGroup{Alias: "sweet"},
		FirstOutputSeconds: firstOutputSeconds,
	}
	gateway, err := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	if err != nil {
		t.Fatalf("gateway.New: %v", err)
	}
	manager, err := rotation.New("", cfg.Rotation)
	if err != nil {
		t.Fatalf("rotation.New: %v", err)
	}
	gateway.rotation = manager
	native := map[config.Tier]map[string]wire.Protocol{config.TierGo: {}, config.TierZen: {}}
	for _, model := range models {
		native[config.TierGo][model] = wire.Chat
		native[config.TierZen][model] = wire.Chat
	}
	gateway.catalog.ReplaceWithCapabilities(nil, models, native, nil, nil)
	gateway.syncRotation()
	return gateway, manager
}

func groupPayload(stream bool) map[string]any {
	return map[string]any{
		"model":    "sota",
		"stream":   stream,
		"messages": []map[string]any{{"role": "user", "content": "hi"}},
	}
}

// The first-output timer exists so a stalled upstream does not hold a streaming
// turn open. A non-stream request has no first event to wait for — its entire
// body is the response — so the timer must not double as a total generation
// deadline: upstream here answers normally, only later than
// rotation.first_output_seconds.
func TestRotationNonStreamRequestSurvivesFirstOutputWindow(t *testing.T) {
	models := append([]string{}, config.SOTAModels[:3]...)
	upstream := newFakeUpstream(t, func(w http.ResponseWriter, r *http.Request, body []byte, call int) {
		time.Sleep(1200 * time.Millisecond)
		jsonResponse(w, http.StatusOK, completion("hello"))
	})
	gateway, manager := rotationGateway(t, upstream.server.URL, 1, models)

	recorder := postJSON(t, gateway, "/v1/chat/completions", groupPayload(false))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	if got := upstream.requestCount(); got != 1 {
		t.Fatalf("upstream saw %d attempts, want 1: a healthy slow response must not be retried", got)
	}
	decoded := decodeJSON(t, recorder.Body.Bytes())
	choices, _ := decoded["choices"].([]any)
	if len(choices) != 1 {
		t.Fatalf("choices = %v, want the upstream completion", decoded["choices"])
	}
	for _, group := range manager.Snapshot().Groups {
		if len(group.Paused) != 0 {
			t.Fatalf("a healthy upstream got its model paused: %v", group.Paused)
		}
	}
}

// The timer must still bound a streaming attempt that never produces a first
// event, otherwise a stalled upstream holds the turn open until the request
// budget expires. One attempt per model keeps the timing observable: three
// models stalled for 5s each would take 15s if the timer did not fire.
func TestRotationStreamRequestStillTimesOutWithoutFirstOutput(t *testing.T) {
	models := append([]string{}, config.SOTAModels[:3]...)
	upstream := newFakeUpstream(t, func(w http.ResponseWriter, r *http.Request, body []byte, call int) {
		time.Sleep(5 * time.Second)
		jsonResponse(w, http.StatusOK, completion("late"))
	})
	gateway, _ := rotationGateway(t, upstream.server.URL, 1, models)

	const stall = 5 * time.Second
	started := time.Now()
	recorder := postJSON(t, gateway, "/v1/chat/completions", groupPayload(true))
	elapsed := time.Since(started)
	calls := upstream.requestCount()
	t.Logf("status=%d calls=%d elapsed=%v", recorder.Code, calls, elapsed)
	if recorder.Code/100 == 2 {
		t.Fatalf("a stalled stream ended with status %d after %v; body=%s", recorder.Code, elapsed, recorder.Body.String())
	}
	// Each attempt must be abandoned long before the upstream could answer;
	// without the timer every attempt would run the full stall.
	if elapsed >= time.Duration(calls)*stall/2 {
		t.Fatalf("the first-output timeout of 1s never cut a stalled attempt: %v elapsed over %d calls", elapsed, calls)
	}
}
