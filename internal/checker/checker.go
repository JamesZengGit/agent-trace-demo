// Package checker is the model-checker stand-in: it scans captured LLM
// messages for prompt injection and data leakage and raises warnings.
//
// STAND-IN NOTICE: production used a separate model-based scanning system.
// This replica uses a small set of deterministic patterns so runs are
// offline, free and repeatable — just enough for the concept to be legible.
// It is intentionally not a main character.
package checker

import (
	"fmt"
	"regexp"

	"agenttrace/internal/model"
)

type rule struct {
	name string
	kind string // "prompt_injection" | "data_leakage"
	re   *regexp.Regexp
}

var rules = []rule{
	{"instruction_override", "prompt_injection",
		regexp.MustCompile(`(?i)ignore (all )?(previous|prior|above) (instructions|rules)`)},
	{"role_hijack", "prompt_injection",
		regexp.MustCompile(`(?i)(you are now|pretend to be|act as) (an? )?(unrestricted|jailbroken|developer mode)`)},
	{"permission_escalation", "prompt_injection",
		regexp.MustCompile(`(?i)(grant|give|escalate).{0,20}(admin|root|full) (access|privileges?)`)},
	{"exfil_instruction", "prompt_injection",
		regexp.MustCompile(`(?i)(send|post|upload|forward) (all |the )?(data|records|results|contents?) to (http|ftp|[a-z0-9.-]+\.(com|net|io|ru))`)},
	{"aws_key", "data_leakage", regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
	{"api_key", "data_leakage", regexp.MustCompile(`(?i)(api[_-]?key|secret[_-]?key|bearer)\s*[:=]\s*[A-Za-z0-9_\-]{16,}`)},
	{"ssn", "data_leakage", regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`)},
	{"card_number", "data_leakage", regexp.MustCompile(`\b(?:\d[ -]?){15,16}\b`)},
}

// Scan inspects the LLM-visible text of a span (prompts, response, bodies)
// and returns model-checker warnings.
func Scan(s *model.Span) []model.Warning {
	var out []model.Warning
	fields := []struct{ where, text string }{
		{"system_prompt", s.SystemPrompt},
		{"user_prompt", s.UserPrompt},
		{"response", s.Response},
		{"request_body", s.RequestBody},
	}
	seen := map[string]bool{}
	for _, f := range fields {
		if f.text == "" {
			continue
		}
		for _, r := range rules {
			if seen[r.name] || !r.re.MatchString(f.text) {
				continue
			}
			seen[r.name] = true
			out = append(out, model.Warning{
				Source: model.WarnModelChecker,
				Rule:   r.name,
				Reason: fmt.Sprintf("%s pattern %q matched in %s", r.kind, r.name, f.where),
			})
		}
	}
	return out
}
