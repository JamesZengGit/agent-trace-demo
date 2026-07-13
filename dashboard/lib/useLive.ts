'use client';

// WebSocket live-edge hook: connects to /api/live, reconnects with
// exponential backoff, and invokes onReconnect so callers can re-fetch the
// focus window (catch-up) after a gap.

import { useEffect, useRef, useState } from 'react';
import { liveWsUrl, type LiveEvent } from './api';

export interface UseLiveOptions {
  onEvent: (ev: LiveEvent) => void;
  /** Called after any successful (re)connect that follows a drop. */
  onReconnect?: () => void;
  enabled?: boolean;
}

export function useLive({ onEvent, onReconnect, enabled = true }: UseLiveOptions): {
  connected: boolean;
} {
  const [connected, setConnected] = useState(false);

  // Keep latest callbacks without re-opening the socket.
  const onEventRef = useRef(onEvent);
  const onReconnectRef = useRef(onReconnect);
  onEventRef.current = onEvent;
  onReconnectRef.current = onReconnect;

  useEffect(() => {
    if (!enabled) return;

    let ws: WebSocket | null = null;
    let closed = false;
    let attempt = 0;
    let hadDrop = false;
    let timer: ReturnType<typeof setTimeout> | null = null;

    const connect = () => {
      if (closed) return;
      try {
        ws = new WebSocket(liveWsUrl());
      } catch {
        scheduleRetry();
        return;
      }

      ws.onopen = () => {
        attempt = 0;
        setConnected(true);
        if (hadDrop) {
          hadDrop = false;
          onReconnectRef.current?.();
        }
      };

      ws.onmessage = (msg) => {
        try {
          const ev = JSON.parse(msg.data as string) as LiveEvent;
          if (ev && (ev.type === 'trace_upsert' || ev.type === 'span')) {
            onEventRef.current(ev);
          }
        } catch {
          // ignore malformed frames
        }
      };

      ws.onclose = () => {
        setConnected(false);
        hadDrop = true;
        scheduleRetry();
      };

      ws.onerror = () => {
        // onclose follows; nothing to do here.
      };
    };

    const scheduleRetry = () => {
      if (closed) return;
      const delay = Math.min(500 * 2 ** attempt, 15_000);
      attempt += 1;
      timer = setTimeout(connect, delay);
    };

    connect();

    return () => {
      closed = true;
      if (timer) clearTimeout(timer);
      if (ws) {
        ws.onclose = null;
        ws.close();
      }
      setConnected(false);
    };
  }, [enabled]);

  return { connected };
}
