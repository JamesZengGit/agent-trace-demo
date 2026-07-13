// mocksvc is the deterministic stand-in environment the fleet talks to:
// an OpenAI-shaped mock LLM, a handful of tools, a database endpoint, the
// user delivery channel, and an external sink (the misbehaving agent's
// destination). Deterministic and offline so benchmarks are free and
// repeatable. Fake the LLM, never the capture.
//
// Failure modes are driven by directives embedded in the user prompt, so the
// fleet can script them: [[fail:500]] [[refuse]] [[overload]] [[cutoff]]
// [[timeout]] [[slow:1500]].
package main

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int         `json:"index"`
		Message      chatMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

var directiveRe = regexp.MustCompile(`\[\[(fail|refuse|overload|cutoff|timeout|slow):?(\d*)\]\]`)

// jitter derives a stable pseudo-latency from content, so runs are repeatable
// but the heatmap still shows a spread.
func jitter(seed string, baseMS, spreadMS int) time.Duration {
	h := fnv.New32a()
	h.Write([]byte(seed))
	return time.Duration(baseMS+int(h.Sum32())%spreadMS) * time.Millisecond
}

func tokens(s string) int { return len(s)/4 + 1 }

func chatHandler(w http.ResponseWriter, r *http.Request) {
	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"bad request"}`, 400)
		return
	}
	var system, user string
	for _, m := range req.Messages {
		switch m.Role {
		case "system":
			system = m.Content
		case "user":
			user = m.Content
		}
	}

	directive, arg := "", 0
	if m := directiveRe.FindStringSubmatch(user); m != nil {
		directive = m[1]
		arg, _ = strconv.Atoi(m[2])
	}

	switch directive {
	case "timeout":
		time.Sleep(30 * time.Second) // longer than any client timeout
		return
	case "slow":
		if arg == 0 {
			arg = 1500
		}
		time.Sleep(time.Duration(arg) * time.Millisecond)
	default:
		time.Sleep(jitter(user, 180, 900))
	}

	if directive == "fail" {
		if arg == 0 {
			arg = 500
		}
		http.Error(w, `{"error":{"type":"server_error","message":"internal model error"}}`, arg)
		return
	}

	reply := generate(system, user)
	finish := "stop"
	switch directive {
	case "refuse":
		reply = "I can't comply with that request. It appears to conflict with my usage guidelines."
	case "overload":
		reply = "The model is currently overloaded with other requests. Please retry your request later."
	case "cutoff":
		if len(reply) > 40 {
			reply = reply[:40]
		}
		reply += " — and the next step would be to"
		finish = "length"
	}

	var resp chatResponse
	resp.ID = fmt.Sprintf("chatcmpl-mock-%d", time.Now().UnixNano())
	resp.Object = "chat.completion"
	resp.Model = req.Model
	resp.Choices = append(resp.Choices, struct {
		Index        int         `json:"index"`
		Message      chatMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	}{0, chatMessage{"assistant", reply}, finish})
	resp.Usage.PromptTokens = tokens(system) + tokens(user)
	resp.Usage.CompletionTokens = tokens(reply)
	resp.Usage.TotalTokens = resp.Usage.PromptTokens + resp.Usage.CompletionTokens
	writeJSON(w, resp)
}

// generate produces a deterministic, mission-flavored answer.
func generate(system, user string) string {
	u := strings.ToLower(user)
	switch {
	case strings.Contains(u, "classif") || strings.Contains(u, "triage"):
		return "Classification: PRIORITY-2 (billing). The ticket describes a duplicated charge after a plan upgrade. Route to the billing queue; suggested first response drafted below.\n\nDraft: Thanks for flagging this — I can see the duplicate charge and have opened a refund review."
	case strings.Contains(u, "plan") || strings.Contains(u, "which step"):
		return "Plan: (1) search recent coverage for the account's industry, (2) search funding announcements, (3) synthesize a one-page brief with three talking points. Proceeding with step 1."
	case strings.Contains(u, "summar") || strings.Contains(u, "report"):
		return "Summary: Two of the three sources point to expansion in the APAC region; revenue guidance was raised 8% quarter over quarter. Recommended talking points: regional hiring, the new platform tier, and the migration case study."
	case strings.Contains(u, "anomal") || strings.Contains(u, "audit") || strings.Contains(u, "spend"):
		return "Audit result: spend is within 4% of forecast. One anomaly: storage costs in project atlas-7 rose 22% week over week — likely an unfinished lifecycle rule. Flagged for review."
	default:
		return "Acknowledged. Based on the provided context, the requested operation completed and the relevant records were updated. No further action required."
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func main() {
	addr := os.Getenv("MOCKSVC_ADDR")
	if addr == "" {
		addr = ":9100"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/chat/completions", chatHandler)

	mux.HandleFunc("/tools/", func(w http.ResponseWriter, r *http.Request) {
		tool := strings.TrimPrefix(r.URL.Path, "/tools/")
		time.Sleep(jitter(tool+r.URL.RawQuery, 60, 400))
		writeJSON(w, map[string]any{
			"tool": tool, "status": "ok",
			"result": fmt.Sprintf("mock result for tool %q", tool),
		})
	})

	mux.HandleFunc("/db/", func(w http.ResponseWriter, r *http.Request) {
		op := strings.TrimPrefix(r.URL.Path, "/db/")
		time.Sleep(jitter(op, 20, 150))
		writeJSON(w, map[string]any{
			"op": op, "rows": []map[string]any{
				{"id": 4211, "account": "acme-industrial", "value": "record-a"},
				{"id": 4212, "account": "globex", "value": "record-b"},
			},
		})
	})

	mux.HandleFunc("/user/deliver", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(jitter("deliver", 10, 60))
		writeJSON(w, map[string]any{"delivered": true})
	})

	// The uncatalogued destination the misbehaving agent talks to.
	mux.HandleFunc("/external/", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(jitter("external", 80, 300))
		writeJSON(w, map[string]any{"received": true})
	})

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })

	log.Printf("mocksvc listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
