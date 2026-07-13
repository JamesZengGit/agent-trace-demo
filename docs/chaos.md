# The orphan incident — reproduced

## What happened in production

At peak, roughly **87% of trace records were orphaned** — spans that agents
had emitted but that never reached storage. The system was healthy by every
usual signal: proxies up, collectors up, database writable.

The cause was connection affinity. Proxies held long-lived connections to a
specific collector **pod:port**. Kubernetes restarts pods routinely —
deploys, node maintenance, autoscaling — and every restart reassigned ports.
The proxies kept shipping into dead connections; everything in flight when a
connection died was stranded, and nothing noticed, because the observability
layer is the last thing anyone monitors.

The fix was **reattachment**: detect the dead connection, reconnect, and
replay everything the collector never acknowledged.

## How this replica reproduces it

The mechanism is faithful even though the environment is smaller:

- The proxy ships spans to the collector over a WebSocket. Every span
  carries a **sequence number**; the collector acknowledges a span only
  *after* the transport has durably accepted it.
- The proxy keeps every unacknowledged span in a **resend buffer**.
- `AT_REATTACH=on` (default): when the connection dies, the proxy
  reconnects and replays the buffer. At-least-once end to end; the
  processor deduplicates by span ID.
- `AT_REATTACH=off` (the original bug): the buffer dies with the
  connection, exactly like records pinned to a dead pod:port.

`make chaos` runs the same steady load twice — killing the collector
mid-run and restarting it seconds later — once in each mode, and samples
spans-reaching-storage every second.

## Measured result

2026-07-13, 300 req/s steady load for 60 s, collector killed at t=10s and
restarted at t=52s (a long outage relative to the window, like production):

| mode | spans sent | reached storage | orphaned |
|---|---:|---:|---:|
| reattach OFF | 17,950 | 6,819 | **62.0%** |
| reattach ON  | 17,936 | all of them | **0%** (converged after a ~25 s replay burst) |

![recovery graph](chaos/recovery.svg)

With reattachment off, everything sent during the outage — plus everything
sitting unacknowledged in the buffer at the moment of death — is gone, and
the loss is *silent*: the load generator saw its requests succeed, because
the proxy kept proxying perfectly. Capture loss is invisible to the traffic
being captured. That is what made the production incident so slow to
surface, and why the orphan rate got to 87% before anyone saw it.

With reattachment on, the storage line goes flat during the outage and then
climbs steeply — the replay burst — and converges back to the sent line.
Loss at the end: zero (any residual gap is spans still in flight when
sampling stopped).

## Lessons that carried into the design

1. **Ack after durability, not after receipt.** The collector acknowledges
   only once the transport has the span; anything less makes the resend
   buffer a lie.
2. **The observability layer is the last thing monitored.** Every component
   here exposes `/healthz` with real counters (accepted, rejected, stranded)
   rather than a bare 200 — the proxy's `stranded` counter is exactly the
   metric that would have caught the production incident in minutes instead
   of weeks.
3. **Losing the pipe must not lose the data.** Durability has to live one
   hop before the failure point.
