package gateway

import (
	"net/http"
	"testing"
)

func chatPayload() map[string]any {
	return map[string]any{
		"model":    "test-model",
		"messages": []map[string]any{{"role": "user", "content": "hi"}},
	}
}

func completion(body string) map[string]any {
	return map[string]any{
		"id": "chatcmpl-1", "object": "chat.completion", "model": "test-model",
		"choices": []map[string]any{{"index": 0, "message": map[string]any{"role": "assistant", "content": body}, "finish_reason": "stop"}},
		"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 1, "total_tokens": 4},
	}
}

// A retryable upstream failure is retried up to retry.max_attempts. When every
// attempt failed with a response, the client receives the upstream's own status
// and message instead of a synthetic gateway error.
func TestRetryableUpstreamFailureIsRetriedThenReported(t *testing.T) {
	upstream := newFakeUpstream(t, func(w http.ResponseWriter, r *http.Request, body []byte, call int) {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"message": "upstream exploded"}})
	})
	gateway := newTestGateway(t, upstream.server.URL, nil)
	recorder := postJSON(t, gateway, "/v1/chat/completions", chatPayload())
	if got := upstream.requestCount(); got != 3 {
		t.Fatalf("upstream saw %d attempts, want retry.max_attempts=3", got)
	}
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want the upstream status 500; body=%s", recorder.Code, recorder.Body.String())
	}
	decoded := decodeJSON(t, recorder.Body.Bytes())
	message, _ := decoded["error"].(map[string]any)["message"].(string)
	if message != "upstream exploded" {
		t.Fatalf("client message = %q, want the upstream message", message)
	}
}

// A request-shape rejection is deterministic: rotating keys through it only
// burns quota, so it must reach the client after a single attempt.
func TestNonRetryableUpstreamRejectionIsNotRetried(t *testing.T) {
	upstream := newFakeUpstream(t, func(w http.ResponseWriter, r *http.Request, body []byte, call int) {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"message": "unknown field 'foo'"}})
	})
	gateway := newTestGateway(t, upstream.server.URL, nil)
	recorder := postJSON(t, gateway, "/v1/chat/completions", chatPayload())
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", recorder.Code, recorder.Body.String())
	}
	if got := upstream.requestCount(); got != 1 {
		t.Fatalf("upstream saw %d attempts for a deterministic rejection, want 1", got)
	}
}

// A healthy non-streaming completion round-trips unchanged.
func TestNonStreamingCompletionRoundTrip(t *testing.T) {
	upstream := newFakeUpstream(t, func(w http.ResponseWriter, r *http.Request, body []byte, call int) {
		jsonResponse(w, http.StatusOK, completion("hello"))
	})
	gateway := newTestGateway(t, upstream.server.URL, nil)
	recorder := postJSON(t, gateway, "/v1/chat/completions", chatPayload())
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	decoded := decodeJSON(t, recorder.Body.Bytes())
	choices, _ := decoded["choices"].([]any)
	if len(choices) != 1 {
		t.Fatalf("choices = %v, want one entry", decoded["choices"])
	}
	message, _ := choices[0].(map[string]any)["message"].(map[string]any)
	if message["content"] != "hello" {
		t.Fatalf("content = %v, want hello", message["content"])
	}
}
