package gateway

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"opencode2api/internal/config"
)

// Retry-After is untrusted upstream input. A second count multiplied by
// time.Second overflows int64 nanoseconds into a negative duration, and an
// HTTP-date far in the future overflows the same way. Either one used to reach
// the cooldown and the persisted rotation pause unbounded, so a single response
// could keep a key or a rotation model out of service for decades.
func TestParseRetryAfterIsBounded(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  time.Duration
	}{
		{"absent", "", 0},
		{"zero", "0", 0},
		{"negative", "-5", 0},
		{"plain seconds", "40", 40 * time.Second},
		{"over the ceiling", "86400", maxRetryAfter},
		{"int64 wrap", "20000000000", maxRetryAfter},
		{"far future date", "Mon, 01 Jan 9999 00:00:00 GMT", maxRetryAfter},
		{"past date", "Mon, 01 Jan 2001 00:00:00 GMT", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseRetryAfter(tc.value)
			if got < 0 {
				t.Fatalf("parseRetryAfter(%q) = %v: a negative pause means the int64 nanosecond product wrapped", tc.value, got)
			}
			if got > maxRetryAfter {
				t.Fatalf("parseRetryAfter(%q) = %v, want at most %v", tc.value, got, maxRetryAfter)
			}
			if tc.want == 0 && got != 0 {
				t.Fatalf("parseRetryAfter(%q) = %v, want 0", tc.value, got)
			}
			if tc.want > 0 && got != tc.want {
				t.Fatalf("parseRetryAfter(%q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

// A quota rejection whose Retry-After offers no usable delay must still take
// the documented default pause. Treating "0" as an instruction to retry at once
// re-selects the model that just refused, spending an attempt on every request.
func TestRotationQuotaWithoutUsableRetryAfterStillPausesModel(t *testing.T) {
	models := append([]string{}, config.SOTAModels[:3]...)
	upstream := newFakeUpstream(t, func(w http.ResponseWriter, r *http.Request, body []byte, call int) {
		if upstreamBodyModel(body) == models[0] {
			w.Header().Set("Retry-After", "0")
			jsonResponse(w, http.StatusTooManyRequests, map[string]any{"error": map[string]any{"message": "insufficient_quota", "type": "rate_limit_error"}})
			return
		}
		jsonResponse(w, http.StatusOK, completion("hello"))
	})
	gateway, manager := rotationGateway(t, upstream.server.URL, 30, models)

	recorder := postJSON(t, gateway, "/v1/chat/completions", groupPayload(false))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want the second model to serve; body=%s", recorder.Code, recorder.Body.String())
	}
	snapshot := manager.Snapshot()
	pausedUntil, paused := snapshot.Groups[0].Paused[models[0]]
	if !paused {
		t.Fatalf("the exhausted model was not paused at all: %v", snapshot.Groups[0].Paused)
	}
	if remaining := time.Until(pausedUntil); remaining < 30*time.Second {
		t.Fatalf("the exhausted model is paused for only %v, want the 60s default pause", remaining)
	}
}

// upstreamBodyModel reads the model a rotation attempt sent upstream.
func upstreamBodyModel(body []byte) string {
	decoded := map[string]any{}
	if json.Unmarshal(body, &decoded) != nil {
		return ""
	}
	model, _ := decoded["model"].(string)
	return model
}
