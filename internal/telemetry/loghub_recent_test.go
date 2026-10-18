package telemetry

import "testing"

// recentHub returns a hub holding sequence 1..published.
func recentHub(t *testing.T, capacity, published int) *LogHub {
	t.Helper()
	hub := NewLogHub(capacity)
	for i := 1; i <= published; i++ {
		hub.Publish(LogEvent{Level: "info", Message: "event"})
	}
	return hub
}

func sequences(events []LogEvent) []uint64 {
	out := make([]uint64, 0, len(events))
	for _, event := range events {
		out = append(out, event.Sequence)
	}
	return out
}

func assertSequences(t *testing.T, got []LogEvent, want []uint64) {
	t.Helper()
	actual := sequences(got)
	if len(actual) != len(want) {
		t.Fatalf("sequences = %v, want %v", actual, want)
	}
	for i := range want {
		if actual[i] != want[i] {
			t.Fatalf("sequences = %v, want %v", actual, want)
		}
	}
}

// The admin log stream pages forward from the cursor the browser presents, and
// treats gap as "older log events have expired". A reply cut down to the newest
// events leaves a hole between that cursor and the first event sent, so it must
// be reported as a gap instead of silently skipping it.
func TestRecentReportsGapWhenTheLimitDropsResumedEvents(t *testing.T) {
	hub := recentHub(t, 100, 20)

	events, gap := hub.Recent(5, 3)
	assertSequences(t, events, []uint64{18, 19, 20})
	if !gap {
		t.Fatalf("Recent(5, 3) dropped events 6..17 and reported gap=false")
	}

	// A fresh viewer is meant to see the tail: there is no cursor to resume
	// from, so no hole is reported.
	events, gap = hub.Recent(0, 3)
	assertSequences(t, events, []uint64{18, 19, 20})
	if gap {
		t.Fatalf("Recent(0, 3) reported a gap for a fresh subscription")
	}

	// A cursor inside the window still reports nothing missing.
	events, gap = hub.Recent(19, 3)
	assertSequences(t, events, []uint64{20})
	if gap {
		t.Fatalf("Recent(19, 3) reported a gap although nothing was dropped")
	}

	events, gap = hub.Recent(19, 100)
	assertSequences(t, events, []uint64{20})
	if gap {
		t.Fatalf("Recent(19, 100) reported a gap although the cursor is inside the ring")
	}

	// A wrapped ring: 250 events through a 100 slot buffer leaves 151..250, so
	// a cursor at 2 has genuinely expired events behind it.
	wrapped := recentHub(t, 100, 250)
	events, gap = wrapped.Recent(2, 1000)
	if first := events[0].Sequence; first != 151 {
		t.Fatalf("first buffered sequence = %d, want 151", first)
	}
	if !gap {
		t.Fatalf("Recent(2, 1000) reported no gap although events 3..150 had expired")
	}

	// The same ring, but the reply is also truncated by the limit: the hole is
	// reported once, and the newest events are the ones shipped.
	events, gap = wrapped.Recent(2, 10)
	assertSequences(t, events, []uint64{241, 242, 243, 244, 245, 246, 247, 248, 249, 250})
	if !gap {
		t.Fatalf("Recent(2, 10) dropped 151..240 and reported gap=false")
	}
}
