import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";

import { fetchSpots, type SpotFeature } from "@/api/client";
import { applySpotEvent, SpotSocket, type BBox } from "@/api/ws";

const EMPTY: SpotFeature[] = [];

export type Viewport = {
  bbox: BBox;
  zoom: number;
  from: string;
  to: string;
};

/** Default discovery window: now → +2h (matches the advance-booking horizon UX). */
export function defaultTimeWindow(now = new Date()): { from: string; to: string } {
  const from = now.toISOString();
  const to = new Date(now.getTime() + 2 * 60 * 60 * 1000).toISOString();
  return { from, to };
}

export function useDiscovery(viewport: Viewport | null, socketEnabled: boolean) {
  const [liveFeatures, setLiveFeatures] = useState<SpotFeature[] | null>(null);
  const socketRef = useRef<SpotSocket | null>(null);

  const query = useQuery({
    queryKey: ["spots", viewport],
    enabled: viewport != null,
    queryFn: () =>
      fetchSpots({
        bbox: viewport!.bbox,
        zoom: viewport!.zoom,
        from: viewport!.from,
        to: viewport!.to,
      }),
    staleTime: 15_000,
  });

  // REST snapshot wins whenever it refreshes; WS patches apply on top until then.
  useEffect(() => {
    if (query.data) {
      setLiveFeatures(query.data.features);
    }
  }, [query.data]);

  useEffect(() => {
    if (!socketEnabled) {
      return;
    }
    const socket = new SpotSocket({
      onSnapshot: (features) => setLiveFeatures(features),
      onSpotEvent: (event) => {
        setLiveFeatures((prev) => applySpotEvent(prev ?? EMPTY, event));
      },
    });
    socketRef.current = socket;
    void socket.connect().catch(() => {
      /* reconnect handles subsequent attempts */
    });
    return () => {
      socket.close();
      socketRef.current = null;
    };
  }, [socketEnabled]);

  useEffect(() => {
    if (!viewport || !socketRef.current) {
      return;
    }
    socketRef.current.setViewport({
      bbox: [...viewport.bbox],
      zoom: viewport.zoom,
      from: viewport.from,
      to: viewport.to,
    });
  }, [viewport]);

  const features = liveFeatures ?? query.data?.features ?? EMPTY;

  const collection = useMemo(
    () => ({ type: "FeatureCollection" as const, features }),
    [features],
  );

  const featureById = useCallback(
    (id: string) => features.find((f) => String(f.id) === id) ?? null,
    [features],
  );

  return {
    collection,
    featureById,
    isLoading: query.isLoading && liveFeatures == null,
    error: query.error,
    refetch: query.refetch,
  };
}
