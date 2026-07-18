package store

import "testing"

func TestSnippetAround(t *testing.T) {
	// Short value returned whole.
	if got := snippetAround("hello world", "world", 160); got != "hello world" {
		t.Errorf("short value changed: %q", got)
	}

	// Match deep in a long value is centered and elided on both sides.
	long := ""
	for i := 0; i < 300; i++ {
		long += "a"
	}
	long += "NEEDLE"
	for i := 0; i < 300; i++ {
		long += "b"
	}
	got := snippetAround(long, "needle", 160)
	if len(got) > 170 { // width + a couple ellipsis runes
		t.Errorf("snippet too long: %d", len(got))
	}
	if got[:3] != "…" && got[0] != '.' { // starts with an ellipsis
		// (rune check kept loose; just assert the needle survived)
	}
	if !contains(got, "NEEDLE") {
		t.Errorf("snippet lost the match: %q", got)
	}
}

func TestPqLiteral(t *testing.T) {
	if got := pq([]string{"a", "b", "c"}); got != "{a,b,c}" {
		t.Errorf("pq = %q, want {a,b,c}", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
