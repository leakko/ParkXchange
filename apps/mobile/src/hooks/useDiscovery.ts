import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";

import { fetchSpots, type SpotFeature } from "@/api/client";
import { applySpotEvent, SpotSocket, type BBox } from "@/api/ws";
import { featureMatchesViewport } from "@/map/discoveryFilter";
import { isLiveMapSpotStatus } from "@/map/liveMapSpot";
import { SpotTombstones } from "@/map/spotTombstones";
import { requestActiveReservationRefreshDebounced } from "@/push/activeReservationSync";
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
  includeLeavingNow: boolean;
  leavingNowOnly: boolean;
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

function filterLiveMapFeatures(features: SpotFeature[]): SpotFeature[] {
  return features.filter((f) => isLiveMapSpotStatus(f.properties.status));
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
        includeLeavingNow: viewport!.includeLeavingNow,
        leavingNowOnly: viewport!.leavingNowOnly,
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

  // Drop the live overlay when the discovery filter changes so a stale WS
  // snapshot cannot keep flexibles on screen after «Solo salidas ya».
  useEffect(() => {
    if (!viewport) {
      return;
    }
    setLiveFeatures(null);
  }, [
    viewport?.from,
    viewport?.to,
    viewport?.includeFlexible,
    viewport?.includeLeavingNow,
    viewport?.leavingNowOnly,
  ]);

  // REST snapshot wins whenever it refreshes, but never resurrect tombstoned ids
  // (stale in-flight fetch right after accept → spot.removed).
  useEffect(() => {
    if (query.data) {
      setLiveFeatures(
        filterLiveMapFeatures(tombstonesRef.current.filter(query.data.features)),
      );
    }
  }, [query.data]);

  useEffect(() => {
    if (!socketEnabled) {
      return;
    }
    const socket = new SpotSocket({
      onSnapshot: (features) =>
        setLiveFeatures(
          filterLiveMapFeatures(tombstonesRef.current.filter(features)),
        ),
      onSpotEvent: (event) => {
        if (event.type === "reservation.updated") {
          requestActiveReservationRefreshDebounced();
        }
        tombstonesRef.current.noteEvent(event);
        setLiveFeatures((prev) => {
          const next = applySpotEvent(prev ?? EMPTY, event);
          if (eventNeedsVehicleRefetch(event, next)) {
            scheduleVehicleRefetch();
          }
          return filterLiveMapFeatures(tombstonesRef.current.filter(next));
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
      include_leaving_now: viewport.includeLeavingNow,
      leaving_now_only: viewport.leavingNowOnly,
    });
  }, [viewport]);

  const rawFeatures = liveFeatures ?? query.data?.features ?? EMPTY;
  const features = useMemo(() => {
    const live = filterLiveMapFeatures(tombstonesRef.current.filter(rawFeatures));
    if (!viewport) {
      return live;
    }
    return live.filter((f) => featureMatchesViewport(f, viewport));
  }, [rawFeatures, viewport]);

  const collection = useMemo(
    () => ({ type: "FeatureCollection" as const, features }),
    [features],
  );

  const featureById = useCallback(
    (id: string) => features.find((f) => String(f.id) === id) ?? null,
    [features],
  );

  const forgetSpot = useCallback(
    (id: string) => {
      const spotId = String(id);
      if (!spotId) {
        return;
      }
      tombstonesRef.current.noteEvent({
        type: "spot.removed",
        id: spotId,
        status: "cancelled",
      } as SpotEventMessage);
      setLiveFeatures((prev) => {
        const base = prev ?? query.data?.features ?? EMPTY;
        return filterLiveMapFeatures(
          tombstonesRef.current.filter(base.filter((f) => String(f.id) !== spotId)),
        );
      });
    },
    [query.data?.features],
  );

  return {
    collection,
    featureById,
    isLoading: query.isLoading && liveFeatures == null,
    error: query.error,
    refetch: query.refetch,
    forgetSpot,
  };
}
