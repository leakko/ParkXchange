import { Ionicons } from "@expo/vector-icons";
import {
  Camera,
  Map as MapView,
  NativeUserLocation,
  type CameraRef,
  type MapRef,
  type PressEvent,
  type ViewStateChangeEvent,
} from "@maplibre/maplibre-react-native";
import { type Href, useLocalSearchParams, usePathname, useRouter } from "expo-router";
import { useCallback, useEffect, useMemo, useReducer, useRef, useState } from "react";
import {
  ActivityIndicator,
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  TextInput,
  View,
  type NativeSyntheticEvent,
} from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";

import type { OfferResponse, SpotFeature, VehicleResponse } from "@/api/client";
import { ApiError, fetchMySpots, getSpot, listMyOffers, listOffers, listVehicles, withdrawSpot } from "@/api/client";
import { apiErrorMessage, apiErrorTitle } from "@/api/errors";
import { getAccessToken } from "@/api/session";
import { ensureEmailVerified } from "@/auth/requireEmailVerified";
import { defaultMapCenter, fallbackZoom, mapStyleUrl, userZoom } from "@/config";
import {
  loadPersistedMapCenter,
  resolveBootstrapCenter,
  shouldAnimateInitialCenter,
} from "@/map/mapHomeCenter";
import { useDiscovery, type Viewport } from "@/hooks/useDiscovery";
import { useSession } from "@/hooks/useSession";
import { useMapLocation } from "@/hooks/useMapLocation";
import { announceAt, parkCarAt, useActiveReservation } from "@/hooks/useSpotActions";
import { useEnRouteLocationReporter } from "@/hooks/useEnRouteLocationReporter";
import { useTranslation } from "@/i18n";
import { ParkCarModal, type ParkCarValues } from "@/map/ParkCarModal";
import { AnnounceModal, type AnnounceValues } from "@/map/AnnounceModal";
import { useConfirm } from "@/ui/ConfirmModal";
import {
  followReducer,
  initialFollowState,
  locationComponentReady,
  markSessionCameraCentered,
  shouldInitialCenterCamera,
} from "@/map/followUser";

/** Fields the selected pin cares about — ignore referential churn from polls/WS. */
function sheetSpotDrifted(current: SpotFeature, next: SpotFeature): boolean {
  return (
    current.properties.status !== next.properties.status ||
    current.properties.exact_location !== next.properties.exact_location ||
    current.properties.price_cents !== next.properties.price_cents ||
    current.geometry.coordinates[0] !== next.geometry.coordinates[0] ||
    current.geometry.coordinates[1] !== next.geometry.coordinates[1]
  );
}
import { ExchangeLayers } from "@/map/ExchangeLayers";
import { AnnounceDraftLayers } from "@/map/AnnounceDraftLayers";
import { HistoryGhostLayers } from "@/map/HistoryGhostLayers";
import { HISTORY_GHOST_MS, isLiveMapSpotStatus } from "@/map/liveMapSpot";
import { MySpotLayers } from "@/map/MySpotLayers";
import { OfferedSpotLayers } from "@/map/OfferedSpotLayers";
import { SearchPlaceLayers } from "@/map/SearchPlaceLayers";
import { UncertaintyCircle } from "@/map/UncertaintyCircle";
import { partitionMapSpots } from "@/map/partitionMapSpots";
import {
  autocompletePlaces,
  boundsForHits,
  matchCategoryPrefix,
  matchExactCategory,
  searchCategoryNearby,
  searchPlacesDetailed,
  shouldLiveAutocomplete,
  type AddressSuggestion,
  type CategoryHint,
  type ViewBox,
} from "@/map/geocode";
import { bannerPeerStatusKey } from "@/map/exchangeCopy";
import { peerEnRouteDistance } from "@/map/peerDistance";
import { passAuthGate } from "@/map/authGate";
import { SpotLayers } from "@/map/SpotLayers";
import { stageSpotForSheet, beginSpotSheetPresentation } from "@/map/spotSheetHandoff";
import { subscribeSpotWithdrawn } from "@/map/spotWithdrawHandoff";
import {
  consumeResumeAnnounceAfterVehicle,
  consumeResumeParkAfterVehicle,
  subscribeVehicleCreated,
} from "@/map/vehicleCreateHandoff";
import { defaultMapFilter } from "@/map/mapFilter";
import {
  stageMapFilter,
  subscribeMapFilter,
} from "@/map/mapFilterHandoff";

const DEBOUNCE_MS = 350;
/** Longer than map pan debounce — typing must not hammer LocationIQ. */
const SUGGEST_DEBOUNCE_MS = 700;
const FOCUS_SPOT_ZOOM = 17;

export default function MapScreen() {
  const { t, formatDateTime, locale } = useTranslation();
  const { confirm, alert } = useConfirm();
  const router = useRouter();
  const pathname = usePathname();
  const focusParams = useLocalSearchParams<{
    focusLon?: string;
    focusLat?: string;
    focusSpot?: string;
    /** "1" = historical glance: camera + ghost pin, no live sheet. */
    focusHistory?: string;
    announceLon?: string;
    announceLat?: string;
    announceLabel?: string;
    announcePrice?: string;
    announceVehicle?: string;
  }>();
  const insets = useSafeAreaInsets();
  const mapRef = useRef<MapRef>(null);
  const cameraRef = useRef<CameraRef>(null);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const suggestDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const mapViewboxRef = useRef<ViewBox | null>(null);
  /** Bumped on pan/search so a late locate refresh cannot yank the camera. */
  const locateGenRef = useRef(0);
  const searchInputRef = useRef<TextInput>(null);
  /** Set when search is tapped while the spot sheet is open; focus after dismiss. */
  const pendingSearchFocusRef = useRef(false);
  /** Query that produced the current searchHits — editing away clears results. */
  const lastSearchedQueryRef = useRef("");
  const [mapFilter, setMapFilter] = useState(() => defaultMapFilter());
  /** Always-current filter for debounced publishViewport (avoid stale overwrite). */
  const mapFilterRef = useRef(mapFilter);
  mapFilterRef.current = mapFilter;

  const { ready, signedIn, error: sessionError, retry: retrySession } = useSession();
  const location = useMapLocation();
  /**
   * undefined = AsyncStorage still loading; null = nothing saved.
   * Map mounts only after storage + location.ready so first paint can use
   * OS last-known (or persisted) instead of flashing Sevilla.
   */
  const [persistedHome, setPersistedHome] = useState<[number, number] | null | undefined>(
    undefined,
  );
  const [homeCenter, setHomeCenter] = useState<[number, number] | null>(null);
  const [homeZoom, setHomeZoom] = useState(fallbackZoom);
  const homeCenterRef = useRef<[number, number] | null>(null);
  const [follow, dispatchFollow] = useReducer(followReducer, undefined, initialFollowState);
  const [viewport, setViewport] = useState<Viewport | null>(null);
  const [selected, setSelected] = useState<SpotFeature | null>(null);
  const [mapReady, setMapReady] = useState(false);
  const [spotsArmed, setSpotsArmed] = useState(false);
  const [mineArmed, setMineArmed] = useState(false);
  const [offeredArmed, setOfferedArmed] = useState(false);
  const [announcing, setAnnouncing] = useState(false);
  const [myOffers, setMyOffers] = useState<OfferResponse[]>([]);
  const [announceOpen, setAnnounceOpen] = useState(false);
  const [announceVehicles, setAnnounceVehicles] = useState<VehicleResponse[]>([]);
  const [announceCoords, setAnnounceCoords] = useState<[number, number] | null>(null);
  const [announceLabel, setAnnounceLabel] = useState<string | null>(null);
  const [announcePriceCents, setAnnouncePriceCents] = useState<number | null>(null);
  const [announceVehicleId, setAnnounceVehicleId] = useState<string | null>(null);
  const [announcePickMode, setAnnouncePickMode] = useState(false);
  /** Keep form fields when returning from map pick / search. */
  const [announceKeepForm, setAnnounceKeepForm] = useState(false);
  const [parking, setParking] = useState(false);
  const [parkOpen, setParkOpen] = useState(false);
  const [parkVehicles, setParkVehicles] = useState<VehicleResponse[]>([]);
  const [parkCoords, setParkCoords] = useState<[number, number] | null>(null);
  const [parkLabel, setParkLabel] = useState<string | null>(null);
  const [parkFromCurrentLocation, setParkFromCurrentLocation] = useState(false);
  const [parkPickMode, setParkPickMode] = useState(false);
  const [searchQuery, setSearchQuery] = useState("");
  const [searchHits, setSearchHits] = useState<AddressSuggestion[]>([]);
  const [suggestHits, setSuggestHits] = useState<AddressSuggestion[]>([]);
  const [categoryHint, setCategoryHint] = useState<CategoryHint | null>(null);
  const [selectedSearchId, setSelectedSearchId] = useState<string | null>(null);
  const [searchBusy, setSearchBusy] = useState(false);
  const [mapViewbox, setMapViewbox] = useState<ViewBox | null>(null);
  mapViewboxRef.current = mapViewbox;
  /** Temporary pin when glancing at a historical spot from account. */
  const [historyGhost, setHistoryGhost] = useState<{
    lon: number;
    lat: number;
  } | null>(null);
  const historyGhostTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  /** Ignore pan dismiss while the focus easeTo is still settling. */
  const historyGhostArmedAtRef = useRef(0);

  const clearHistoryGhost = useCallback(() => {
    if (historyGhostTimerRef.current) {
      clearTimeout(historyGhostTimerRef.current);
      historyGhostTimerRef.current = null;
    }
    setHistoryGhost(null);
  }, []);

  const showHistoryGhost = useCallback((lon: number, lat: number) => {
    if (historyGhostTimerRef.current) {
      clearTimeout(historyGhostTimerRef.current);
    }
    historyGhostArmedAtRef.current = Date.now();
    setHistoryGhost({ lon, lat });
    historyGhostTimerRef.current = setTimeout(() => {
      historyGhostTimerRef.current = null;
      setHistoryGhost(null);
    }, HISTORY_GHOST_MS);
  }, []);

  useEffect(() => {
    return () => {
      if (historyGhostTimerRef.current) {
        clearTimeout(historyGhostTimerRef.current);
      }
    };
  }, []);

  const puckReady = locationComponentReady(follow.locationGranted, location.coords);

  // Foreground location is requested by useMapLocation (policy case A).
  // Background / “always” is only requested after «Voy de camino» (geofence).

  useEffect(() => {
    void loadPersistedMapCenter().then((persisted) => {
      setPersistedHome(persisted);
    });
  }, []);

  useEffect(() => {
    if (homeCenter != null) {
      return;
    }
    if (persistedHome === undefined || !location.ready) {
      return;
    }
    const center = resolveBootstrapCenter({
      osLastKnown: location.coords,
      persisted: persistedHome,
      fallback: defaultMapCenter,
    });
    homeCenterRef.current = center;
    setHomeCenter(center);
    setHomeZoom(location.coords != null || persistedHome != null ? userZoom : fallbackZoom);
  }, [homeCenter, persistedHome, location.ready, location.coords]);

  const requireSignIn = useCallback(
    (returnTo: string = "/") => {
      router.push(`/auth/login?returnTo=${encodeURIComponent(returnTo)}` as Href);
    },
    [router],
  );

  /** Soft-gate: session exists, but announce/reserve need a confirmed email. */
  const requireEmailVerified = useCallback(async (): Promise<boolean> => {
    return ensureEmailVerified({
      t,
      onUnauthorized: () => requireSignIn("/"),
    });
  }, [requireSignIn, t]);

  /** Open or update the single spot form sheet (never stack / remount). */
  const openSpotDetail = useCallback(
    (spot: SpotFeature) => {
      const id = String(spot.id ?? "");
      if (!id) {
        return;
      }
      setSelected(spot);
      stageSpotForSheet(spot);
      if (pathname.startsWith("/spot")) {
        // Sheet already open — only swap content (no navigation remount).
        return;
      }
      // Fresh presentation: new singular id so the sheet opens at peek, not
      // the detent left from a previous expanded session.
      beginSpotSheetPresentation();
      router.push(`/spot/${id}` as Href);
    },
    [pathname, router],
  );

  /** Dismiss the spot sheet so map chrome (search) is not fighting detents. */
  const spotSheetOpen = pathname.startsWith("/spot");
  const dismissSpotSheet = useCallback(() => {
    if (!spotSheetOpen) {
      return;
    }
    if (router.canGoBack()) {
      router.back();
    } else {
      router.replace("/" as Href);
    }
  }, [spotSheetOpen, router]);

  // Focus search only after the sheet has left — focusing while it is mounted
  // makes Android expand the form sheet violently for the keyboard.
  useEffect(() => {
    if (spotSheetOpen || !pendingSearchFocusRef.current) {
      return;
    }
    pendingSearchFocusRef.current = false;
    const t = setTimeout(() => searchInputRef.current?.focus(), 16);
    return () => clearTimeout(t);
  }, [spotSheetOpen]);

  /** Fly the camera to a spot, then open its sheet. */
  const focusSpotOnMap = useCallback(
    (spot: SpotFeature) => {
      clearHistoryGhost();
      const lon = Number(spot.geometry.coordinates[0]);
      const lat = Number(spot.geometry.coordinates[1]);
      if (Number.isFinite(lon) && Number.isFinite(lat)) {
        dispatchFollow({ type: "claim_camera" });
        cameraRef.current?.easeTo({
          center: [lon, lat],
          zoom: FOCUS_SPOT_ZOOM,
          duration: 500,
        });
      }
      openSpotDetail(spot);
    },
    [openSpotDetail, clearHistoryGhost],
  );

  const { collection, featureById, isLoading, error, refetch, forgetSpot } = useDiscovery(
    viewport,
    signedIn,
  );
  const {
    active,
    activeSpot,
    isOwner,
    isDriver,
    busy,
    markEnRoute,
    markReady,
    clearReady,
    refresh: refreshActiveReservation,
  } = useActiveReservation(signedIn);

  const iAmEnRoute =
    !!active &&
    ((isOwner && !!active.owner_en_route_at && !active.owner_ready_at) ||
      (isDriver && !!active.driver_en_route_at && !active.driver_ready_at));
  useEnRouteLocationReporter({
    reservationId: active?.id ?? null,
    enabled: iAmEnRoute,
  });

  const leavingNowSpot = useMemo(() => {
    if (active || !signedIn) {
      return null;
    }
    return (
      collection.features.find(
        (f) =>
          f.properties.is_mine &&
          !!f.properties.leaving_now &&
          f.properties.status === "available",
      ) ?? null
    );
  }, [active, signedIn, collection.features]);

  const [leavingNowOfferCount, setLeavingNowOfferCount] = useState(0);
  useEffect(() => {
    if (!leavingNowSpot) {
      setLeavingNowOfferCount(0);
      return;
    }
    let cancelled = false;
    const load = async () => {
      try {
        const offers = await listOffers(String(leavingNowSpot.id));
        if (!cancelled) {
          setLeavingNowOfferCount(
            offers.filter((o) => o.status === "pending").length,
          );
        }
      } catch {
        if (!cancelled) {
          setLeavingNowOfferCount(0);
        }
      }
    };
    void load();
    const timer = setInterval(() => void load(), 8_000);
    return () => {
      cancelled = true;
      clearInterval(timer);
    };
  }, [leavingNowSpot?.id]);

  const [mySpotFeatures, setMySpotFeatures] = useState<SpotFeature[]>([]);

  const refreshMyOffers = useCallback(async () => {
    if (!signedIn) {
      setMyOffers([]);
      return;
    }
    try {
      const list = await listMyOffers();
      setMyOffers(list);
    } catch {
      setMyOffers([]);
    }
  }, [signedIn]);

  const refreshMySpotsOverlay = useCallback(async () => {
    if (!signedIn) {
      setMySpotFeatures([]);
      return;
    }
    try {
      const collection = await fetchMySpots();
      setMySpotFeatures(
        collection.features.filter((f) => isLiveMapSpotStatus(f.properties.status)),
      );
    } catch {
      setMySpotFeatures([]);
    }
  }, [signedIn]);

  /** Drop a withdrawn listing from discovery + own-pin overlay immediately. */
  const removeSpotFromMap = useCallback(
    async (spotId: string) => {
      const id = String(spotId);
      forgetSpot(id);
      setMySpotFeatures((prev) => prev.filter((f) => String(f.id) !== id));
      setSelected((prev) => (prev && String(prev.id) === id ? null : prev));
      await Promise.all([refetch(), refreshMySpotsOverlay()]);
    },
    [forgetSpot, refetch, refreshMySpotsOverlay],
  );

  useEffect(() => {
    return subscribeSpotWithdrawn((spotId) => {
      void removeSpotFromMap(spotId);
    });
  }, [removeSpotFromMap]);

  useEffect(() => {
    void refreshMyOffers();
    void refreshMySpotsOverlay();
  }, [refreshMyOffers, refreshMySpotsOverlay]);

  // Refresh owner pins when an exchange starts, changes, or ends (active → null).
  // Ending without a refresh left a stale reserved pin on the map after complete.
  const activeExchangeKey = active ? `${active.id}:${active.status}` : "none";
  const prevActiveExchangeKey = useRef(activeExchangeKey);
  useEffect(() => {
    const prev = prevActiveExchangeKey.current;
    prevActiveExchangeKey.current = activeExchangeKey;
    void refreshMySpotsOverlay();
    if (prev !== "none" && activeExchangeKey === "none") {
      setSelected(null);
      void refetch();
    }
  }, [activeExchangeKey, refreshMySpotsOverlay, refetch]);

  const pendingOfferBySpotId = useMemo(() => {
    const map = new Map<string, OfferResponse>();
    for (const offer of myOffers) {
      if (offer.status === "pending") {
        map.set(String(offer.spot_id), offer);
      }
    }
    return map;
  }, [myOffers]);

  const mySpotsById = useMemo(() => {
    const map = new Map<string, SpotFeature>();
    for (const feature of mySpotFeatures) {
      map.set(String(feature.id), feature);
    }
    return map;
  }, [mySpotFeatures]);

  // Discovery/WS must not overwrite the selected pin during a live exchange:
  // getSpot (exact, reserved/handover) and the viewport copy (often fuzzed or
  // a beat behind on status) used to thrash setSelected forever.
  useEffect(() => {
    if (!selected?.id) {
      return;
    }
    if (active && String(active.spot_id) === String(selected.id)) {
      return;
    }
    const live = featureById(String(selected.id));
    if (!live) {
      return;
    }
    if (sheetSpotDrifted(selected, live)) {
      setSelected(live);
    }
  }, [featureById, selected, active]);

  const spotData = useMemo(() => {
    const byId = new Map<
      string,
      {
        type: "Feature";
        properties: {
          id: string;
          price_cents: number;
          status: string;
          is_mine: boolean;
          has_my_offer: boolean;
          has_agreement: boolean;
          exact_location: boolean;
          leaving_now: boolean;
          /** No preferred time → grey P on the map (matches filter “Resto”). */
          flexible: boolean;
        };
        geometry: { type: "Point"; coordinates: [number, number] };
      }
    >();

    for (const feature of collection.features) {
      const id = String(feature.id ?? "");
      if (!isLiveMapSpotStatus(feature.properties.status)) {
        continue;
      }
      // Active exchange owns this pin via ExchangeLayers (exact coords). Keeping
      // the discovery/offered copy fights the exact pin after accept.
      if (active && String(active.spot_id) === id) {
        continue;
      }
      const leavingNow = Boolean(feature.properties.leaving_now);
      byId.set(id, {
        type: "Feature",
        properties: {
          id,
          price_cents: feature.properties.price_cents,
          status: feature.properties.status,
          is_mine: Boolean(feature.properties.is_mine),
          has_my_offer: pendingOfferBySpotId.has(id),
          has_agreement: Boolean(
            active && String(active.spot_id) === id && (isOwner || isDriver),
          ),
          exact_location: Boolean(feature.properties.exact_location),
          leaving_now: leavingNow,
          flexible: !leavingNow && !feature.properties.preferred_departure_at,
        },
        geometry: {
          type: "Point",
          coordinates: [
            Number(feature.geometry.coordinates[0]),
            Number(feature.geometry.coordinates[1]),
          ],
        },
      });
    }

    // Owner listings (including reserved/handover) stay on the map via /spots/mine.
    for (const feature of mySpotFeatures) {
      if (!isLiveMapSpotStatus(feature.properties.status)) {
        continue;
      }
      const status = feature.properties.status;
      const id = String(feature.id ?? "");
      if (active && String(active.spot_id) === id && isDriver && !isOwner) {
        continue;
      }
      const leavingNow = Boolean(feature.properties.leaving_now);
      byId.set(id, {
        type: "Feature",
        properties: {
          id,
          price_cents: feature.properties.price_cents,
          status,
          is_mine: true,
          has_my_offer: false,
          has_agreement: Boolean(
            active && String(active.spot_id) === id && (isOwner || isDriver),
          ),
          exact_location: Boolean(feature.properties.exact_location),
          leaving_now: leavingNow,
          flexible: !leavingNow && !feature.properties.preferred_departure_at,
        },
        geometry: {
          type: "Point",
          coordinates: [
            Number(feature.geometry.coordinates[0]),
            Number(feature.geometry.coordinates[1]),
          ],
        },
      });
    }

    const { mine, offered, others } = partitionMapSpots([...byId.values()]);
    return {
      others: { type: "FeatureCollection" as const, features: others },
      mine: { type: "FeatureCollection" as const, features: mine },
      offered: { type: "FeatureCollection" as const, features: offered },
    };
  }, [collection.features, pendingOfferBySpotId, mySpotFeatures, active, isDriver, isOwner]);

  // Live exchange: sheet follows getSpot only, and only when fields actually drift
  // (poll every 5s must not rewrite selected with a fresh object each time).
  useEffect(() => {
    if (!activeSpot) {
      return;
    }
    setSelected((prev) => {
      if (!prev || String(prev.id) !== String(activeSpot.id)) {
        return prev;
      }
      if (!sheetSpotDrifted(prev, activeSpot)) {
        return prev;
      }
      return activeSpot;
    });
  }, [activeSpot]);

  // Driver exchange pin only — owner reserved spots already appear in MySpotLayers.
  const exchangeData = useMemo(() => {
    if (!activeSpot || !isDriver || isOwner) {
      return { type: "FeatureCollection" as const, features: [] };
    }
    return {
      type: "FeatureCollection" as const,
      features: [
        {
          type: "Feature" as const,
          properties: { id: String(activeSpot.id ?? "") },
          geometry: {
            type: "Point" as const,
            coordinates: [
              Number(activeSpot.geometry.coordinates[0]),
              Number(activeSpot.geometry.coordinates[1]),
            ] as [number, number],
          },
        },
      ],
    };
  }, [activeSpot, isDriver, isOwner]);

  const historyGhostData = useMemo(() => {
    if (!historyGhost) {
      return { type: "FeatureCollection" as const, features: [] };
    }
    return {
      type: "FeatureCollection" as const,
      features: [
        {
          type: "Feature" as const,
          properties: { id: "history-ghost" },
          geometry: {
            type: "Point" as const,
            coordinates: [historyGhost.lon, historyGhost.lat] as [number, number],
          },
        },
      ],
    };
  }, [historyGhost]);

  // Owner with an active exchange: ensure their reserved pin is present even if
  // /spots/mine has not refreshed yet.
  const ownerExchangeMine = useMemo(() => {
    if (!activeSpot || !isOwner) {
      return spotData.mine;
    }
    const id = String(activeSpot.id ?? "");
    if (spotData.mine.features.some((f) => f.properties.id === id)) {
      return spotData.mine;
    }
    return {
      type: "FeatureCollection" as const,
      features: [
        ...spotData.mine.features,
        {
          type: "Feature" as const,
          properties: {
            id,
            price_cents: activeSpot.properties.price_cents,
            status: activeSpot.properties.status,
            is_mine: true,
            has_my_offer: false,
            exact_location: Boolean(activeSpot.properties.exact_location),
            leaving_now: Boolean(activeSpot.properties.leaving_now),
            flexible:
              !activeSpot.properties.leaving_now &&
              !activeSpot.properties.preferred_departure_at,
          },
          geometry: {
            type: "Point" as const,
            coordinates: [
              Number(activeSpot.geometry.coordinates[0]),
              Number(activeSpot.geometry.coordinates[1]),
            ] as [number, number],
          },
        },
      ],
    };
  }, [spotData.mine, activeSpot, isOwner]);

  useEffect(() => {
    if (mapReady && spotData.others.features.length > 0) {
      setSpotsArmed(true);
    }
  }, [mapReady, spotData.others.features.length]);

  useEffect(() => {
    if (mapReady && ownerExchangeMine.features.length > 0) {
      setMineArmed(true);
    }
  }, [mapReady, ownerExchangeMine.features.length]);

  useEffect(() => {
    if (mapReady && spotData.offered.features.length > 0) {
      setOfferedArmed(true);
    }
  }, [mapReady, spotData.offered.features.length]);

  useEffect(() => {
    if (!location.ready) {
      return;
    }
    dispatchFollow(location.granted ? { type: "location_granted" } : { type: "location_denied" });
  }, [location.ready, location.granted]);

  useEffect(() => {
    // Pick mode owns the camera: consume the one-shot so a late GPS fix (or
    // leaving pick) cannot yank back to the user.
    if (announcePickMode && location.coords && mapReady) {
      markSessionCameraCentered();
      return;
    }
    // One decision per JS process when map + GPS are ready — never continuous
    // follow. Jump only if the bootstrap center is far from the fix (avoids
    // Sevilla→Bilbao teleports when lastKnown/persisted already put us nearby).
    if (
      !shouldInitialCenterCamera({
        mapReady,
        coords: location.coords,
        announcePickMode,
      }) ||
      !location.coords ||
      homeCenter == null
    ) {
      return;
    }
    if (shouldAnimateInitialCenter(homeCenterRef.current, location.coords)) {
      cameraRef.current?.jumpTo({ center: location.coords, zoom: userZoom });
      homeCenterRef.current = location.coords;
      setHomeCenter(location.coords);
    }
    markSessionCameraCentered();
  }, [location.coords, announcePickMode, userZoom, mapReady, homeCenter]);

  const publishViewport = useCallback(async () => {
    const map = mapRef.current;
    if (!map) {
      return;
    }
    const [bounds, zoom] = await Promise.all([map.getBounds(), map.getZoom()]);
    const [west, south, east, north] = bounds;
    if (!(east > west) || !(north > south)) {
      return;
    }
    if (east - west > 0.5 || north - south > 0.5) {
      return;
    }
    const filter = mapFilterRef.current;
    setViewport({
      bbox: bounds,
      zoom,
      from: filter.from,
      to: filter.to,
      includeFlexible: filter.includeFlexible,
      includeLeavingNow: filter.includeLeavingNow,
      leavingNowOnly: filter.leavingNowOnly,
    });
    setMapViewbox(bounds as ViewBox);
  }, []);

  const onRegionDidChange = useCallback(
    (event: NativeSyntheticEvent<ViewStateChangeEvent>) => {
      if (event.nativeEvent.userInteraction) {
        locateGenRef.current += 1;
        dispatchFollow({ type: "user_gesture" });
        // easeTo for focus can report userInteraction on some builds — ignore briefly.
        if (Date.now() - historyGhostArmedAtRef.current > 800) {
          clearHistoryGhost();
        }
      }
      if (debounceRef.current) {
        clearTimeout(debounceRef.current);
      }
      debounceRef.current = setTimeout(() => {
        void publishViewport();
      }, DEBOUNCE_MS);
    },
    [publishViewport, clearHistoryGhost],
  );

  const onPressFeature = useCallback(
    (id: string) => {
      clearHistoryGhost();
      void (async () => {
        const fromActive = activeSpot && String(activeSpot.id) === id ? activeSpot : null;
        const fromMine = mySpotsById.get(id);
        const fromDiscovery = featureById(id);
        // Prefer party views (active exchange / own listing) over discovery fuzz.
        let spot = fromActive ?? fromMine ?? fromDiscovery;
        if (!spot) {
          try {
            spot = await getSpot(id);
          } catch {
            return;
          }
        }
        if (!isLiveMapSpotStatus(spot.properties.status) && !fromActive) {
          const lon = Number(spot.geometry.coordinates[0]);
          const lat = Number(spot.geometry.coordinates[1]);
          if (Number.isFinite(lon) && Number.isFinite(lat)) {
            showHistoryGhost(lon, lat);
          }
          return;
        }
        openSpotDetail(spot);
        const openExchange =
          !!fromActive ||
          spot.properties.status === "reserved" ||
          spot.properties.status === "handover";
        if (openExchange) {
          void refreshActiveReservation();
        }
      })();
    },
    [
      featureById,
      mySpotsById,
      activeSpot,
      refreshActiveReservation,
      openSpotDetail,
      clearHistoryGhost,
      showHistoryGhost,
    ],
  );

  const clearSearchHits = useCallback(() => {
    setSearchHits([]);
    setSuggestHits([]);
    setCategoryHint(null);
    setSelectedSearchId(null);
    setSearchQuery("");
    lastSearchedQueryRef.current = "";
  }, []);

  const onChangeSearchQuery = useCallback((text: string) => {
    setSearchQuery(text);
    if (text.trim() !== lastSearchedQueryRef.current) {
      setSearchHits([]);
      setSelectedSearchId(null);
    }
  }, []);

  // Local category hint + debounced LocationIQ autocomplete (no Nearby until tap).
  // Do not depend on mapViewbox — panning would re-fire autocomplete and burn quota.
  useEffect(() => {
    const q = searchQuery.trim();
    setCategoryHint(matchCategoryPrefix(q, locale));
    if (suggestDebounceRef.current) {
      clearTimeout(suggestDebounceRef.current);
    }
    if (!shouldLiveAutocomplete(q)) {
      setSuggestHits([]);
      return;
    }
    suggestDebounceRef.current = setTimeout(() => {
      void (async () => {
        try {
          const hits = await autocompletePlaces(q, {
            viewbox: mapViewboxRef.current ?? undefined,
            locale,
            limit: 3,
          });
          setSuggestHits(hits);
        } catch {
          setSuggestHits([]);
        }
      })();
    }, SUGGEST_DEBOUNCE_MS);
    return () => {
      if (suggestDebounceRef.current) {
        clearTimeout(suggestDebounceRef.current);
      }
    };
  }, [searchQuery, locale]);

  const onPressMap = useCallback(
    (event: NativeSyntheticEvent<PressEvent>) => {
      clearHistoryGhost();
      if (announcePickMode || parkPickMode) {
        const [lon, lat] = event.nativeEvent.lngLat;
        clearSearchHits();
        const label = t("announce.location.coords", {
          lat: lat.toFixed(5),
          lon: lon.toFixed(5),
        });
        if (parkPickMode) {
          setParkCoords([lon, lat]);
          setParkLabel(null);
          setParkFromCurrentLocation(false);
          setParkPickMode(false);
          setParkOpen(true);
          return;
        }
        setAnnounceCoords([lon, lat]);
        setAnnounceLabel(label);
        setAnnouncePickMode(false);
        setAnnounceOpen(true);
        return;
      }
      setSelected(null);
    },
    [announcePickMode, parkPickMode, clearSearchHits, clearHistoryGhost, t],
  );

  const onPressSearchHit = useCallback(
    (id: string) => {
      const hit = searchHits.find((h) => h.id === id);
      if (!hit) {
        return;
      }
      // Preview on the map; exact car position is chosen with announce pick / map.
      setSelectedSearchId(id);
      dispatchFollow({ type: "claim_camera" });
      cameraRef.current?.easeTo({
        center: [hit.lon, hit.lat],
        zoom: Math.max(userZoom, 15),
        duration: 450,
      });
    },
    [searchHits, userZoom],
  );

  const applySearchResult = useCallback(
    (
      hits: AddressSuggestion[],
      inViewport: AddressSuggestion[],
      shouldZoomOut: boolean,
      committedQuery: string,
    ) => {
      lastSearchedQueryRef.current = committedQuery.trim();
      setSearchHits(hits);
      setSuggestHits([]);
      setSelectedSearchId(null);
      if (hits.length === 0) {
        void alert({
          title: t("map.search.empty.title"),
          message: t("map.search.empty.message"),
          confirmLabel: t("common.ok"),
        });
        return;
      }
      dispatchFollow({ type: "claim_camera" });
      if (shouldZoomOut) {
        const fit = boundsForHits(hits.slice(0, 8));
        if (fit) {
          const [west, south, east, north] = fit;
          cameraRef.current?.fitBounds([west, south, east, north], {
            padding: { top: 180, right: 48, bottom: 48, left: 48 },
            duration: 450,
          });
        }
      }
      if (inViewport.length === 1) {
        setSelectedSearchId(inViewport[0]!.id);
      } else if (hits.length === 1) {
        setSelectedSearchId(hits[0]!.id);
      }
    },
    [alert, t],
  );

  const resolveViewbox = useCallback(async (): Promise<ViewBox | undefined> => {
    let viewbox = mapViewbox;
    try {
      const bounds = await mapRef.current?.getBounds();
      if (bounds && bounds[2]! > bounds[0]! && bounds[3]! > bounds[1]!) {
        viewbox = bounds as ViewBox;
        setMapViewbox(viewbox);
      }
    } catch {
      /* keep state */
    }
    return viewbox ?? undefined;
  }, [mapViewbox]);

  const runMapSearch = useCallback(async () => {
    const q = searchQuery.trim();
    if (q.length < 3) {
      return;
    }
    setSearchBusy(true);
    try {
      const viewbox = await resolveViewbox();
      const { hits, inViewport, shouldZoomOut } = await searchPlacesDetailed(q, {
        viewbox,
        locale,
      });
      applySearchResult(hits, inViewport, shouldZoomOut, q);
    } catch (err) {
      const detail = err instanceof Error && err.message.trim() ? err.message : t("common.error");
      await alert({
        title: t("map.search.failed"),
        message: detail,
        confirmLabel: t("common.ok"),
      });
    } finally {
      setSearchBusy(false);
    }
  }, [searchQuery, locale, resolveViewbox, applySearchResult, alert, t]);

  const runCategoryHintSearch = useCallback(
    async (hint: CategoryHint) => {
      setSearchBusy(true);
      try {
        const viewbox = await resolveViewbox();
        const { hits, inViewport, shouldZoomOut } = await searchCategoryNearby(hint.osmTags, {
          viewbox,
          locale,
        });
        setSearchQuery(hint.label);
        applySearchResult(hits, inViewport, shouldZoomOut, hint.label);
      } catch (err) {
        const detail = err instanceof Error && err.message.trim() ? err.message : t("common.error");
        await alert({
          title: t("map.search.failed"),
          message: detail,
          confirmLabel: t("common.ok"),
        });
      } finally {
        setSearchBusy(false);
      }
    },
    [resolveViewbox, locale, applySearchResult, alert, t],
  );

  const onPressSuggestHit = useCallback(
    (hit: AddressSuggestion) => {
      applySearchResult([hit], [hit], false, hit.label);
      setSearchQuery(hit.label);
      setSelectedSearchId(hit.id);
      dispatchFollow({ type: "claim_camera" });
      cameraRef.current?.easeTo({
        center: [hit.lon, hit.lat],
        zoom: Math.max(userZoom, 15),
        duration: 450,
      });
    },
    [applySearchResult, userZoom],
  );

  const onRecenter = useCallback(() => {
    dispatchFollow({ type: "recenter" });
    const gen = ++locateGenRef.current;
    const cached = location.coords;
    // One camera move only. A late getCurrentPosition (~few seconds) must not
    // easeTo again — the user may already have panned away.
    if (cached) {
      cameraRef.current?.easeTo({
        center: cached,
        zoom: userZoom,
        duration: 400,
      });
      void location.refresh();
      return;
    }
    void (async () => {
      const fresh = await location.refresh();
      if (gen !== locateGenRef.current || !fresh) {
        return;
      }
      cameraRef.current?.easeTo({
        center: fresh,
        zoom: userZoom,
        duration: 400,
      });
    })();
  }, [location, userZoom]);

  // Filter form sheet publishes Apply/Reset here (same pattern as spot handoff).
  useEffect(() => {
    return subscribeMapFilter((next) => {
      setMapFilter(next);
      setViewport((current) =>
        current
          ? {
              ...current,
              from: next.from,
              to: next.to,
              includeFlexible: next.includeFlexible,
              includeLeavingNow: next.includeLeavingNow,
              leavingNowOnly: next.leavingNowOnly,
            }
          : current,
      );
    });
  }, []);

  // Deep-link from "show my spot on map" in account → fly camera; live opens
  // sheet, history shows a temporary ghost pin.
  useEffect(() => {
    const lon = Number.parseFloat(String(focusParams.focusLon ?? ""));
    const lat = Number.parseFloat(String(focusParams.focusLat ?? ""));
    if (!mapReady || !Number.isFinite(lon) || !Number.isFinite(lat)) {
      return;
    }
    const wantHistory =
      focusParams.focusHistory === "1" || focusParams.focusHistory === "true";
    dispatchFollow({ type: "claim_camera" });
    cameraRef.current?.easeTo({
      center: [lon, lat],
      zoom: FOCUS_SPOT_ZOOM,
      duration: 500,
    });
    const spotId = focusParams.focusSpot ? String(focusParams.focusSpot) : null;
    if (wantHistory) {
      setSelected(null);
      showHistoryGhost(lon, lat);
    } else if (spotId) {
      void (async () => {
        try {
          const spot = await getSpot(spotId);
          if (!isLiveMapSpotStatus(spot.properties.status)) {
            setSelected(null);
            showHistoryGhost(lon, lat);
            return;
          }
          clearHistoryGhost();
          setSelected(spot);
          setMineArmed(true);
          setSpotsArmed(true);
          openSpotDetail(spot);
        } catch {
          showHistoryGhost(lon, lat);
        }
      })();
    } else {
      showHistoryGhost(lon, lat);
    }
    router.setParams({
      focusLon: undefined,
      focusLat: undefined,
      focusSpot: undefined,
      focusHistory: undefined,
    });
  }, [
    mapReady,
    focusParams.focusLon,
    focusParams.focusLat,
    focusParams.focusSpot,
    focusParams.focusHistory,
    openSpotDetail,
    router,
    showHistoryGhost,
    clearHistoryGhost,
  ]);

  const afterAnnounce = useCallback(
    async (spot: SpotFeature, message: string) => {
      setSpotsArmed(true);
      setMineArmed(true);
      openSpotDetail(spot);
      await Promise.all([refetch(), refreshMySpotsOverlay()]);
      await alert({
        title: t("map.alert.announced.title"),
        message,
        confirmLabel: t("common.ok"),
      });
    },
    [alert, openSpotDetail, refetch, refreshMySpotsOverlay, t],
  );

  const openAnnounce = useCallback(
    async (
      coords: [number, number] | null,
      label: string | null,
      extras?: { priceCents?: number | null; vehicleId?: string | null },
    ) => {
      const hasSession = signedIn && !!(await getAccessToken());
      const allowed = await passAuthGate({
        signedIn: hasSession,
        confirmSignIn: () =>
          confirm({
            title: t("auth.required.title"),
            message: t("auth.required.announce"),
            cancelLabel: t("common.cancel"),
            confirmLabel: t("auth.required.signIn"),
          }),
        onRequireSignIn: () => requireSignIn("/"),
      });
      if (!allowed) {
        return;
      }
      if (!(await requireEmailVerified())) {
        return;
      }
      setAnnouncing(true);
      try {
        const list = await listVehicles();
        if (list.length === 0) {
          const add = await confirm({
            title: t("announce.needVehicle.title"),
            message: t("announce.needVehicle.message"),
            cancelLabel: t("common.cancel"),
            confirmLabel: t("announce.needVehicle.add"),
          });
          if (add) {
            router.push("/account/vehicles/new?from=announce" as Href);
          }
          return;
        }
        setAnnounceVehicles(list);
        setAnnounceCoords(coords);
        setAnnounceLabel(label);
        setAnnouncePriceCents(extras?.priceCents ?? null);
        setAnnounceVehicleId(extras?.vehicleId ?? null);
        setAnnounceKeepForm(false);
        setAnnouncePickMode(false);
        setAnnounceOpen(true);
      } catch (err) {
        if (err instanceof ApiError && err.code === "unauthorized") {
          const go = await confirm({
            title: t("auth.required.title"),
            message: t("auth.required.announce"),
            cancelLabel: t("common.cancel"),
            confirmLabel: t("auth.required.signIn"),
          });
          if (go) {
            requireSignIn("/");
          }
          return;
        }
        await alert({
          title: t("map.alert.announceFailed.title"),
          message: apiErrorMessage(err, t),
          confirmLabel: t("common.ok"),
        });
      } finally {
        setAnnouncing(false);
      }
    },
    [alert, confirm, requireEmailVerified, requireSignIn, router, signedIn, t],
  );

  const openParkCar = useCallback(
    async (coords: [number, number] | null, label: string | null) => {
      const hasSession = signedIn && !!(await getAccessToken());
      const allowed = await passAuthGate({
        signedIn: hasSession,
        confirmSignIn: () =>
          confirm({
            title: t("auth.required.title"),
            message: t("auth.required.announce"),
            cancelLabel: t("common.cancel"),
            confirmLabel: t("auth.required.signIn"),
          }),
        onRequireSignIn: () => requireSignIn("/"),
      });
      if (!allowed) {
        return;
      }
      if (!(await requireEmailVerified())) {
        return;
      }
      setParking(true);
      try {
        const list = await listVehicles();
        if (list.length === 0) {
          let nextCoords = coords;
          let fromCurrent = false;
          if (!nextCoords) {
            const { currentLatLon } = await import("@/push/locationSeed");
            const here = await currentLatLon();
            if (here) {
              nextCoords = [here.longitude, here.latitude];
              fromCurrent = true;
            }
          }
          setParkCoords(nextCoords);
          setParkLabel(label);
          setParkFromCurrentLocation(fromCurrent);
          const add = await confirm({
            title: t("announce.needVehicle.title"),
            message: t("announce.needVehicle.message"),
            cancelLabel: t("common.cancel"),
            confirmLabel: t("announce.needVehicle.add"),
          });
          if (add) {
            router.push("/account/vehicles/new?from=park" as Href);
          }
          return;
        }
        let nextCoords = coords;
        let nextLabel = label;
        let fromCurrent = false;
        if (!nextCoords) {
          const { currentLatLon } = await import("@/push/locationSeed");
          const here = await currentLatLon();
          if (here) {
            nextCoords = [here.longitude, here.latitude];
            fromCurrent = true;
          }
        }
        setParkVehicles(list);
        setParkCoords(nextCoords);
        setParkLabel(nextLabel);
        setParkFromCurrentLocation(fromCurrent);
        setParkPickMode(false);
        setParkOpen(true);
      } catch (err) {
        await alert({
          title: t("map.alert.parkFailed.title"),
          message: apiErrorMessage(err, t),
          confirmLabel: t("common.ok"),
        });
      } finally {
        setParking(false);
      }
    },
    [alert, confirm, requireEmailVerified, requireSignIn, router, signedIn, t],
  );

  // After “add car” from the announce soft-gate, reopen the form with the new list.
  useEffect(() => {
    return subscribeVehicleCreated((kind) => {
      if (kind !== "announce") {
        return;
      }
      if (!consumeResumeAnnounceAfterVehicle()) {
        return;
      }
      void openAnnounce(null, null);
    });
  }, [openAnnounce]);

  // After “add car” from + Mi coche, reopen the park form (keep map pick / GPS).
  useEffect(() => {
    return subscribeVehicleCreated((kind) => {
      if (kind !== "park") {
        return;
      }
      if (!consumeResumeParkAfterVehicle()) {
        return;
      }
      void (async () => {
        try {
          const list = await listVehicles();
          setParkVehicles(list);
          setParkPickMode(false);
          setParkOpen(true);
        } catch (err) {
          await alert({
            title: t("map.alert.parkFailed.title"),
            message: apiErrorMessage(err, t),
            confirmLabel: t("common.ok"),
          });
        }
      })();
    });
  }, [alert, t]);

  // Deep-link from reservation history “re-announce” → open form prefilled.
  useEffect(() => {
    const lon = Number.parseFloat(String(focusParams.announceLon ?? ""));
    const lat = Number.parseFloat(String(focusParams.announceLat ?? ""));
    if (!mapReady || !Number.isFinite(lon) || !Number.isFinite(lat)) {
      return;
    }
    const labelRaw = focusParams.announceLabel ? String(focusParams.announceLabel) : "";
    const priceRaw = Number.parseInt(String(focusParams.announcePrice ?? ""), 10);
    const vehicleRaw = focusParams.announceVehicle ? String(focusParams.announceVehicle) : "";
    dispatchFollow({ type: "claim_camera" });
    cameraRef.current?.easeTo({
      center: [lon, lat],
      zoom: FOCUS_SPOT_ZOOM,
      duration: 500,
    });
    void openAnnounce([lon, lat], labelRaw || null, {
      priceCents: Number.isFinite(priceRaw) && priceRaw > 0 ? priceRaw : null,
      vehicleId: vehicleRaw || null,
    });
    router.setParams({
      announceLon: undefined,
      announceLat: undefined,
      announceLabel: undefined,
      announcePrice: undefined,
      announceVehicle: undefined,
    });
  }, [
    mapReady,
    focusParams.announceLon,
    focusParams.announceLat,
    focusParams.announceLabel,
    focusParams.announcePrice,
    focusParams.announceVehicle,
    openAnnounce,
    router,
  ]);

  const submitAnnouncement = useCallback(
    async (values: AnnounceValues) => {
      setAnnouncing(true);
      try {
        const spot = await announceAt(values.lon, values.lat, {
          guidePriceCents: values.guidePriceCents,
          preferredDepartureAt: values.preferredDepartureAt,
          autoCancelNoShow: values.autoCancelNoShow,
          leavingNow: values.leavingNow,
          vehicleId: values.vehicleId,
          notes: t("announce.notes.longPress"),
          addressHint: values.addressHint,
        });
        setAnnounceOpen(false);
        setAnnounceCoords(null);
        setAnnounceLabel(null);
        setAnnouncePriceCents(null);
        setAnnounceVehicleId(null);
        setAnnounceKeepForm(false);
        await afterAnnounce(spot, t("map.alert.announced.message"));
      } catch (err) {
        await alert({
          title: apiErrorTitle(err, t, "map.alert.announceFailed.title"),
          message: apiErrorMessage(err, t),
          confirmLabel: t("common.ok"),
        });
      } finally {
        setAnnouncing(false);
      }
    },
    [afterAnnounce, alert, t],
  );

  const onLongPress = useCallback(
    (event: NativeSyntheticEvent<PressEvent>) => {
      const [lon, lat] = event.nativeEvent.lngLat;
      void (async () => {
        const ok = await confirm({
          title: t("map.alert.parkHere.title"),
          message: t("map.alert.parkHere.message", {
            lat: lat.toFixed(5),
            lon: lon.toFixed(5),
          }),
          cancelLabel: t("common.cancel"),
          confirmLabel: t("map.alert.parkHere.confirm"),
        });
        if (ok) {
          await openParkCar([lon, lat], null);
        }
      })();
    },
    [confirm, openParkCar, t],
  );

  const centerOnRelevantSpot = useCallback(() => {
    const mine = mySpotFeatures.filter((f) => isLiveMapSpotStatus(f.properties.status));
    if (mine.length > 0) {
      const unpublished = mine.find((f) => f.properties.status === "unpublished");
      const pick =
        unpublished ??
        [...mine].sort((a, b) => {
          const aLeave = a.properties.leaving_now ? 0 : 1;
          const bLeave = b.properties.leaving_now ? 0 : 1;
          if (aLeave !== bLeave) {
            return aLeave - bLeave;
          }
          const aAt = a.properties.preferred_departure_at
            ? Date.parse(a.properties.preferred_departure_at)
            : Number.POSITIVE_INFINITY;
          const bAt = b.properties.preferred_departure_at
            ? Date.parse(b.properties.preferred_departure_at)
            : Number.POSITIVE_INFINITY;
          return aAt - bAt;
        })[0];
      if (!pick) {
        return;
      }
      const lon = Number(pick.geometry.coordinates[0]);
      const lat = Number(pick.geometry.coordinates[1]);
      if (!Number.isFinite(lon) || !Number.isFinite(lat)) {
        return;
      }
      dispatchFollow({ type: "claim_camera" });
      cameraRef.current?.easeTo({
        center: [lon, lat],
        zoom: FOCUS_SPOT_ZOOM,
        duration: 500,
      });
      return;
    }
    // Driver: accepted reservation with the nearest exchange time.
    if (isDriver && activeSpot) {
      const lon = Number(activeSpot.geometry.coordinates[0]);
      const lat = Number(activeSpot.geometry.coordinates[1]);
      if (!Number.isFinite(lon) || !Number.isFinite(lat)) {
        return;
      }
      dispatchFollow({ type: "claim_camera" });
      cameraRef.current?.easeTo({
        center: [lon, lat],
        zoom: FOCUS_SPOT_ZOOM,
        duration: 500,
      });
    }
  }, [mySpotFeatures, isDriver, activeSpot]);

  const showCenterSpotFab =
    mySpotFeatures.some((f) => isLiveMapSpotStatus(f.properties.status)) ||
    (!!isDriver && !!activeSpot);

  return (
    <View style={styles.fill}>
      {homeCenter == null ? (
        <View style={[styles.fill, styles.homeBoot]}>
          <ActivityIndicator color="#7EC8E3" />
        </View>
      ) : (
        <MapView
          ref={mapRef}
          style={styles.fill}
          mapStyle={mapStyleUrl}
          onDidFinishLoadingMap={() => {
            setMapReady(true);
            void publishViewport();
          }}
          onRegionDidChange={onRegionDidChange}
          onPress={onPressMap}
          onLongPress={onLongPress}
        >
          <Camera
            ref={cameraRef}
            initialViewState={{
              center: homeCenter,
              zoom: homeZoom,
            }}
          />
          {/*
            No layerIndex on overlays: on Android, MLRN inserts them below the
            native location component. High layerIndex was putting P icons above
            the puck. NativeUserLocation last so iOS also stacks it on top.
          */}
          {selected &&
          !selected.properties.exact_location &&
          selected.geometry.coordinates[0] != null &&
          selected.geometry.coordinates[1] != null ? (
            <UncertaintyCircle
              lon={selected.geometry.coordinates[0]}
              lat={selected.geometry.coordinates[1]}
            />
          ) : null}
          {spotsArmed ? (
            <SpotLayers data={spotData.others} onPressFeature={onPressFeature} />
          ) : null}
          {offeredArmed ? (
            <OfferedSpotLayers data={spotData.offered} onPressFeature={onPressFeature} />
          ) : null}
          {mineArmed ? (
            <MySpotLayers data={ownerExchangeMine} onPressFeature={onPressFeature} />
          ) : null}
          {exchangeData.features.length > 0 ? (
            <ExchangeLayers data={exchangeData} role="driver" onPressFeature={onPressFeature} />
          ) : null}
          {historyGhostData.features.length > 0 ? (
            <HistoryGhostLayers data={historyGhostData} />
          ) : null}
          {searchHits.length > 0 ? (
            <SearchPlaceLayers
              hits={searchHits}
              selectedId={selectedSearchId}
              onPressHit={onPressSearchHit}
            />
          ) : null}
          {(announceOpen || announcePickMode) && announceCoords ? (
            <AnnounceDraftLayers coords={announceCoords} />
          ) : null}
          {(parkOpen || parkPickMode) && parkCoords ? (
            <AnnounceDraftLayers coords={parkCoords} />
          ) : null}
          {puckReady ? <NativeUserLocation key={location.puckEpoch} mode="default" /> : null}
        </MapView>
      )}

      {!ready ? (
        <View style={[styles.banner, { top: insets.top + 8 }]} pointerEvents="none">
          <ActivityIndicator color="#F4F7FA" />
          <Text style={styles.bannerText}>{t("map.banner.checkingSession")}</Text>
        </View>
      ) : null}
      {ready && isLoading ? (
        <View style={[styles.banner, { top: insets.top + 8 }]} pointerEvents="none">
          <ActivityIndicator color="#F4F7FA" />
          <Text style={styles.bannerText}>{t("map.banner.loadingSpots")}</Text>
        </View>
      ) : null}

      <View style={[styles.topChrome, { top: insets.top + 8 }]}>
        {ready && !isLoading ? (
          <View style={styles.spotCountChip} pointerEvents="none">
            <Text style={styles.spotCountText}>
              {t("map.banner.spotCount", { count: collection.features.length })}
            </Text>
          </View>
        ) : (
          <View style={styles.spotCountPlaceholder} />
        )}
        <View style={styles.searchCluster}>
          <View style={styles.searchBar}>
            <TextInput
              ref={searchInputRef}
              style={styles.searchInput}
              value={searchQuery}
              onChangeText={onChangeSearchQuery}
              onPressIn={() => {
                if (!spotSheetOpen) {
                  return;
                }
                // Dismiss first; focus after unmount so the keyboard never
                // fights the form sheet (which expands the sheet violently).
                pendingSearchFocusRef.current = true;
                dismissSpotSheet();
              }}
              showSoftInputOnFocus={!spotSheetOpen}
              placeholder={t("map.search.placeholder")}
              placeholderTextColor="#7A93A0"
              returnKeyType="search"
              onSubmitEditing={() => void runMapSearch()}
              editable={!searchBusy}
            />
            <Pressable
              style={styles.searchBtn}
              disabled={searchBusy}
              onPress={() => void runMapSearch()}
            >
              {searchBusy ? (
                <ActivityIndicator color="#fff" size="small" />
              ) : (
                <Text style={styles.searchBtnText}>{t("map.search.button")}</Text>
              )}
            </Pressable>
            {searchHits.length > 0 || suggestHits.length > 0 || categoryHint ? (
              <Pressable onPress={clearSearchHits} accessibilityLabel={t("map.search.clear")}>
                <Ionicons name="close-circle" size={22} color="#9DB4C0" />
              </Pressable>
            ) : null}
          </View>
          {searchHits.length === 0 && (categoryHint || suggestHits.length > 0) ? (
            <View style={styles.searchSuggestPanel}>
              {categoryHint ? (
                <Pressable
                  style={styles.searchCategoryRow}
                  onPress={() => void runCategoryHintSearch(categoryHint)}
                  disabled={searchBusy}
                >
                  <Ionicons name="grid-outline" size={16} color="#00BBF9" />
                  <Text style={styles.searchCategoryText} numberOfLines={1}>
                    {t("map.search.categoryNear", {
                      category: categoryHint.label,
                    })}
                  </Text>
                </Pressable>
              ) : null}
              {suggestHits.slice(0, 3).map((hit) => (
                <Pressable
                  key={hit.id}
                  style={styles.searchSuggestRow}
                  onPress={() => onPressSuggestHit(hit)}
                >
                  <Text style={styles.searchSuggestText} numberOfLines={1}>
                    {hit.label}
                  </Text>
                </Pressable>
              ))}
            </View>
          ) : null}
          {searchHits.length > 0 ? (
            <ScrollView
              style={styles.searchResults}
              keyboardShouldPersistTaps="handled"
              nestedScrollEnabled
            >
              {searchHits.slice(0, 6).map((hit) => {
                const selected = hit.id === selectedSearchId;
                return (
                  <Pressable
                    key={hit.id}
                    style={[
                      styles.searchResultRow,
                      selected ? styles.searchResultRowSelected : null,
                    ]}
                    onPress={() => onPressSearchHit(hit.id)}
                  >
                    <Text style={styles.searchResultText} numberOfLines={2}>
                      {hit.label}
                    </Text>
                  </Pressable>
                );
              })}
              <Text style={styles.searchAttribution}>{t("map.search.attribution")}</Text>
            </ScrollView>
          ) : null}
        </View>
        {announcePickMode || parkPickMode ? (
          <View style={styles.pickBanner}>
            <Text style={styles.pickBannerText}>
              {searchHits.length > 0 ? t("map.search.pickHint") : t("announce.location.pickHint")}
            </Text>
            <Pressable
              style={styles.pickBannerBack}
              accessibilityRole="button"
              accessibilityLabel={t("announce.location.backToForm")}
              onPress={() => {
                if (parkPickMode) {
                  setParkPickMode(false);
                  setParkOpen(true);
                  return;
                }
                setAnnouncePickMode(false);
                setAnnounceOpen(true);
              }}
            >
              <Ionicons name="arrow-back" size={20} color="#F4F7FA" />
            </Pressable>
          </View>
        ) : null}
      </View>

      {active ? (
        <Pressable
          style={[styles.banner, styles.activeBanner, { top: insets.top + 118 }]}
          onPress={() => {
            if (activeSpot) {
              focusSpotOnMap(activeSpot);
            }
          }}
          accessibilityRole="button"
          accessibilityLabel={t("map.banner.openExchange")}
        >
          <View style={styles.activeBannerBody}>
            <Text style={styles.bannerText}>
              {t("map.banner.exchangeActive", {
                datetime: formatDateTime(active.exchange_at),
              })}
            </Text>
            <Text style={styles.bannerPeer}>
              {t(
                bannerPeerStatusKey({
                  res: active,
                  iAmOwner: isOwner,
                }),
              )}
            </Text>
            {(() => {
              const distance = peerEnRouteDistance(active, isOwner);
              if (!distance) {
                return null;
              }
              return (
                <Text style={styles.bannerPeerDistance}>
                  {distance.current
                    ? t("exchange.statusPanel.distanceCurrent", { meters: distance.meters })
                    : t("exchange.statusPanel.distanceStale", {
                        meters: distance.meters,
                        minutes: distance.ageMinutes,
                      })}
                </Text>
              );
            })()}
          </View>
          {(() => {
            const myEnRoute = isOwner
              ? active.owner_en_route_at
              : active.driver_en_route_at;
            const myReady = isOwner
              ? active.owner_ready_at
              : active.driver_ready_at;
            if (myReady) {
              return (
                <View style={styles.activeBannerActions}>
                  <Pressable
                    style={[styles.bannerBtn, busy && styles.bannerBtnDisabled]}
                    disabled={busy}
                    onPress={() => void clearReady()}
                  >
                    <Text style={styles.bannerBtnText}>
                      {t("map.banner.unready")}
                    </Text>
                  </Pressable>
                </View>
              );
            }
            return (
              <View style={styles.activeBannerActions}>
                {!myEnRoute ? (
                  <Pressable
                    style={[
                      styles.bannerBtn,
                      styles.bannerBtnSecondary,
                      busy && styles.bannerBtnDisabled,
                    ]}
                    disabled={busy}
                    onPress={() => void markEnRoute()}
                  >
                    <Text style={styles.bannerBtnText}>
                      {t("map.banner.enRoute")}
                    </Text>
                  </Pressable>
                ) : null}
                <Pressable
                  style={[styles.bannerBtn, busy && styles.bannerBtnDisabled]}
                  disabled={busy}
                  onPress={() => void markReady()}
                >
                  <Text style={styles.bannerBtnText}>
                    {t("map.banner.ready")}
                  </Text>
                </Pressable>
              </View>
            );
          })()}
        </Pressable>
      ) : leavingNowSpot ? (
        <View
          style={[styles.banner, styles.activeBanner, { top: insets.top + 118 }]}
        >
          <Pressable
            style={styles.activeBannerBody}
            onPress={() => {
              router.push(`/account/spots/${String(leavingNowSpot.id)}` as Href);
            }}
          >
            <Text style={styles.bannerText}>
              {t("map.banner.leavingNowWaiting", {
                count: leavingNowOfferCount,
              })}
            </Text>
          </Pressable>
          <View style={styles.activeBannerActions}>
            <Pressable
              style={styles.bannerBtn}
              onPress={() => {
                router.push(`/account/spots/${String(leavingNowSpot.id)}` as Href);
              }}
            >
              <Text style={styles.bannerBtnText}>{t("map.banner.leavingNowOffers")}</Text>
            </Pressable>
            <Pressable
              style={[styles.bannerBtn, { backgroundColor: "#3D1F2B" }]}
              onPress={() => {
                void (async () => {
                  const ok = await confirm({
                    title: t("map.banner.leavingNowLeave.title"),
                    message: t("map.banner.leavingNowLeave.message"),
                    cancelLabel: t("common.cancel"),
                    confirmLabel: t("map.banner.leavingNowLeave.confirm"),
                    destructive: true,
                  });
                  if (!ok) {
                    return;
                  }
                  try {
                    await withdrawSpot(String(leavingNowSpot.id));
                    await removeSpotFromMap(String(leavingNowSpot.id));
                  } catch (err) {
                    await alert({
                      title: t("map.alert.withdrawFailed.title"),
                      message: apiErrorMessage(err, t),
                      confirmLabel: t("common.ok"),
                    });
                  }
                })();
              }}
            >
              <Text style={styles.bannerBtnText}>{t("map.banner.leavingNowLeave.cta")}</Text>
            </Pressable>
          </View>
        </View>
      ) : null}

      {error ? (
        <View style={[styles.banner, { top: insets.top + 118 }]}>
          <Text style={styles.bannerText}>
            {error instanceof Error ? error.message : t("map.banner.loadSpotsFailed")}
          </Text>
        </View>
      ) : null}
      {sessionError ? (
        <View style={[styles.banner, { top: insets.top + 118 }]}>
          <Text style={styles.bannerText}>
            {t("map.banner.sessionError", { message: sessionError })}
          </Text>
          <Pressable onPress={() => void retrySession()}>
            <Text style={styles.link}>{t("map.banner.retry")}</Text>
          </Pressable>
        </View>
      ) : null}

      {showCenterSpotFab ? (
        <Pressable
          style={[styles.locateFab, styles.centerSpotFab, { bottom: 256 + insets.bottom }]}
          onPress={centerOnRelevantSpot}
          accessibilityRole="button"
          accessibilityLabel={t("map.fab.centerOnSpot")}
        >
          <Text style={styles.centerSpotP}>P</Text>
          <View style={styles.centerSpotLocate}>
            <Ionicons name="locate" size={12} color="#fff" />
          </View>
        </Pressable>
      ) : null}

      <Pressable
        style={[
          styles.locateFab,
          { bottom: 204 + insets.bottom },
          !follow.locationGranted ? styles.fabDisabled : null,
        ]}
        disabled={!follow.locationGranted}
        onPress={onRecenter}
        accessibilityRole="button"
        accessibilityLabel={t("map.fab.locateMe")}
      >
        <Ionicons name="locate" size={24} color="#fff" />
      </Pressable>

      <Pressable
        style={[styles.filterFab, { bottom: 152 + insets.bottom }]}
        onPress={() => {
          stageMapFilter(mapFilter);
          router.push("/filter" as Href);
        }}
        accessibilityRole="button"
        accessibilityLabel={t("map.filter.fab")}
        accessibilityHint={mapFilter.isCustom ? t("map.filter.fabHint") : undefined}
      >
        <Ionicons name="options-outline" size={22} color="#fff" />
        {mapFilter.isCustom ? <View style={styles.filterBadge} /> : null}
      </Pressable>

      <Pressable
        style={[styles.accountFab, { bottom: 100 + insets.bottom }]}
        onPress={() => {
          if (!signedIn) {
            requireSignIn("/account");
            return;
          }
          router.push("/account" as Href);
        }}
        accessibilityRole="button"
        accessibilityLabel={t("map.fab.account")}
      >
        <Ionicons name="person" size={22} color="#fff" />
      </Pressable>

      <Pressable
        style={[styles.fab, { bottom: 36 + insets.bottom }]}
        disabled={parking || !ready}
        onPress={() => {
          void openParkCar(null, null);
        }}
      >
        {parking ? (
          <ActivityIndicator color="#fff" />
        ) : (
          <Text style={styles.fabText}>{t("map.fab.announce")}</Text>
        )}
      </Pressable>

      <ParkCarModal
        visible={parkOpen}
        busy={parking}
        vehicles={parkVehicles}
        initialCoordinates={parkCoords}
        initialAddressLabel={parkLabel}
        initialFromCurrentLocation={parkFromCurrentLocation}
        onCancel={() => {
          setParkOpen(false);
          setParkPickMode(false);
        }}
        onPickOnMap={() => {
          setParkOpen(false);
          setParkPickMode(true);
          dispatchFollow({ type: "claim_camera" });
        }}
        onAddVehicle={() => {
          setParkOpen(false);
          router.push("/account/vehicles/new?from=park" as Href);
        }}
        onSubmit={async (values: ParkCarValues) => {
          setParking(true);
          try {
            const spot = await parkCarAt(values.lon, values.lat, {
              vehicleId: values.vehicleId,
              addressHint: values.addressHint,
            });
            setParkOpen(false);
            setSpotsArmed(true);
            setMineArmed(true);
            openSpotDetail(spot);
            await Promise.all([refetch(), refreshMySpotsOverlay()]);
            await alert({
              title: t("map.alert.parked.title"),
              message: t("map.alert.parked.message"),
              confirmLabel: t("common.ok"),
            });
          } catch (err) {
            await alert({
              title: apiErrorTitle(err, t, "map.alert.parkFailed.title"),
              message: apiErrorMessage(err, t),
              confirmLabel: t("common.ok"),
            });
          } finally {
            setParking(false);
          }
        }}
      />

      <AnnounceModal
        visible={announceOpen}
        busy={announcing}
        vehicles={announceVehicles}
        initialCoordinates={announceCoords}
        initialAddressLabel={announceLabel}
        initialGuidePriceCents={announcePriceCents}
        initialVehicleId={announceVehicleId}
        keepForm={announceKeepForm}
        onCancel={() => {
          setAnnounceOpen(false);
          setAnnouncePickMode(false);
          setAnnounceCoords(null);
          setAnnounceLabel(null);
          setAnnouncePriceCents(null);
          setAnnounceVehicleId(null);
          setAnnounceKeepForm(false);
        }}
        onPickOnMap={() => {
          setAnnounceKeepForm(true);
          setAnnounceOpen(false);
          setAnnouncePickMode(true);
          dispatchFollow({ type: "claim_camera" });
          if (announceCoords) {
            cameraRef.current?.easeTo({
              center: announceCoords,
              zoom: Math.max(userZoom, FOCUS_SPOT_ZOOM),
              duration: 400,
            });
          }
        }}
        onSubmit={submitAnnouncement}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  fill: { flex: 1 },
  homeBoot: {
    alignItems: "center",
    justifyContent: "center",
    backgroundColor: "#0B1F33",
  },
  topChrome: {
    position: "absolute",
    left: 12,
    right: 12,
    zIndex: 20,
    gap: 8,
  },
  searchCluster: {
    gap: 4,
  },
  spotCountChip: {
    alignSelf: "center",
    backgroundColor: "rgba(11,31,51,0.85)",
    paddingHorizontal: 14,
    paddingVertical: 8,
    borderRadius: 999,
  },
  spotCountText: {
    color: "#F4F7FA",
    fontSize: 13,
    fontWeight: "600",
  },
  spotCountPlaceholder: {
    height: 32,
  },
  searchBar: {
    flexDirection: "row",
    alignItems: "center",
    gap: 8,
    backgroundColor: "rgba(11,31,51,0.92)",
    borderRadius: 12,
    paddingHorizontal: 10,
    paddingVertical: 8,
  },
  searchInput: {
    flex: 1,
    color: "#F4F7FA",
    fontSize: 15,
    paddingVertical: 6,
    paddingHorizontal: 4,
  },
  searchBtn: {
    backgroundColor: "#E85D04",
    borderRadius: 8,
    paddingHorizontal: 12,
    paddingVertical: 8,
    minWidth: 72,
    alignItems: "center",
  },
  searchBtnText: { color: "#fff", fontWeight: "600", fontSize: 13 },
  searchResults: {
    maxHeight: 148,
    backgroundColor: "rgba(11,31,51,0.94)",
    borderRadius: 12,
    paddingVertical: 2,
  },
  searchSuggestPanel: {
    backgroundColor: "rgba(11,31,51,0.94)",
    borderRadius: 12,
    overflow: "hidden",
  },
  searchSuggestRow: {
    paddingHorizontal: 12,
    paddingVertical: 7,
    borderTopWidth: StyleSheet.hairlineWidth,
    borderTopColor: "rgba(157,180,192,0.2)",
  },
  searchSuggestText: {
    color: "#D6E2EA",
    fontSize: 13,
  },
  searchResultsTitle: {
    color: "#9DB4C0",
    fontSize: 12,
    fontWeight: "600",
    paddingHorizontal: 12,
    paddingBottom: 4,
  },
  searchResultRow: {
    paddingHorizontal: 12,
    paddingVertical: 6,
    borderTopWidth: StyleSheet.hairlineWidth,
    borderTopColor: "rgba(157,180,192,0.25)",
    flexDirection: "row",
    alignItems: "center",
    gap: 8,
  },
  searchResultRowSelected: {
    backgroundColor: "rgba(0,187,249,0.18)",
  },
  searchResultText: {
    color: "#F4F7FA",
    fontSize: 13,
    lineHeight: 16,
    flex: 1,
  },
  searchCategoryRow: {
    flexDirection: "row",
    alignItems: "center",
    gap: 8,
    paddingVertical: 8,
    paddingHorizontal: 12,
  },
  searchCategoryText: {
    color: "#00BBF9",
    fontSize: 13,
    fontWeight: "600",
    flex: 1,
  },
  searchAttribution: {
    color: "#7A93A0",
    fontSize: 10,
    paddingHorizontal: 12,
    paddingTop: 4,
    paddingBottom: 6,
  },
  banner: {
    position: "absolute",
    top: 108,
    alignSelf: "center",
    maxWidth: "92%",
    flexDirection: "row",
    gap: 8,
    alignItems: "center",
    backgroundColor: "rgba(11,31,51,0.85)",
    paddingHorizontal: 14,
    paddingVertical: 8,
    borderRadius: 999,
  },
  activeBanner: {
    top: 148,
    backgroundColor: "rgba(232,93,4,0.92)",
    maxWidth: "92%",
    borderRadius: 16,
    flexDirection: "column",
    alignItems: "stretch",
    gap: 10,
    paddingVertical: 12,
  },
  activeBannerBody: { gap: 2, paddingHorizontal: 2 },
  activeBannerActions: {
    alignItems: "center",
    gap: 8,
    width: "100%",
  },
  bannerBtn: {
    borderWidth: 1.5,
    borderColor: "#FFE8D6",
    backgroundColor: "rgba(255,255,255,0.16)",
    paddingHorizontal: 16,
    paddingVertical: 8,
    borderRadius: 10,
    minWidth: 160,
    alignItems: "center",
  },
  bannerBtnSecondary: {
    borderColor: "rgba(255,232,214,0.55)",
    backgroundColor: "rgba(255,255,255,0.08)",
  },
  bannerBtnDisabled: { opacity: 0.5 },
  bannerBtnText: {
    color: "#FFFFFF",
    fontSize: 14,
    fontWeight: "600",
  },
  bannerPeer: { color: "#FFE8D6", fontSize: 12, lineHeight: 16 },
  bannerPeerDistance: { color: "#FFE8D6", fontSize: 12, lineHeight: 16, fontWeight: "700" },
  pickBanner: {
    flexDirection: "row",
    alignItems: "center",
    gap: 10,
    backgroundColor: "rgba(27,154,170,0.95)",
    borderRadius: 14,
    paddingHorizontal: 12,
    paddingVertical: 10,
  },
  pickBannerText: {
    flex: 1,
    color: "#F4F7FA",
    fontSize: 13,
    lineHeight: 18,
  },
  pickBannerBack: {
    width: 36,
    height: 36,
    borderRadius: 18,
    backgroundColor: "rgba(11,31,51,0.35)",
    alignItems: "center",
    justifyContent: "center",
  },
  bannerText: { color: "#F4F7FA", fontSize: 13 },
  link: { color: "#fff", fontWeight: "700", fontSize: 13 },
  filterFab: {
    position: "absolute",
    right: 20,
    width: 48,
    height: 48,
    borderRadius: 24,
    backgroundColor: "#16324F",
    elevation: 4,
    alignItems: "center",
    justifyContent: "center",
  },
  filterBadge: {
    position: "absolute",
    top: 1,
    right: 1,
    width: 11,
    height: 11,
    borderRadius: 6,
    borderWidth: 2,
    borderColor: "#16324F",
    backgroundColor: "#E85D04",
  },
  accountFab: {
    position: "absolute",
    right: 20,
    width: 48,
    height: 48,
    borderRadius: 24,
    backgroundColor: "#16324F",
    elevation: 4,
    alignItems: "center",
    justifyContent: "center",
  },
  locateFab: {
    position: "absolute",
    right: 20,
    width: 48,
    height: 48,
    borderRadius: 24,
    backgroundColor: "#16324F",
    elevation: 4,
    alignItems: "center",
    justifyContent: "center",
  },
  fabDisabled: { opacity: 0.45 },
  centerSpotFab: {
    backgroundColor: "#1A73E8",
  },
  centerSpotP: {
    color: "#fff",
    fontSize: 22,
    fontWeight: "800",
  },
  centerSpotLocate: {
    position: "absolute",
    right: 4,
    bottom: 4,
    width: 16,
    height: 16,
    borderRadius: 8,
    backgroundColor: "#0B1F33",
    alignItems: "center",
    justifyContent: "center",
  },
  fab: {
    position: "absolute",
    right: 20,
    backgroundColor: "#1B9AAA",
    borderRadius: 999,
    paddingHorizontal: 18,
    paddingVertical: 14,
    elevation: 4,
  },
  fabText: { color: "#fff", fontWeight: "700", fontSize: 15 },
});
