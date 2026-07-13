package policy

import "testing"

func TestMatch(t *testing.T) {
	cases := []struct {
		pattern, dest string
		want          bool
	}{
		{"*/v1/chat/completions", "mocksvc:9100/v1/chat/completions", true},
		{"*/db/tickets-*", "mocksvc:9100/db/tickets-fetch", true},
		{"*/db/tickets-*", "mocksvc:9100/db/spend-records", false},
		{"*/tools/web_search", "mocksvc:9100/tools/web_search", true},
		{"*/tools/web_search", "mocksvc:9100/tools/send_email", false},
		{"*/db/inventory-*", "mocksvc:9100/external/upload", false},
		{"exact", "exact", true},
		{"exact", "exactly-not", false},
		{"*", "anything/at/all", true},
	}
	for _, c := range cases {
		if got := match(c.pattern, c.dest); got != c.want {
			t.Errorf("match(%q, %q) = %v, want %v", c.pattern, c.dest, got, c.want)
		}
	}
}

func TestCheckAgentGlobs(t *testing.T) {
	e := &Engine{cfg: Config{Rules: []Rule{
		{Agent: "*", Allow: []string{"*/v1/chat/completions"}},
		{Agent: "bench-*", Allow: []string{"*/db/bench-*"}},
	}}}
	if w := e.Check("bench-20260713-100", "mocksvc:9100/db/bench-read"); w != nil {
		t.Errorf("bench agent should be allowed its bench destination, got warning %+v", w)
	}
	if w := e.Check("bench-20260713-100", "mocksvc:9100/external/upload"); w == nil {
		t.Error("bench agent must still warn on uncatalogued destinations")
	}
	if w := e.Check("anyone", "mocksvc:9100/v1/chat/completions"); w != nil {
		t.Errorf("wildcard LLM rule should apply to every agent, got %+v", w)
	}
}
