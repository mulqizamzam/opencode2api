package gateway

import (
	"strings"
	"testing"
)

// An oversized upstream body must be reported, not silently truncated: a
// truncated JSON document would be delivered to the client as a 200.
func TestReadCappedBodyRejectsOversizedBody(t *testing.T) {
	data, err := readCappedBody(strings.NewReader(strings.Repeat("a", 16)), 16)
	if err != nil {
		t.Fatalf("a body of exactly the limit was rejected: %v", err)
	}
	if len(data) != 16 {
		t.Fatalf("read %d bytes, want 16", len(data))
	}
	if _, err := readCappedBody(strings.NewReader(strings.Repeat("a", 17)), 16); err == nil {
		t.Fatalf("a body over the limit was accepted and truncated to 16 bytes")
	}
	if _, err := readCappedBody(strings.NewReader("{\"truncated\": "), 64); err != nil {
		t.Fatalf("a short body was rejected: %v", err)
	}
}
