// Package redact is the storage-hygiene hook (labeled 2026 improvement, not
// in the original article): captured payloads pass through here before they
// are written to PostgreSQL, so secrets and PII never land on disk. The
// checker sees the raw text first — detection runs before masking.
package redact

import (
	"regexp"

	"agenttrace/internal/model"
)

var patterns = []struct {
	re   *regexp.Regexp
	mask string
}{
	{regexp.MustCompile(`AKIA[0-9A-Z]{16}`), "[REDACTED:aws_key]"},
	{regexp.MustCompile(`(?i)(api[_-]?key|secret[_-]?key|bearer)(\s*[:=]\s*)[A-Za-z0-9_\-]{16,}`), "$1$2[REDACTED:secret]"},
	{regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`), "[REDACTED:ssn]"},
	{regexp.MustCompile(`\b(?:\d[ -]?){15,16}\b`), "[REDACTED:card]"},
	{regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`), "[REDACTED:email]"},
}

func text(s string) string {
	for _, p := range patterns {
		s = p.re.ReplaceAllString(s, p.mask)
	}
	return s
}

// Span masks sensitive content in all captured payload fields, in place.
func Span(s *model.Span) {
	s.SystemPrompt = text(s.SystemPrompt)
	s.UserPrompt = text(s.UserPrompt)
	s.Response = text(s.Response)
	s.RequestBody = text(s.RequestBody)
	s.ResponseBody = text(s.ResponseBody)
}
