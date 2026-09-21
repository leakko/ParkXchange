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
import { type Href, useLocalSearchParams, useRouter } from "expo-router";
import { useCallback, useEffect, useMemo, useReducer, useRef, useState } from "react";
import {
  ActivityIndicator,
  Alert,
  Pressable,
  StyleSheet,
  Text,
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
import { UncertaintyCircle } from "@/map/UncertaintyCircle";
import { partitionMapSpots } from "@/map/partitionMapSpots";
import {
  AnnounceModal,
  type AnnounceValues,
} from "@/map/AnnounceModal";
import { bannerPeerStatusKey } from "@/map/exchangeCopy";
import { SpotLayers } from "@/map/SpotLayers";
import { SpotSheet } from "@/map/SpotSheet";

const DEBOUNCE_MS = 350;
const FOCUS_SPOT_ZOOM = 17;

export default function MapScreen() {
  const { t, formatDateTime } = useTranslation();
  const router = useRouter();
  const focusParams = useLocalSearchParams<{
    focusLon?: string;
    focusLat?: string;
    focusSpot?: string;
  }>();
  const insets = useSafeAreaInsets();
  const mapRef = useRef<MapRef>(null);
  const cameraRef = useRef<CameraRef>(null);
  const sheetRef = useRef<BottomSheet>(null);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const jumpedToUserRef = useRef(false);
  const lastJumpCoordsRef = useRef<[number, number] | null>(null);
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
  const [announcePickMode, setAnnouncePickMode] = useState(false);

  const puckReady = locationComponentReady(
    follow.locationGranted,
    location.coords,
  );

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

  // Keep the sheet in sync when the viewport fetch flips exact_location
  // (e.g. after an offer is accepted and the driver becomes the holder).
  useEffect(() => {
    if (!selected?.id) {
      return;
    }
    const live = featureById(String(selected.id));
    if (!live) {
      return;
    }
    if (
      live.properties.exact_location !== selected.properties.exact_location ||
      live.geometry.coordinates[0] !== selected.geometry.coordinates[0] ||
      live.geometry.coordinates[1] !== selected.geometry.coordinates[1]
    ) {
      setSelected(live);
    }
  }, [featureById, selected]);

  // Prefer the exact active-exchange spot once it loads for the open sheet.
  useEffect(() => {
    if (!selected?.id || !activeSpot) {
      return;
    }
    if (String(activeSpot.id) !== String(selected.id)) {
      return;
    }
    if (
      activeSpot.properties.exact_location !== selected.properties.exact_location ||
      activeSpot.geometry.coordinates[0] !== selected.geometry.coordinates[0] ||
      activeSpot.geometry.coordinates[1] !== selected.geometry.coordinates[1]
    ) {
      setSelected(activeSpot);
    }
  }, [activeSpot, selected]);

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
  }, [collection.features, pendingOfferBySpotId, mySpotFeatures]);

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

  const onPressMap = useCallback(
    (event: NativeSyntheticEvent<PressEvent>) => {
      if (announcePickMode) {
        const [lon, lat] = event.nativeEvent.lngLat;
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
    [announcePickMode, t],
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
    async (coords: [number, number] | null, label: string | null) => {
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
        });
        setAnnounceOpen(false);
        setAnnounceCoords(null);
        setAnnounceLabel(null);
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
        {puckReady ? <NativeUserLocation mode="default" /> : null}
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
        <View style={styles.banner} pointerEvents="none">
          <ActivityIndicator color="#F4F7FA" />
          <Text style={styles.bannerText}>{t("map.banner.checkingSession")}</Text>
        </View>
      ) : null}
      {ready && isLoading ? (
        <View style={styles.banner} pointerEvents="none">
          <ActivityIndicator color="#F4F7FA" />
          <Text style={styles.bannerText}>{t("map.banner.loadingSpots")}</Text>
        </View>
      ) : null}
      {ready && !isLoading ? (
        <View style={styles.banner} pointerEvents="none">
          <Text style={styles.bannerText}>
            {t("map.banner.spotCount", { count: collection.features.length })}
          </Text>
        </View>
      ) : null}
      {active ? (
        <View style={[styles.banner, styles.activeBanner]}>
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
          <Pressable
            onPress={() => {
              if (activeSpot) {
                setSelected(activeSpot);
                sheetRef.current?.snapToIndex(1);
              }
            }}
          >
            <Text style={styles.link}>{t("map.banner.openExchange")}</Text>
          </Pressable>
          {!((isOwner && active.owner_en_route_at) ||
            (isDriver && active.driver_en_route_at)) ? (
            <Pressable
              disabled={busy}
              onPress={() => {
                void markEnRoute();
              }}
            >
              <Text style={styles.link}>{t("map.banner.enRoute")}</Text>
            </Pressable>
          ) : null}
        </View>
      ) : null}
      {announcePickMode ? (
        <View style={[styles.banner, styles.pickBanner]}>
          <Text style={styles.bannerText}>{t("announce.location.pickHint")}</Text>
          <Pressable
            onPress={() => {
              setAnnouncePickMode(false);
              setAnnounceOpen(true);
            }}
          >
            <Text style={styles.link}>{t("common.cancel")}</Text>
          </Pressable>
        </View>
      ) : null}
      {error ? (
        <View style={[styles.banner, { top: active ? 128 : 88 }]}>
          <Text style={styles.bannerText}>
            {error instanceof Error ? error.message : t("map.banner.loadSpotsFailed")}
          </Text>
        </View>
      ) : null}
      {sessionError ? (
        <View style={styles.banner}>
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
        onCancel={() => {
          setAnnounceOpen(false);
          setAnnouncePickMode(false);
          setAnnounceCoords(null);
          setAnnounceLabel(null);
        }}
        onPickOnMap={() => {
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
  banner: {
    position: "absolute",
    top: 48,
    alignSelf: "center",
    flexDirection: "row",
    gap: 8,
    alignItems: "center",
    backgroundColor: "rgba(11,31,51,0.85)",
    paddingHorizontal: 14,
    paddingVertical: 8,
    borderRadius: 999,
  },
  activeBanner: {
    top: 88,
    backgroundColor: "rgba(232,93,4,0.92)",
    maxWidth: "92%",
    borderRadius: 16,
    alignItems: "flex-start",
  },
  activeBannerBody: { flex: 1, gap: 2, paddingRight: 4 },
  bannerPeer: { color: "#FFE8D6", fontSize: 12, lineHeight: 16 },
  pickBanner: {
    top: 88,
    backgroundColor: "rgba(27,154,170,0.95)",
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
