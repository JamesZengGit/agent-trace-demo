// loadgen drives the real capture path for benchmarks: concurrent workers
// send LLM-shaped requests through the proxy at a target rate. It measures
// what the client actually achieved (sent, failed, proxy round-trip
// latencies) and prints machine-readable results; the bench script pairs
// them with what reached storage.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	var (
		proxyAddr = flag.String("proxy", envOr("PROXY_URL", "http://localhost:8080"), "capture proxy URL")
		target    = flag.String("target", envOr("MOCKSVC_URL", "http://localhost:9100"), "upstream base URL")
		rps       = flag.Int("rps", 200, "target requests/sec")
		duration  = flag.Duration("duration", 30*time.Second, "test duration")
		workers   = flag.Int("workers", 32, "concurrent workers")
		agentID   = flag.String("agent", "loadgen", "agent id for the spans")
		mode      = flag.String("mode", "db", "request shape: db (cheap) or llm (mock model latency)")
	)
	flag.Parse()

	proxyURL, err := url.Parse(*proxyAddr)
	if err != nil {
		log.Fatal(err)
	}
	client := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			Proxy:               http.ProxyURL(proxyURL),
			MaxIdleConnsPerHost: *workers * 2,
		},
	}

	llmPayload, _ := json.Marshal(map[string]any{
		"model":    "mock-small-1",
		"messages": []map[string]string{{"role": "user", "content": "benchmark ping"}},
	})

	var sent, failed atomic.Uint64
	var mu sync.Mutex
	var latencies []float64

	tick := time.NewTicker(time.Second / time.Duration(*rps))
	defer tick.Stop()
	deadline := time.After(*duration)
	jobs := make(chan struct{}, *workers*4)

	var wg sync.WaitGroup
	for range *workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobs {
				var req *http.Request
				if *mode == "llm" {
					req, _ = http.NewRequest("POST", *target+"/v1/chat/completions", bytes.NewReader(llmPayload))
				} else {
					req, _ = http.NewRequest("GET", *target+"/db/bench-read", nil)
				}
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Agent-ID", *agentID)
				t0 := time.Now()
				resp, err := client.Do(req)
				ms := float64(time.Since(t0).Microseconds()) / 1000
				if err != nil {
					failed.Add(1)
					continue
				}
				resp.Body.Close()
				sent.Add(1)
				mu.Lock()
				latencies = append(latencies, ms)
				mu.Unlock()
			}
		}()
	}

	start := time.Now()
loop:
	for {
		select {
		case <-deadline:
			break loop
		case <-tick.C:
			select {
			case jobs <- struct{}{}:
			default:
				// workers saturated: request skipped, counted as backpressure
				failed.Add(1)
			}
		}
	}
	close(jobs)
	wg.Wait()
	elapsed := time.Since(start).Seconds()

	sort.Float64s(latencies)
	pct := func(p float64) float64 {
		if len(latencies) == 0 {
			return 0
		}
		i := int(p * float64(len(latencies)-1))
		return latencies[i]
	}
	out := map[string]any{
		"mode": *mode, "target_rps": *rps, "duration_s": elapsed,
		"sent": sent.Load(), "failed_or_skipped": failed.Load(),
		"achieved_rps": float64(sent.Load()) / elapsed,
		"latency_ms":   map[string]float64{"p50": pct(0.50), "p95": pct(0.95), "p99": pct(0.99)},
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(out)
	fmt.Fprintf(os.Stderr, "loadgen done: %d sent in %.1fs (%.0f rps achieved)\n",
		sent.Load(), elapsed, float64(sent.Load())/elapsed)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
