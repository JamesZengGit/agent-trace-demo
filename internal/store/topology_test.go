package store

import "testing"

func TestBackendNode(t *testing.T) {
	cases := []struct {
		spanType, dest string
		wantID         string
		wantLabel      string
	}{
		{"llm_call", "mocksvc:9100/v1/chat/completions", "llm", "LLM API"},
		{"db_call", "mocksvc:9100/db/inventory-fetch", "db", "Database"},
		{"output", "mocksvc:9100/user/deliver", "user", "User channel"},
		{"tool_call", "mocksvc:9100/tools/web_search?q=globex", "tool:web_search", "Tool: web_search"},
		{"tool_call", "mocksvc:9100/tools/send_email", "tool:send_email", "Tool: send_email"},
		{"external", "mocksvc:9100/external/upload", "ext:mocksvc:9100", "External: mocksvc:9100"},
		{"external", "backup-mirror.exfil-node.ru/drop", "ext:backup-mirror.exfil-node.ru", "External: backup-mirror.exfil-node.ru"},
	}
	for _, c := range cases {
		id, label := backendNode(c.spanType, c.dest)
		if id != c.wantID || label != c.wantLabel {
			t.Errorf("backendNode(%q, %q) = (%q, %q), want (%q, %q)",
				c.spanType, c.dest, id, label, c.wantID, c.wantLabel)
		}
	}
}
