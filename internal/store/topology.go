package store

import (
	"context"
	"sort"
	"strings"
	"time"
)

// The topology is not configured anywhere — it is derived entirely from
// captured traffic. Every span already records which agent called which
// destination, so the fleet's network map is one aggregation query over the
// EAV detail table (destination/type/error/warnings rows self-joined by
// span_id, windowed via the summary table).

// TopologyAgent is one left-hand node of the map.
type TopologyAgent struct {
	ID    string `json:"id"`
	Calls int    `json:"calls"`
}

// TopologyBackend is one right-hand node: a destination the fleet talks to,
// grouped to stay readable (one LLM node, one database node, one node per
// tool, one per external host).
type TopologyBackend struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Kind  string `json:"kind"` // llm_call | tool_call | db_call | output | external
}

// TopologyEdge means "this agent has called this backend" (static
// connectivity), with aggregate counts for tooltips and error/warning marks.
type TopologyEdge struct {
	Agent    string `json:"agent"`
	Backend  string `json:"backend"`
	Calls    int    `json:"calls"`
	Errors   int    `json:"errors"`
	Warnings int    `json:"warnings"`
}

type Topology struct {
	Agents   []TopologyAgent   `json:"agents"`
	Backends []TopologyBackend `json:"backends"`
	Edges    []TopologyEdge    `json:"edges"`
}

// backendNode groups a raw destination into a stable map node.
func backendNode(spanType, destination string) (id, label string) {
	switch spanType {
	case "llm_call":
		return "llm", "LLM API"
	case "db_call":
		return "db", "Database"
	case "output":
		return "user", "User channel"
	case "tool_call":
		tool := destination
		if i := strings.LastIndex(destination, "/"); i >= 0 {
			tool = destination[i+1:]
		}
		if j := strings.IndexAny(tool, "?#"); j >= 0 {
			tool = tool[:j]
		}
		return "tool:" + tool, "Tool: " + tool
	default: // external / anything uncatalogued: group by host
		host := destination
		if i := strings.Index(destination, "/"); i >= 0 {
			host = destination[:i]
		}
		return "ext:" + host, "External: " + host
	}
}

// QueryTopology aggregates the observed agent -> backend connectivity for
// traces starting within [from, to]. limitAgents > 0 keeps only the busiest
// N agents (readability cap for large fleets); 0 means everyone.
func (s *Store) QueryTopology(ctx context.Context, from, to time.Time, limitAgents int) (*Topology, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT dd.agent_id,
       dd.detail_value                                   AS destination,
       COALESCE(dt.detail_value, '')                     AS span_type,
       COUNT(*)                                          AS calls,
       COUNT(*) FILTER (WHERE de.detail_value = 'true')  AS errors,
       COUNT(dw.id)                                      AS warnings
FROM trace_detail dd
JOIN trace_summary ts
  ON ts.trace_id = dd.trace_id AND ts.agent_id = dd.agent_id
LEFT JOIN trace_detail dt ON dt.span_id = dd.span_id AND dt.detail_name = 'type'
LEFT JOIN trace_detail de ON de.span_id = dd.span_id AND de.detail_name = 'error'
LEFT JOIN trace_detail dw ON dw.span_id = dd.span_id AND dw.detail_name = 'warnings'
WHERE dd.detail_name = 'destination'
  AND ts.start_time >= $1 AND ts.start_time <= $2
GROUP BY dd.agent_id, dd.detail_value, dt.detail_value`,
		from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	edges := map[[2]string]*TopologyEdge{}
	backends := map[string]TopologyBackend{}
	agentCalls := map[string]int{}
	for rows.Next() {
		var agent, dest, spanType string
		var calls, errs, warns int
		if err := rows.Scan(&agent, &dest, &spanType, &calls, &errs, &warns); err != nil {
			return nil, err
		}
		id, label := backendNode(spanType, dest)
		if _, ok := backends[id]; !ok {
			backends[id] = TopologyBackend{ID: id, Label: label, Kind: spanType}
		}
		key := [2]string{agent, id}
		e := edges[key]
		if e == nil {
			e = &TopologyEdge{Agent: agent, Backend: id}
			edges[key] = e
		}
		e.Calls += calls
		e.Errors += errs
		e.Warnings += warns
		agentCalls[agent] += calls
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	t := &Topology{}
	for id, calls := range agentCalls {
		t.Agents = append(t.Agents, TopologyAgent{ID: id, Calls: calls})
	}
	sort.Slice(t.Agents, func(i, j int) bool {
		if t.Agents[i].Calls != t.Agents[j].Calls {
			return t.Agents[i].Calls > t.Agents[j].Calls
		}
		return t.Agents[i].ID < t.Agents[j].ID
	})
	if limitAgents > 0 && len(t.Agents) > limitAgents {
		t.Agents = t.Agents[:limitAgents]
	}
	kept := map[string]bool{}
	for _, a := range t.Agents {
		kept[a.ID] = true
	}

	usedBackends := map[string]bool{}
	for _, e := range edges {
		if !kept[e.Agent] {
			continue
		}
		t.Edges = append(t.Edges, *e)
		usedBackends[e.Backend] = true
	}
	sort.Slice(t.Edges, func(i, j int) bool {
		if t.Edges[i].Agent != t.Edges[j].Agent {
			return t.Edges[i].Agent < t.Edges[j].Agent
		}
		return t.Edges[i].Backend < t.Edges[j].Backend
	})
	for id, b := range backends {
		if usedBackends[id] {
			t.Backends = append(t.Backends, b)
		}
	}
	sort.Slice(t.Backends, func(i, j int) bool {
		if t.Backends[i].Kind != t.Backends[j].Kind {
			return t.Backends[i].Kind < t.Backends[j].Kind
		}
		return t.Backends[i].ID < t.Backends[j].ID
	})
	return t, nil
}
