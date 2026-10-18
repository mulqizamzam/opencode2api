package protocol

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

// transcode drives one upstream SSE body through the real transcoder and
// returns the frames the downstream client received.
func transcode(t *testing.T, from, to Protocol, upstream string) (string, error) {
	t.Helper()
	recorder := httptest.NewRecorder()
	_, _, err := TranscodeStream(context.Background(), recorder, strings.NewReader(upstream), from, to, "test-model")
	return recorder.Body.String(), err
}

// sseFrames splits an emitted SSE body into (eventName, data) pairs.
func sseFrames(t *testing.T, body string) [][2]string {
	t.Helper()
	frames := [][2]string{}
	eventName := ""
	for _, line := range strings.Split(body, "\n") {
		switch {
		case strings.HasPrefix(line, "event:"):
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			frames = append(frames, [2]string{eventName, strings.TrimSpace(strings.TrimPrefix(line, "data:"))})
		}
	}
	return frames
}

const chatPrelude = `data: {"id":"chatcmpl-1","model":"test-model","choices":[{"index":0,"delta":{"role":"assistant"}}]}` + "\n\n"

// Reasoning fragments must reach the client byte-for-byte. A leading space is
// part of the fragment: the client concatenates the deltas, so trimming one
// delta glues two words together ("Let me" + " think" -> "Let methink").
func TestReasoningDeltaPreservesWhitespace(t *testing.T) {
	upstream := chatPrelude +
		`data: {"id":"chatcmpl-1","choices":[{"index":0,"delta":{"reasoning_content":"Let me"}}]}` + "\n\n" +
		`data: {"id":"chatcmpl-1","choices":[{"index":0,"delta":{"reasoning_content":" think"}}]}` + "\n\n" +
		`data: {"id":"chatcmpl-1","choices":[{"index":0,"delta":{"reasoning_content":"\n\n"}}]}` + "\n\n" +
		`data: {"id":"chatcmpl-1","choices":[{"index":0,"delta":{"content":"done"},"finish_reason":"stop"}]}` + "\n\n" +
		"data: [DONE]\n\n"

	body, err := transcode(t, Chat, Anthropic, upstream)
	if err != nil {
		t.Fatalf("TranscodeStream: %v", err)
	}
	thinking := ""
	for _, frame := range sseFrames(t, body) {
		var payload map[string]any
		if json.Unmarshal([]byte(frame[1]), &payload) != nil {
			continue
		}
		if payload["type"] != "content_block_delta" {
			continue
		}
		delta, _ := payload["delta"].(map[string]any)
		if delta["type"] == "thinking_delta" {
			thinking += delta["thinking"].(string)
		}
	}
	if thinking != "Let me think\n\n" {
		t.Fatalf("concatenated thinking = %q, want %q", thinking, "Let me think\n\n")
	}
}

const anthropicRedactedStream = `event: message_start
data: {"type":"message_start","message":{"id":"msg_1","model":"test-model","usage":{"input_tokens":3}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"redacted_thinking","data":"ENCRYPTED_BLOB"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}

event: message_stop
data: {"type":"message_stop"}

`

// The terminal response.completed document is what a client stores for the next
// turn. It must agree with the output_item.done events: an encrypted reasoning
// payload present in the streamed item must also be present in the completed
// document, otherwise the reasoning reference is lost and the replay 400s.
func TestCompletedDocumentKeepsEncryptedReasoning(t *testing.T) {
	body, err := transcode(t, Anthropic, Responses, anthropicRedactedStream)
	if err != nil {
		t.Fatalf("TranscodeStream: %v", err)
	}
	completed := map[string]any{}
	seenItemDone := false
	for _, frame := range sseFrames(t, body) {
		var payload map[string]any
		if json.Unmarshal([]byte(frame[1]), &payload) != nil {
			continue
		}
		switch payload["type"] {
		case "response.output_item.done":
			item, _ := payload["item"].(map[string]any)
			if item["encrypted_content"] == "ENCRYPTED_BLOB" {
				seenItemDone = true
			}
		case "response.completed":
			completed, _ = payload["response"].(map[string]any)
		}
	}
	if !seenItemDone {
		t.Fatalf("upstream encrypted reasoning never reached output_item.done:\n%s", body)
	}
	output, _ := completed["output"].([]any)
	found := ""
	for _, raw := range output {
		item, _ := raw.(map[string]any)
		if item["type"] == "reasoning" {
			found, _ = item["encrypted_content"].(string)
		}
	}
	if found != "ENCRYPTED_BLOB" {
		t.Fatalf("response.completed reasoning encrypted_content = %q, want ENCRYPTED_BLOB; output=%v", found, output)
	}
}

// A final SSE record that arrives complete but without its trailing blank line
// (a very common last-write truncation) must still be dispatched. Dropping it
// turns a complete stream into a reported failure.
func TestFinalRecordWithoutBlankLineIsDispatched(t *testing.T) {
	upstream := chatPrelude +
		`data: {"id":"chatcmpl-1","choices":[{"index":0,"delta":{"content":"hello"},"finish_reason":"stop"}]}` + "\n\n" +
		"data: [DONE]\n"

	body, err := transcode(t, Chat, Anthropic, upstream)
	if err != nil {
		t.Fatalf("TranscodeStream: %v", err)
	}
	for _, frame := range sseFrames(t, body) {
		if frame[0] == "error" {
			t.Fatalf("complete stream reported as failed: %s", frame[1])
		}
	}
	if !strings.Contains(body, "message_stop") {
		t.Fatalf("stream without terminal blank line lost its terminal event:\n%s", body)
	}
}

// A stream that terminated normally must not be reported as failed because of a
// frame that arrived after the terminal event. The gateway passes upstream bytes
// through verbatim, so a trailing non-JSON data frame is the upstream's problem,
// not a reason to inject an error frame into an already-complete response.
func TestTrailingJunkAfterNormalTerminationIsNotAFailure(t *testing.T) {
	upstream := anthropicRedactedStream + "data: [DONE]\n\n"
	recorder := httptest.NewRecorder()
	_, _, err := ForwardStream(context.Background(), recorder, strings.NewReader(upstream), Anthropic, "test-model")
	body := recorder.Body.String()
	for _, frame := range sseFrames(t, body) {
		if frame[0] == "error" {
			t.Fatalf("normal termination reported as a failure: %s\n%s", frame[1], body)
		}
	}
	if err != nil {
		t.Fatalf("ForwardStream returned %v for a normally terminated stream", err)
	}
}
