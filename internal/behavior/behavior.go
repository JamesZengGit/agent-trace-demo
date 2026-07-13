// Package behavior builds the decision tree of a closed trace: behavior ->
// sub-behavior -> span leaves. A decision tree represents a mission; a
// mission represents an agent behavior.
//
// STAND-IN NOTICE: production used a small model that reads span content and
// groups spans under behavior labels. This replica uses a deterministic
// labeler behind the same interface, so runs are offline and repeatable.
package behavior

import (
	"fmt"
	"strings"

	"agenttrace/internal/model"
)

// Labeler groups a closed trace's spans into a behavior tree.
type Labeler interface {
	Label(spans []*model.Span) *model.BehaviorNode
}

// Deterministic is the offline stand-in labeler.
type Deterministic struct{}

// Label assigns each span a behavior/sub-behavior from its type and content,
// then folds consecutive same-behavior spans into shared branches.
func (Deterministic) Label(spans []*model.Span) *model.BehaviorNode {
	if len(spans) == 0 {
		return nil
	}
	root := &model.BehaviorNode{Label: missionPurpose(spans), Kind: "behavior"}
	var current *model.BehaviorNode
	for _, s := range spans {
		beh, sub := classify(s)
		s.Behavior, s.SubBehavior = beh, sub
		if current == nil || current.Label != beh {
			current = &model.BehaviorNode{Label: beh, Kind: "behavior"}
			root.Children = append(root.Children, current)
		}
		leafParent := current
		if sub != "" {
			var subNode *model.BehaviorNode
			for _, c := range current.Children {
				if c.Kind == "sub_behavior" && c.Label == sub {
					subNode = c
					break
				}
			}
			if subNode == nil {
				subNode = &model.BehaviorNode{Label: sub, Kind: "sub_behavior"}
				current.Children = append(current.Children, subNode)
			}
			leafParent = subNode
		}
		leaf := &model.BehaviorNode{
			Label:   spanLeafLabel(s),
			Kind:    "span",
			SpanID:  s.SpanID,
			Error:   s.Error,
			Warning: len(s.Warnings) > 0,
		}
		leafParent.Children = append(leafParent.Children, leaf)
	}
	propagateFlags(root)
	return root
}

// classify maps one span to (behavior, sub-behavior) labels.
func classify(s *model.Span) (string, string) {
	switch s.Type {
	case model.SpanLLMCall:
		p := strings.ToLower(s.UserPrompt + " " + s.SystemPrompt)
		switch {
		case strings.Contains(p, "plan") || strings.Contains(p, "decide") || strings.Contains(p, "which step"):
			return "Planning", "Task decomposition"
		case strings.Contains(p, "summar") || strings.Contains(p, "report"):
			return "Synthesis", "Summarization"
		case strings.Contains(p, "classif") || strings.Contains(p, "triage") || strings.Contains(p, "categor"):
			return "Analysis", "Classification"
		default:
			return "Reasoning", "LLM consultation"
		}
	case model.SpanToolCall:
		tool := lastPathSegment(s.Destination)
		return "Tool use", "Tool: " + tool
	case model.SpanDBCall:
		return "Data access", "Database query"
	case model.SpanOutput:
		return "Responding", "Deliver result to user"
	case model.SpanExternal:
		return "External communication", "Uncatalogued destination"
	}
	return "Activity", ""
}

// missionPurpose picks the trace-level label — the trunk of the tree.
func missionPurpose(spans []*model.Span) string {
	for _, s := range spans {
		if s.Type == model.SpanLLMCall && s.SystemPrompt != "" {
			line := strings.SplitN(strings.TrimSpace(s.SystemPrompt), "\n", 2)[0]
			if len(line) > 80 {
				line = line[:77] + "..."
			}
			return "Mission: " + line
		}
	}
	return "Mission: " + spans[0].AgentID
}

func spanLeafLabel(s *model.Span) string {
	return fmt.Sprintf("%s %s (%.0fms)", s.Method, s.Destination, s.DurationMS())
}

func lastPathSegment(dest string) string {
	parts := strings.Split(strings.TrimSuffix(dest, "/"), "/")
	return parts[len(parts)-1]
}

// propagateFlags bubbles error/warning marks from leaves to branches so the
// tree shows where trouble lives at every level.
func propagateFlags(n *model.BehaviorNode) (bool, bool) {
	e, w := n.Error, n.Warning
	for _, c := range n.Children {
		ce, cw := propagateFlags(c)
		e, w = e || ce, w || cw
	}
	n.Error, n.Warning = e, w
	return e, w
}
