package jsonutil

import (
	"encoding/json"
	"math"
	"testing"
)

// A decoded JSON number that has no int64 representation must read as absent
// rather than as a garbage count. Go's float-to-int conversion is
// implementation-defined out of range: on amd64 1e300 and NaN both land on
// math.MinInt64, which every caller would treat as a real token count.
func TestIntAtRefusesValuesWithoutAnIntegerRepresentation(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  int64
	}{
		{"integral float", float64(42), 42},
		{"fraction truncates", float64(3.9), 3},
		{"negative", float64(-7), -7},
		{"overflowing float", float64(1e300), 0},
		{"negative overflow", float64(-1e300), 0},
		{"nan", math.NaN(), 0},
		{"infinity", math.Inf(1), 0},
		{"min int64", float64(math.MinInt64), math.MinInt64},
		{"two to the sixty-three", float64(1) * math.MaxInt64, 0},
		{"go int", 7, 7},
		{"go int64", int64(1) << 40, int64(1) << 40},
		{"json number", json.Number("123"), 123},
		{"json number exceeds int64", json.Number("99999999999999999999"), 0},
		{"json number fraction", json.Number("1.5"), 0},
		{"string is not a number", "42", 0},
		{"absent", nil, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			object := map[string]any{"value": tc.value}
			if got := Int64At(object, "value"); got != tc.want {
				t.Fatalf("Int64At = %d, want %d", got, tc.want)
			}
		})
	}
}

// Int64At must not narrow through int: a 32-bit build would truncate every
// value above 2^31, including a Unix timestamp.
func TestInt64AtKeepsFullWidth(t *testing.T) {
	object := map[string]any{"created": float64(4380000000)}
	if got := Int64At(object, "created"); got != 4380000000 {
		t.Fatalf("Int64At = %d, want 4380000000", got)
	}
}
