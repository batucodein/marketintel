"use client";

import { useEffect, useRef } from "react";
import { getAccessToken } from "@/lib/api/client";

const API_URL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8000";

export interface OutreachEvent {
  kind: "inbound" | "conversation_read" | "campaign_progress" | "simulation_progress";
  conversation_id?: string;
  campaign_id?: string;
  at: string;
  data?: Record<string, unknown>;
}

// useOutreachEvents opens a Server-Sent-Events stream to /outreach/events
// and invokes onEvent for every push. EventSource auto-reconnects on
// transient errors; we only have to manage the lifecycle of the open stream.
//
// Auth: EventSource cannot set custom headers, so the JWT goes in the
// access_token query param (the auth middleware accepts both forms).
export function useOutreachEvents(onEvent: (e: OutreachEvent) => void) {
  // Hold onEvent in a ref so the EventSource doesn't churn when the
  // caller passes a fresh closure each render.
  const cbRef = useRef(onEvent);
  useEffect(() => {
    cbRef.current = onEvent;
  }, [onEvent]);

  useEffect(() => {
    const token = getAccessToken();
    if (!token) return;

    const url = `${API_URL}/outreach/events?access_token=${encodeURIComponent(token)}`;
    let es: EventSource | null = new EventSource(url);

    function handle(e: MessageEvent) {
      try {
        const payload = JSON.parse(e.data) as OutreachEvent;
        cbRef.current(payload);
      } catch {
        // ignore malformed events
      }
    }

    es.addEventListener("inbound", handle);
    es.addEventListener("conversation_read", handle);
    es.addEventListener("campaign_progress", handle);
    es.addEventListener("simulation_progress", handle);

    es.onerror = () => {
      // Browser will retry automatically. Nothing to do.
    };

    return () => {
      if (es) {
        es.close();
        es = null;
      }
    };
  }, []);
}
