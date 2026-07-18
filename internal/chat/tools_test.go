package chat

import (
	"testing"
	"time"
)

func TestParseSince(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"", 24 * time.Hour},        // default
		{"15m", 15 * time.Minute},
		{"1h", time.Hour},
		{"24h", 24 * time.Hour},
		{"7d", 7 * 24 * time.Hour},
		{"2D", 2 * 24 * time.Hour},  // case-insensitive
		{"garbage", 24 * time.Hour}, // fallback
		{"0", 24 * time.Hour},       // non-positive -> default
		{"-5m", 24 * time.Hour},     // negative -> default
	}
	for _, c := range cases {
		if got := parseSince(c.in); got != c.want {
			t.Errorf("parseSince(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestArgHelpers(t *testing.T) {
	// The model sends loosely-typed JSON; numbers arrive as float64, and some
	// models send booleans/ints as strings.
	args := map[string]any{
		"agent_id":    "inventory-sync",
		"only_errors": true,
		"only_warn_s": "true",
		"limit_f":     float64(42),
		"limit_s":     "17",
	}
	if got := strArg(args, "agent_id"); got != "inventory-sync" {
		t.Errorf("strArg = %q", got)
	}
	if !boolArg(args, "only_errors") || !boolArg(args, "only_warn_s") {
		t.Error("boolArg should accept bool and \"true\"")
	}
	if got := intArg(args, "limit_f", 0); got != 42 {
		t.Errorf("intArg(float64) = %d", got)
	}
	if got := intArg(args, "limit_s", 0); got != 17 {
		t.Errorf("intArg(string) = %d", got)
	}
	if got := intArg(args, "missing", 5); got != 5 {
		t.Errorf("intArg(missing) = %d, want default 5", got)
	}
}

func TestClampInt(t *testing.T) {
	if clampInt(0, 1, 200) != 1 || clampInt(999, 1, 200) != 200 || clampInt(50, 1, 200) != 50 {
		t.Error("clampInt bounds wrong")
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 800); got != "short" {
		t.Errorf("truncate short changed it: %q", got)
	}
	long := make([]byte, 900)
	for i := range long {
		long[i] = 'x'
	}
	got := truncate(string(long), 800)
	if len(got) <= 800 || got[:800] != string(long[:800]) {
		t.Error("truncate should keep first 800 chars + marker")
	}
}
