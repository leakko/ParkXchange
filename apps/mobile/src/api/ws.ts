import type { components } from "@parkxchange/api-contract";

import { issueWsTicket, type SpotFeature } from "@/api/client";
import { wsUrl } from "@/config";

export type BBox = readonly [number, number, number, number];

type ViewportMessage = components["schemas"]["ViewportMessage"];
type SnapshotMessage = components["schemas"]["SnapshotMessage"];
type SpotEventMessage = components["schemas"]["SpotEventMessage"];

type ServerMessage = SnapshotMessage | SpotEventMessage | { type: "error"; code?: string; message?: string };

export type SpotSocketHandlers = {
  onSnapshot: (features: SpotFeature[]) => void;
  onSpotEvent: (event: SpotEventMessage) => void;
  onStatus?: (status: "connecting" | "open" | "closed" | "error") => void;
};

/**
 * WebSocket client with exponential backoff reconnect and viewport
 * re-subscription. Auth is a short-lived ticket in the query string because
 * React Native cannot set headers on the upgrade.
 */
export class SpotSocket {
  private socket: WebSocket | null = null;
  private viewport: ViewportMessage | null = null;
  private closedByUser = false;
  private attempt = 0;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;

  constructor(private readonly handlers: SpotSocketHandlers) {}

  setViewport(next: Omit<ViewportMessage, "type">): void {
    this.viewport = { type: "viewport", ...next };
    if (this.socket?.readyState === WebSocket.OPEN) {
      this.socket.send(JSON.stringify(this.viewport));
    }
  }

  async connect(): Promise<void> {
    this.closedByUser = false;
    await this.open();
  }

  close(): void {
    this.closedByUser = true;
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    this.socket?.close();
    this.socket = null;
  }

  private async open(): Promise<void> {
    this.handlers.onStatus?.("connecting");
    const { ticket } = await issueWsTicket();
    const url = `${wsUrl}/v1/ws?ticket=${encodeURIComponent(ticket)}`;
    const socket = new WebSocket(url);
    this.socket = socket;

    socket.onopen = () => {
      this.attempt = 0;
      this.handlers.onStatus?.("open");
      if (this.viewport) {
        socket.send(JSON.stringify(this.viewport));
      }
    };

    socket.onmessage = (evt) => {
      let msg: ServerMessage;
      try {
        msg = JSON.parse(String(evt.data)) as ServerMessage;
      } catch {
        return;
      }
      if (msg.type === "snapshot") {
        this.handlers.onSnapshot(msg.features as SpotFeature[]);
        return;
      }
      if (
        msg.type === "spot.added" ||
        msg.type === "spot.updated" ||
        msg.type === "spot.removed" ||
        msg.type === "reservation.updated"
      ) {
        this.handlers.onSpotEvent(msg);
      }
    };

    socket.onerror = () => {
      this.handlers.onStatus?.("error");
    };

    socket.onclose = () => {
      this.handlers.onStatus?.("closed");
      this.socket = null;
      if (!this.closedByUser) {
        this.scheduleReconnect();
      }
    };
  }

  private scheduleReconnect(): void {
    const delay = Math.min(30_000, 500 * 2 ** this.attempt);
    this.attempt += 1;
    this.reconnectTimer = setTimeout(() => {
      void this.open().catch(() => this.scheduleReconnect());
    }, delay);
  }
}

/** Apply a realtime spot event onto a FeatureCollection's feature list. */
export function applySpotEvent(
  features: SpotFeature[],
  event: SpotEventMessage,
): SpotFeature[] {
  if (event.type === "spot.removed") {
    return features.filter((f) => String(f.id) !== event.id);
  }

  const existing = features.find((f) => String(f.id) === event.id);
  if (event.type === "spot.added" || event.type === "spot.updated") {
    const props = existing?.properties;
    const next: SpotFeature = {
      type: "Feature",
      id: event.id,
      geometry: { type: "Point", coordinates: [event.lon, event.lat] },
      properties: {
        owner_id: props?.owner_id ?? "",
        owner_name: props?.owner_name ?? "…",
        size_class: props?.size_class ?? "medium",
        status: event.status ?? props?.status ?? "available",
        price_cents: event.price_cents ?? props?.price_cents ?? 0,
        available_from: props?.available_from ?? new Date().toISOString(),
        expires_at: props?.expires_at ?? new Date().toISOString(),
        exact_location: event.exact_location,
        is_mine: props?.is_mine ?? false,
        ...(props?.owner_rating != null ? { owner_rating: props.owner_rating } : {}),
        ...(props?.address_hint != null ? { address_hint: props.address_hint } : {}),
        ...(props?.notes != null ? { notes: props.notes } : {}),
      },
    };
    if (existing) {
      return features.map((f) => (String(f.id) === event.id ? next : f));
    }
    return [...features, next];
  }

  return features;
}
