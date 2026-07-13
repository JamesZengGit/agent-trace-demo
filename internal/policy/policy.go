// Package policy is the access-restriction engine: which agents may call
// which destinations. It is deliberately minimal — a config file and one check
// in the capture path. In the production platform this is a full contextual-
// boundary system; here it is a stand-in with just enough behavior to power
// the misbehaving-agent detection. Violations produce warnings, not blocks:
// the trace layer observes and flags, enforcement lives in the big system.
package policy

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"agenttrace/internal/model"
)

// Rule allows an agent (or "*") to reach destination patterns. A destination
// is host/path; patterns support a trailing "*" wildcard.
type Rule struct {
	Agent string   `yaml:"agent"`
	Allow []string `yaml:"allow"`
}

type Config struct {
	Rules []Rule `yaml:"rules"`
}

type Engine struct {
	cfg Config
}

func Load(file string) (*Engine, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("read policy: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("parse policy: %w", err)
	}
	return &Engine{cfg: cfg}, nil
}

// Check returns a policy warning if agentID is not allowed to reach dest,
// and nil if the call is within policy.
func (e *Engine) Check(agentID, dest string) *model.Warning {
	for _, r := range e.cfg.Rules {
		if !match(r.Agent, agentID) {
			continue
		}
		for _, pat := range r.Allow {
			if match(pat, dest) {
				return nil
			}
		}
	}
	return &model.Warning{
		Source: model.WarnPolicyEngine,
		Rule:   "destination_not_allowed",
		Reason: fmt.Sprintf("agent %q called %q, which is outside its allowed destinations", agentID, dest),
	}
}

// match supports "*" wildcards anywhere in the pattern: the literal chunks
// must appear in order, anchored at the ends unless a "*" sits there.
func match(pattern, dest string) bool {
	chunks := strings.Split(pattern, "*")
	if !strings.HasPrefix(pattern, "*") {
		if !strings.HasPrefix(dest, chunks[0]) {
			return false
		}
	}
	if !strings.HasSuffix(pattern, "*") {
		if !strings.HasSuffix(dest, chunks[len(chunks)-1]) {
			return false
		}
	}
	rest := dest
	for _, c := range chunks {
		if c == "" {
			continue
		}
		i := strings.Index(rest, c)
		if i < 0 {
			return false
		}
		rest = rest[i+len(c):]
	}
	return true
}
