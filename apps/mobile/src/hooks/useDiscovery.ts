import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";

import { fetchSpots, type SpotFeature } from "@/api/client";
import { applySpotEvent, SpotSocket, type BBox } from "@/api/ws";
import { SpotTombstones } from "@/map/spotTombstones";
import type { components } from "@parkxchange/api-contract";

const EMPTY: SpotFeature[] = [];

/** Debounce REST refetch after WS events that lack a real vehicle summary. */
const VEHICLE_REFETCH_MS = 400;

type SpotEventMessage = components["schemas"]["SpotEventMessage"];

export type Viewport = {
  bbox: BBox;
  zoom: number;
  from: string;
  to: string;
  includeFlexible: boolean;
};

function vehicleIncomplete(feature: SpotFeature | undefined): boolean {
  if (!feature?.properties.exact_location) {
    // Pre-reservation payloads omit the car on purpose; do not refetch forever.
    return false;
  }
  const v = feature.properties.vehicle;
  return !v?.id || !v.plate;
}

function eventNeedsVehicleRefetch(event: SpotEventMessage, features: SpotFeature[]): boolean {
  if (event.type !== "spot.added" && event.type !== "spot.updated") {
    return false;
  }
  const feature = features.find((f) => String(f.id) === event.id);
  return vehicleIncomplete(feature);
}

export function useDiscovery(viewport: Viewport | null, socketEnabled: boolean) {
  const [liveFeatures, setLiveFeatures] = useState<SpotFeature[] | null>(null);
  const socketRef = useRef<SpotSocket | null>(null);
  const refetchTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const refetchRef = useRef<() => void>(() => {});
  const tombstonesRef = useRef(new SpotTombstones());

  const query = useQuery({
    queryKey: ["spots", viewport],
    enabled: viewport != null,
    queryFn: () =>
      fetchSpots({
        bbox: viewport!.bbox,
        zoom: viewport!.zoom,
        from: viewport!.from,
        to: viewport!.to,
        includeFlexible: viewport!.includeFlexible,
      }),
    staleTime: 15_000,
  });

  refetchRef.current = () => {
    void query.refetch();
  };

  const scheduleVehicleRefetch = useCallback(() => {
    if (refetchTimerRef.current) {
      clearTimeout(refetchTimerRef.current);
    }
    refetchTimerRef.current = setTimeout(() => {
      refetchTimerRef.current = null;
      refetchRef.current();
    }, VEHICLE_REFETCH_MS);
  }, []);

  // REST snapshot wins whenever it refreshes, but never resurrect tombstoned ids
  // (stale in-flight fetch right after accept → spot.removed).
  useEffect(() => {
    if (query.data) {
      setLiveFeatures(tombstonesRef.current.filter(query.data.features));
    }
  }, [query.data]);

  useEffect(() => {
    if (!socketEnabled) {
      return;
    }
    const socket = new SpotSocket({
      onSnapshot: (features) =>
        setLiveFeatures(tombstonesRef.current.filter(features)),
      onSpotEvent: (event) => {
        tombstonesRef.current.noteEvent(event);
        setLiveFeatures((prev) => {
          const next = applySpotEvent(prev ?? EMPTY, event);
          if (eventNeedsVehicleRefetch(event, next)) {
            scheduleVehicleRefetch();
          }
          return tombstonesRef.current.filter(next);
        });
      },
    });
    socketRef.current = socket;
    void socket.connect().catch(() => {
      /* reconnect handles subsequent attempts */
    });
    return () => {
      socket.close();
      socketRef.current = null;
      if (refetchTimerRef.current) {
        clearTimeout(refetchTimerRef.current);
        refetchTimerRef.current = null;
      }
    };
  }, [socketEnabled, scheduleVehicleRefetch]);

  useEffect(() => {
    if (!viewport || !socketRef.current) {
      return;
    }
    socketRef.current.setViewport({
      bbox: [...viewport.bbox],
      zoom: viewport.zoom,
      from: viewport.from,
      to: viewport.to,
      include_flexible: viewport.includeFlexible,
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
