import { Ionicons } from "@expo/vector-icons";
import BottomSheet from "@gorhom/bottom-sheet";
import {
  Camera,
  Map as MapView,
  NativeUserLocation,
  type CameraRef,
  type MapRef,
  type PressEvent,
  type ViewStateChangeEvent,
} from "@maplibre/maplibre-react-native";
import { type Href, useFocusEffect, useLocalSearchParams, useRouter } from "expo-router";
import { useCallback, useEffect, useMemo, useReducer, useRef, useState } from "react";
import {
  ActivityIndicator,
  Alert,
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
import {
  ApiError,
  createOffer,
  fetchActiveReservations,
  fetchMySpots,
  getSpot,
  listMyOffers,
  listVehicles,
  withdrawOffer,
  withdrawSpot,
} from "@/api/client";
import { apiErrorMessage } from "@/api/errors";
import { getAccessToken } from "@/api/session";
import { ensureEmailVerified } from "@/auth/requireEmailVerified";
import {
  defaultMapCenter,
  fallbackZoom,
  mapStyleUrl,
  userZoom,
} from "@/config";
import {
  defaultTimeWindow,
  useDiscovery,
  type Viewport,
} from "@/hooks/useDiscovery";

/** Fields the open sheet cares about — ignore referential churn from polls/WS. */
function sheetSpotDrifted(current: SpotFeature, next: SpotFeature): boolean {
  return (
    current.properties.status !== next.properties.status ||
    current.properties.exact_location !== next.properties.exact_location ||
    current.properties.price_cents !== next.properties.price_cents ||
    current.geometry.coordinates[0] !== next.geometry.coordinates[0] ||
    current.geometry.coordinates[1] !== next.geometry.coordinates[1]
  );
}
import { useSession } from "@/hooks/useSession";
import { useMapLocation } from "@/hooks/useMapLocation";
import { announceAt, useActiveReservation } from "@/hooks/useSpotActions";
import { useTranslation } from "@/i18n";
import {
  followReducer,
  initialFollowState,
  locationComponentReady,
} from "@/map/followUser";
import { ExchangeLayers } from "@/map/ExchangeLayers";
import { MySpotLayers } from "@/map/MySpotLayers";
import { OfferedSpotLayers } from "@/map/OfferedSpotLayers";
import { SearchPlaceLayers } from "@/map/SearchPlaceLayers";
import { UncertaintyCircle } from "@/map/UncertaintyCircle";
import { partitionMapSpots } from "@/map/partitionMapSpots";
import {
  AnnounceModal,
  type AnnounceValues,
} from "@/map/AnnounceModal";
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
import { bannerNextStep, bannerPeerStatusKey } from "@/map/exchangeCopy";
import { promptAlwaysLocationOnFirstOpen } from "@/push/locationPermissions";
import { SpotLayers } from "@/map/SpotLayers";
import { SpotSheet } from "@/map/SpotSheet";

const DEBOUNCE_MS = 350;
/** Longer than map pan debounce — typing must not hammer LocationIQ. */
const SUGGEST_DEBOUNCE_MS = 700;
const FOCUS_SPOT_ZOOM = 17;

export default function MapScreen() {
  const { t, formatDateTime, locale } = useTranslation();
  const router = useRouter();
  const focusParams = useLocalSearchParams<{
    focusLon?: string;
    focusLat?: string;
    focusSpot?: string;
    announceLon?: string;
    announceLat?: string;
    announceLabel?: string;
    announcePrice?: string;
    announceVehicle?: string;
  }>();
  const insets = useSafeAreaInsets();
  const mapRef = useRef<MapRef>(null);
  const cameraRef = useRef<CameraRef>(null);
  const sheetRef = useRef<BottomSheet>(null);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const suggestDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const jumpedToUserRef = useRef(false);
  const lastJumpCoordsRef = useRef<[number, number] | null>(null);
  const mapViewboxRef = useRef<ViewBox | null>(null);
  /** Query that produced the current searchHits — editing away clears results. */
  const lastSearchedQueryRef = useRef("");
  const timeWindow = useMemo(() => defaultTimeWindow(), []);

  const { ready, signedIn, error: sessionError, retry: retrySession } = useSession();
  const location = useMapLocation();
  const [follow, dispatchFollow] = useReducer(
    followReducer,
    undefined,
    initialFollowState,
  );
  const [viewport, setViewport] = useState<Viewport | null>(null);
  const [selected, setSelected] = useState<SpotFeature | null>(null);
  const [mapReady, setMapReady] = useState(false);
  const [spotsArmed, setSpotsArmed] = useState(false);
  const [mineArmed, setMineArmed] = useState(false);
  const [offeredArmed, setOfferedArmed] = useState(false);
  const [announcing, setAnnouncing] = useState(false);
  const [offerBusy, setOfferBusy] = useState(false);
  const [vehicles, setVehicles] = useState<VehicleResponse[]>([]);
  const [myOffers, setMyOffers] = useState<OfferResponse[]>([]);
  const [announceOpen, setAnnounceOpen] = useState(false);
  const [announceVehicles, setAnnounceVehicles] = useState<VehicleResponse[]>([]);
  const [announceCoords, setAnnounceCoords] = useState<[number, number] | null>(
    null,
  );
  const [announceLabel, setAnnounceLabel] = useState<string | null>(null);
  const [announcePriceCents, setAnnouncePriceCents] = useState<number | null>(
    null,
  );
  const [announceVehicleId, setAnnounceVehicleId] = useState<string | null>(
    null,
  );
  const [announcePickMode, setAnnouncePickMode] = useState(false);
  /** Keep form fields when returning from map pick / search. */
  const [announceKeepForm, setAnnounceKeepForm] = useState(false);
  const [searchQuery, setSearchQuery] = useState("");
  const [searchHits, setSearchHits] = useState<AddressSuggestion[]>([]);
  const [suggestHits, setSuggestHits] = useState<AddressSuggestion[]>([]);
  const [categoryHint, setCategoryHint] = useState<CategoryHint | null>(null);
  const [selectedSearchId, setSelectedSearchId] = useState<string | null>(null);
  const [searchBusy, setSearchBusy] = useState(false);
  const [mapViewbox, setMapViewbox] = useState<ViewBox | null>(null);
  mapViewboxRef.current = mapViewbox;

  const puckReady = locationComponentReady(
    follow.locationGranted,
    location.coords,
  );

  useEffect(() => {
    void (async () => {
      await promptAlwaysLocationOnFirstOpen((key) => t(key as Parameters<typeof t>[0]));
      await location.refresh();
    })();
    // Once on mount — t/location.refresh are stable enough for first-open.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const requireSignIn = useCallback(
    (returnTo: string = "/") => {
      router.push(
        `/auth/login?returnTo=${encodeURIComponent(returnTo)}` as Href,
      );
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

  const { collection, featureById, isLoading, error, refetch } = useDiscovery(
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
    cancel,
    refresh: refreshActiveReservation,
  } = useActiveReservation(signedIn);

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
      // Completed / cancelled / expired listings are history — keep them off the map.
      setMySpotFeatures(
        collection.features.filter((f) => {
          const status = f.properties.status;
          return (
            status === "available" ||
            status === "reserved" ||
            status === "handover"
          );
        }),
      );
    } catch {
      setMySpotFeatures([]);
    }
  }, [signedIn]);

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
      sheetRef.current?.close();
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

  const pendingOfferForSelected = selected
    ? (pendingOfferBySpotId.get(String(selected.id)) ?? null)
    : null;

  const mySpotsById = useMemo(() => {
    const map = new Map<string, SpotFeature>();
    for (const feature of mySpotFeatures) {
      map.set(String(feature.id), feature);
    }
    return map;
  }, [mySpotFeatures]);

  useEffect(() => {
    if (!signedIn) {
      setVehicles([]);
      return;
    }
    let cancelled = false;
    void (async () => {
      try {
        const list = await listVehicles();
        if (!cancelled) {
          setVehicles(list);
        }
      } catch {
        if (!cancelled) {
          setVehicles([]);
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [signedIn]);

  const refreshVehicles = useCallback(async () => {
    if (!signedIn) {
      setVehicles([]);
      return [] as VehicleResponse[];
    }
    try {
      const list = await listVehicles();
      setVehicles(list);
      return list;
    } catch {
      setVehicles([]);
      return [] as VehicleResponse[];
    }
  }, [signedIn]);

  useEffect(() => {
    if (!selected || !signedIn) {
      return;
    }
    void refreshVehicles();
  }, [selected, signedIn, refreshVehicles]);

  // Returning from /account/vehicles/new must refresh the list — invalidateQueries
  // alone does not update this screen's local vehicles state.
  useFocusEffect(
    useCallback(() => {
      if (signedIn) {
        void refreshVehicles();
      }
    }, [signedIn, refreshVehicles]),
  );

  // Discovery/WS must not overwrite the open sheet during a live exchange:
  // getSpot (exact, reserved/handover) and the viewport copy (often fuzzed or
  // a beat behind on status) used to thrash setSelected forever — the sheet
  // meta line flickering reserved ↔ handover/other until the app is killed.
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
        };
        geometry: { type: "Point"; coordinates: [number, number] };
      }
    >();

    for (const feature of collection.features) {
      const id = String(feature.id ?? "");
      // Active exchange owns this pin via ExchangeLayers (exact coords). Keeping
      // the discovery/offered copy fights the exact pin after accept.
      if (active && String(active.spot_id) === id) {
        continue;
      }
      byId.set(id, {
        type: "Feature",
        properties: {
          id,
          price_cents: feature.properties.price_cents,
          status: feature.properties.status,
          is_mine: Boolean(feature.properties.is_mine),
          has_my_offer: pendingOfferBySpotId.has(id),
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
      const status = feature.properties.status;
      if (
        status !== "available" &&
        status !== "reserved" &&
        status !== "handover"
      ) {
        continue;
      }
      const id = String(feature.id ?? "");
      if (active && String(active.spot_id) === id && isDriver && !isOwner) {
        continue;
      }
      byId.set(id, {
        type: "Feature",
        properties: {
          id,
          price_cents: feature.properties.price_cents,
          status,
          is_mine: true,
          has_my_offer: false,
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
  }, [
    collection.features,
    pendingOfferBySpotId,
    mySpotFeatures,
    active,
    isDriver,
    isOwner,
  ]);

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
    dispatchFollow(
      location.granted
        ? { type: "location_granted" }
        : { type: "location_denied" },
    );
  }, [location.ready, location.granted]);

  useEffect(() => {
    if (!location.coords || !follow.followUser) {
      return;
    }
    const [lon, lat] = location.coords;
    const prev = lastJumpCoordsRef.current;
    const movedFar =
      prev != null && Math.hypot(lon - prev[0], lat - prev[1]) > 0.5;
    if (jumpedToUserRef.current && !movedFar) {
      return;
    }
    cameraRef.current?.jumpTo({ center: location.coords, zoom: userZoom });
    jumpedToUserRef.current = true;
    lastJumpCoordsRef.current = location.coords;
  }, [location.coords, follow.followUser]);

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
    setViewport({
      bbox: bounds,
      zoom,
      from: timeWindow.from,
      to: timeWindow.to,
    });
    setMapViewbox(bounds as ViewBox);
  }, [timeWindow.from, timeWindow.to]);

  const onRegionDidChange = useCallback(
    (event: NativeSyntheticEvent<ViewStateChangeEvent>) => {
      if (event.nativeEvent.userInteraction) {
        dispatchFollow({ type: "user_gesture" });
      }
      if (debounceRef.current) {
        clearTimeout(debounceRef.current);
      }
      debounceRef.current = setTimeout(() => {
        void publishViewport();
      }, DEBOUNCE_MS);
    },
    [publishViewport],
  );

  const onPressFeature = useCallback(
    (id: string) => {
      void (async () => {
        const fromActive =
          activeSpot && String(activeSpot.id) === id ? activeSpot : null;
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
        setSelected(spot);
        const openExchange =
          !!fromActive ||
          spot.properties.status === "reserved" ||
          spot.properties.status === "handover";
        sheetRef.current?.snapToIndex(openExchange ? 1 : 0);
        if (openExchange) {
          void refreshActiveReservation();
        }
      })();
    },
    [featureById, mySpotsById, activeSpot, refreshActiveReservation],
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
      if (announcePickMode) {
        const [lon, lat] = event.nativeEvent.lngLat;
        clearSearchHits();
        setAnnounceCoords([lon, lat]);
        setAnnounceLabel(
          t("announce.location.coords", {
            lat: lat.toFixed(5),
            lon: lon.toFixed(5),
          }),
        );
        setAnnouncePickMode(false);
        setAnnounceOpen(true);
        return;
      }
      setSelected(null);
      sheetRef.current?.close();
    },
    [announcePickMode, clearSearchHits, t],
  );

  const onPressSearchHit = useCallback(
    (id: string) => {
      const hit = searchHits.find((h) => h.id === id);
      if (!hit) {
        return;
      }
      // Preview on the map; exact car position is chosen with announce pick / map.
      setSelectedSearchId(id);
      dispatchFollow({ type: "user_gesture" });
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
        Alert.alert(t("map.search.empty.title"), t("map.search.empty.message"));
        return;
      }
      dispatchFollow({ type: "user_gesture" });
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
    [t],
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
      const detail =
        err instanceof Error && err.message.trim()
          ? err.message
          : t("common.error");
      Alert.alert(t("map.search.failed"), detail);
    } finally {
      setSearchBusy(false);
    }
  }, [searchQuery, locale, resolveViewbox, applySearchResult, t]);

  const runCategoryHintSearch = useCallback(
    async (hint: CategoryHint) => {
      setSearchBusy(true);
      try {
        const viewbox = await resolveViewbox();
        const { hits, inViewport, shouldZoomOut } = await searchCategoryNearby(
          hint.osmTags,
          { viewbox, locale },
        );
        setSearchQuery(hint.label);
        applySearchResult(hits, inViewport, shouldZoomOut, hint.label);
      } catch (err) {
        const detail =
          err instanceof Error && err.message.trim()
            ? err.message
            : t("common.error");
        Alert.alert(t("map.search.failed"), detail);
      } finally {
        setSearchBusy(false);
      }
    },
    [resolveViewbox, locale, applySearchResult, t],
  );

  const onPressSuggestHit = useCallback(
    (hit: AddressSuggestion) => {
      applySearchResult([hit], [hit], false, hit.label);
      setSearchQuery(hit.label);
      setSelectedSearchId(hit.id);
      dispatchFollow({ type: "user_gesture" });
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
    void (async () => {
      const coords = await location.refresh();
      if (!coords) {
        return;
      }
      lastJumpCoordsRef.current = coords;
      cameraRef.current?.easeTo({
        center: coords,
        zoom: userZoom,
        duration: 400,
      });
    })();
  }, [location]);

  // Deep-link from "show my spot on map" in account → fly camera + open sheet.
  useEffect(() => {
    const lon = Number.parseFloat(String(focusParams.focusLon ?? ""));
    const lat = Number.parseFloat(String(focusParams.focusLat ?? ""));
    if (!mapReady || !Number.isFinite(lon) || !Number.isFinite(lat)) {
      return;
    }
    dispatchFollow({ type: "user_gesture" });
    cameraRef.current?.easeTo({
      center: [lon, lat],
      zoom: FOCUS_SPOT_ZOOM,
      duration: 500,
    });
    const spotId = focusParams.focusSpot
      ? String(focusParams.focusSpot)
      : null;
    if (spotId) {
      void (async () => {
        try {
          const spot = await getSpot(spotId);
          setSelected(spot);
          setMineArmed(true);
          setSpotsArmed(true);
          sheetRef.current?.snapToIndex(0);
        } catch {
          /* camera move is enough */
        }
      })();
    }
    router.setParams({
      focusLon: undefined,
      focusLat: undefined,
      focusSpot: undefined,
    });
  }, [
    mapReady,
    focusParams.focusLon,
    focusParams.focusLat,
    focusParams.focusSpot,
    router,
  ]);

  const afterAnnounce = useCallback(
    async (spot: SpotFeature, message: string) => {
      setSelected(spot);
      setSpotsArmed(true);
      setMineArmed(true);
      sheetRef.current?.snapToIndex(0);
      await Promise.all([refetch(), refreshMySpotsOverlay()]);
      Alert.alert(t("map.alert.announced.title"), message);
    },
    [refetch, refreshMySpotsOverlay, t],
  );

  const openAnnounce = useCallback(
    async (
      coords: [number, number] | null,
      label: string | null,
      extras?: { priceCents?: number | null; vehicleId?: string | null },
    ) => {
      if (!signedIn || !(await getAccessToken())) {
        requireSignIn("/");
        return;
      }
      if (!(await requireEmailVerified())) {
        return;
      }
      setAnnouncing(true);
      try {
        const list = await listVehicles();
        if (list.length === 0) {
          Alert.alert(
            t("announce.needVehicle.title"),
            t("announce.needVehicle.message"),
            [
              { text: t("common.cancel"), style: "cancel" },
              {
                text: t("announce.needVehicle.add"),
                onPress: () =>
                  router.push("/account/vehicles/new?from=announce" as Href),
              },
            ],
          );
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
          requireSignIn("/");
          return;
        }
        Alert.alert(
          t("map.alert.announceFailed.title"),
          apiErrorMessage(err, t),
        );
      } finally {
        setAnnouncing(false);
      }
    },
    [requireEmailVerified, requireSignIn, router, signedIn, t],
  );

  // Deep-link from reservation history “re-announce” → open form prefilled.
  useEffect(() => {
    const lon = Number.parseFloat(String(focusParams.announceLon ?? ""));
    const lat = Number.parseFloat(String(focusParams.announceLat ?? ""));
    if (!mapReady || !Number.isFinite(lon) || !Number.isFinite(lat)) {
      return;
    }
    const labelRaw = focusParams.announceLabel
      ? String(focusParams.announceLabel)
      : "";
    const priceRaw = Number.parseInt(String(focusParams.announcePrice ?? ""), 10);
    const vehicleRaw = focusParams.announceVehicle
      ? String(focusParams.announceVehicle)
      : "";
    dispatchFollow({ type: "user_gesture" });
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
        Alert.alert(
          t("map.alert.announceFailed.title"),
          apiErrorMessage(err, t),
        );
      } finally {
        setAnnouncing(false);
      }
    },
    [afterAnnounce, t],
  );

  const onLongPress = useCallback(
    (event: NativeSyntheticEvent<PressEvent>) => {
      const [lon, lat] = event.nativeEvent.lngLat;
      Alert.alert(
        t("map.alert.announceHere.title"),
        t("map.alert.announceHere.message", {
          lat: lat.toFixed(5),
          lon: lon.toFixed(5),
        }),
        [
          { text: t("common.cancel"), style: "cancel" },
          {
            text: t("map.alert.announceHere.confirm"),
            onPress: () => {
              void openAnnounce(
                [lon, lat],
                t("announce.location.coords", {
                  lat: lat.toFixed(5),
                  lon: lon.toFixed(5),
                }),
              );
            },
          },
        ],
      );
    },
    [openAnnounce, t],
  );

  const onEditSpot = useCallback(
    (spot: SpotFeature) => {
      const id = String(spot.id ?? "");
      if (!id) {
        return;
      }
      router.push(`/account/spots/${id}` as Href);
    },
    [router],
  );

  const onWithdrawSpot = useCallback(
    (spot: SpotFeature) => {
      const id = String(spot.id ?? "");
      if (!id) {
        return;
      }
      Alert.alert(
        t("map.alert.withdrawListing.title"),
        t("map.alert.withdrawListing.message"),
        [
          { text: t("common.cancel"), style: "cancel" },
          {
            text: t("map.alert.withdraw.confirm"),
            style: "destructive",
            onPress: () => {
              void (async () => {
                try {
                  await withdrawSpot(id);
                  setSelected(null);
                  sheetRef.current?.close();
                  await Promise.all([refetch(), refreshMySpotsOverlay()]);
                } catch (err) {
                  Alert.alert(
                    t("map.alert.withdrawFailed.title"),
                    err instanceof Error ? err.message : t("common.error"),
                  );
                }
              })();
            },
          },
        ],
      );
    },
    [refetch, t],
  );

  return (
    <View style={styles.fill}>
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
            center: defaultMapCenter,
            zoom: fallbackZoom,
          }}
          {...(follow.followUser && puckReady
            ? { trackUserLocation: "default" as const }
            : {})}
        />
        {puckReady ? (
          <NativeUserLocation key={location.puckEpoch} mode="default" />
        ) : null}
        {searchHits.length > 0 ? (
          <SearchPlaceLayers
            hits={searchHits}
            selectedId={selectedSearchId}
            onPressHit={onPressSearchHit}
          />
        ) : null}
        {spotsArmed ? (
          <SpotLayers data={spotData.others} onPressFeature={onPressFeature} />
        ) : null}
        {offeredArmed ? (
          <OfferedSpotLayers
            data={spotData.offered}
            onPressFeature={onPressFeature}
          />
        ) : null}
        {mineArmed ? (
          <MySpotLayers data={ownerExchangeMine} onPressFeature={onPressFeature} />
        ) : null}
        {exchangeData.features.length > 0 ? (
          <ExchangeLayers
            data={exchangeData}
            role="driver"
            onPressFeature={onPressFeature}
          />
        ) : null}
        {selected &&
        !selected.properties.exact_location &&
        selected.geometry.coordinates[0] != null &&
        selected.geometry.coordinates[1] != null ? (
          <UncertaintyCircle
            lon={selected.geometry.coordinates[0]}
            lat={selected.geometry.coordinates[1]}
          />
        ) : null}
      </MapView>

      {!ready ? (
        <View
          style={[styles.banner, { top: insets.top + 8 }]}
          pointerEvents="none"
        >
          <ActivityIndicator color="#F4F7FA" />
          <Text style={styles.bannerText}>{t("map.banner.checkingSession")}</Text>
        </View>
      ) : null}
      {ready && isLoading ? (
        <View
          style={[styles.banner, { top: insets.top + 8 }]}
          pointerEvents="none"
        >
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
        <View style={styles.searchBar}>
          <TextInput
            style={styles.searchInput}
            value={searchQuery}
            onChangeText={onChangeSearchQuery}
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
          {searchHits.length > 0 ||
          suggestHits.length > 0 ||
          categoryHint ? (
            <Pressable
              onPress={clearSearchHits}
              accessibilityLabel={t("map.search.clear")}
            >
              <Ionicons name="close-circle" size={22} color="#9DB4C0" />
            </Pressable>
          ) : null}
        </View>
        {searchHits.length === 0 &&
        (categoryHint || suggestHits.length > 0) ? (
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
            <Text style={styles.searchAttribution}>
              {t("map.search.attribution")}
            </Text>
          </ScrollView>
        ) : null}
        {announcePickMode ? (
          <View style={styles.pickBanner}>
            <Text style={styles.pickBannerText}>
              {searchHits.length > 0
                ? t("map.search.pickHint")
                : t("announce.location.pickHint")}
            </Text>
            <Pressable
              style={styles.pickBannerBack}
              accessibilityRole="button"
              accessibilityLabel={t("announce.location.backToForm")}
              onPress={() => {
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
              setSelected(activeSpot);
              sheetRef.current?.snapToIndex(1);
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
          </View>
          {(() => {
            const next = bannerNextStep({ res: active, iAmOwner: isOwner });
            return (
              <View style={styles.activeBannerActions}>
                <Pressable
                  style={[styles.bannerBtn, busy && styles.bannerBtnDisabled]}
                  disabled={busy}
                  onPress={() => {
                    if (next.action === "en_route") {
                      void markEnRoute();
                    } else if (next.action === "ready") {
                      void markReady();
                    } else {
                      void clearReady();
                    }
                  }}
                >
                  <Text style={styles.bannerBtnText}>{t(next.labelKey)}</Text>
                </Pressable>
              </View>
            );
          })()}
        </Pressable>
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

      <Pressable
        style={[styles.accountFab, { bottom: 164 + insets.bottom }]}
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
        style={[
          styles.locateFab,
          { bottom: 100 + insets.bottom },
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
        style={[styles.fab, { bottom: 36 + insets.bottom }]}
        disabled={announcing || !ready}
        onPress={() => {
          if (!signedIn) {
            requireSignIn("/");
            return;
          }
          void openAnnounce(null, null);
        }}
      >
        {announcing ? (
          <ActivityIndicator color="#fff" />
        ) : (
          <Text style={styles.fabText}>{t("map.fab.announce")}</Text>
        )}
      </Pressable>

      <SpotSheet
        ref={sheetRef}
        spot={selected}
        active={active}
        pendingOffer={pendingOfferForSelected}
        vehicles={vehicles}
        isOwner={isOwner}
        isDriver={isDriver}
        busy={busy || offerBusy}
        onMakeOffer={async (spot, vehicleId, exchangeAt, amountCents) => {
          if (!signedIn) {
            requireSignIn("/");
            return;
          }
          if (!(await requireEmailVerified())) {
            return;
          }
          const spotId = String(spot.id ?? "");
          setOfferBusy(true);
          try {
            await createOffer(spotId, {
              vehicle_id: vehicleId,
              exchange_at: exchangeAt,
              amount_cents: amountCents,
            });
            Alert.alert(t("map.alert.offerSent.title"), t("map.alert.offerSent.message"));
            await Promise.all([
              refetch(),
              refreshMyOffers(),
              refreshMySpotsOverlay(),
              refreshActiveReservation(),
            ]);
          } catch (err) {
            Alert.alert(t("map.alert.offerFailed.title"), apiErrorMessage(err, t));
          } finally {
            setOfferBusy(false);
          }
        }}
        onWithdrawOffer={async (offer) => {
          setOfferBusy(true);
          try {
            await withdrawOffer(offer.id);
            await Promise.all([refreshMyOffers(), refreshMySpotsOverlay()]);
          } catch (err) {
            Alert.alert(
              t("spotSheet.offer.withdrawFailed.title"),
              apiErrorMessage(err, t),
            );
            throw err;
          } finally {
            setOfferBusy(false);
          }
        }}
        onAddVehicle={() => {
          sheetRef.current?.close();
          router.push("/account/vehicles/new?from=offer" as Href);
        }}
        onEnRoute={() => void markEnRoute()}
        onReady={() => void markReady()}
        onUnready={() => void clearReady()}
        onCancel={() => {
          void cancel().then(() => {
            void refreshActiveReservation();
            void refreshMySpotsOverlay();
          });
        }}
        onEdit={onEditSpot}
        onViewOffers={onEditSpot}
        onWithdraw={onWithdrawSpot}
        onManageExchange={() => {
          void (async () => {
            await refreshActiveReservation();
            const spotId = String(selected?.id ?? "");
            try {
              const list = await fetchActiveReservations();
              const match = spotId
                ? list.find((r) => String(r.spot_id) === spotId)
                : list[0];
              if (match) {
                sheetRef.current?.close();
                router.push(`/account/reservations/${match.id}` as Href);
                return;
              }
            } catch {
              /* fall through */
            }
            sheetRef.current?.close();
            router.push("/account/reservations" as Href);
          })();
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
        }}
        onSubmit={submitAnnouncement}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  fill: { flex: 1 },
  topChrome: {
    position: "absolute",
    left: 12,
    right: 12,
    zIndex: 20,
    gap: 12,
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
  bannerBtnDisabled: { opacity: 0.5 },
  bannerBtnText: {
    color: "#FFFFFF",
    fontSize: 14,
    fontWeight: "600",
  },
  bannerPeer: { color: "#FFE8D6", fontSize: 12, lineHeight: 16 },
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
