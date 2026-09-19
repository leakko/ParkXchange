import { Ionicons } from "@expo/vector-icons";
import BottomSheet from "@gorhom/bottom-sheet";
import {
  Camera,
  Map,
  NativeUserLocation,
  type CameraRef,
  type MapRef,
  type PressEvent,
  type ViewStateChangeEvent,
} from "@maplibre/maplibre-react-native";
import { type Href, useRouter } from "expo-router";
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

import type { SpotFeature, VehicleResponse } from "@/api/client";
import { ApiError, createOffer, listVehicles, withdrawSpot } from "@/api/client";
import { apiErrorMessage } from "@/api/errors";
import { getAccessToken } from "@/api/session";
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
import { MySpotLayers } from "@/map/MySpotLayers";
import { UncertaintyCircle } from "@/map/UncertaintyCircle";
import { partitionMapSpots } from "@/map/partitionMapSpots";
import {
  AnnounceModal,
  type AnnounceValues,
} from "@/map/AnnounceModal";
import { SpotLayers } from "@/map/SpotLayers";
import { SpotSheet } from "@/map/SpotSheet";

const DEBOUNCE_MS = 350;

export default function MapScreen() {
  const { t, formatDateTime } = useTranslation();
  const router = useRouter();
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
  const [announcing, setAnnouncing] = useState(false);
  const [offerBusy, setOfferBusy] = useState(false);
  const [vehicles, setVehicles] = useState<VehicleResponse[]>([]);
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
    markOwnerReady,
    markDriverArrived,
    markDriverReady,
    cancel,
  } = useActiveReservation(signedIn);

  useEffect(() => {
    if (!signedIn) {
      setVehicles([]);
      return;
    }
    void listVehicles().then(setVehicles).catch(() => setVehicles([]));
  }, [signedIn]);

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

  const spotData = useMemo(() => {
    const features = collection.features.map((feature) => ({
      type: "Feature" as const,
      properties: {
        id: String(feature.id ?? ""),
        price_cents: feature.properties.price_cents,
        status: feature.properties.status,
        is_mine: Boolean(feature.properties.is_mine),
      },
      geometry: {
        type: "Point" as const,
        coordinates: [
          Number(feature.geometry.coordinates[0]),
          Number(feature.geometry.coordinates[1]),
        ] as [number, number],
      },
    }));
    const { mine, others } = partitionMapSpots(features);
    return {
      others: { type: "FeatureCollection" as const, features: others },
      mine: { type: "FeatureCollection" as const, features: mine },
    };
  }, [collection.features]);

  useEffect(() => {
    if (mapReady && spotData.others.features.length > 0) {
      setSpotsArmed(true);
    }
  }, [mapReady, spotData.others.features.length]);

  useEffect(() => {
    if (mapReady && spotData.mine.features.length > 0) {
      setMineArmed(true);
    }
  }, [mapReady, spotData.mine.features.length]);

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
      const spot = featureById(id);
      setSelected(spot);
      sheetRef.current?.snapToIndex(0);
    },
    [featureById],
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

  const afterAnnounce = useCallback(
    async (spot: SpotFeature, message: string) => {
      setSelected(spot);
      setSpotsArmed(true);
      setMineArmed(true);
      sheetRef.current?.snapToIndex(0);
      await refetch();
      Alert.alert(t("map.alert.announced.title"), message);
    },
    [refetch, t],
  );

  const openAnnounce = useCallback(
    async (coords: [number, number] | null, label: string | null) => {
      if (!signedIn || !(await getAccessToken())) {
        requireSignIn("/");
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
    [requireSignIn, router, signedIn, t],
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
                  await refetch();
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
      <Map
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
        {mineArmed ? (
          <MySpotLayers data={spotData.mine} onPressFeature={onPressFeature} />
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
      </Map>

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
          <Text style={styles.bannerText}>
            {t("map.banner.exchangeActive", {
              datetime: formatDateTime(active.exchange_at),
            })}
          </Text>
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
        style={styles.accountFab}
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
        style={styles.fab}
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
        vehicles={vehicles}
        isOwner={isOwner}
        isDriver={isDriver}
        busy={busy || offerBusy}
        onMakeOffer={async (spot, vehicleId, exchangeAt, amountCents) => {
          if (!signedIn) {
            requireSignIn("/");
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
            await refetch();
          } catch (err) {
            Alert.alert(
              t("map.alert.offerFailed.title"),
              err instanceof Error ? err.message : t("common.error"),
            );
          } finally {
            setOfferBusy(false);
          }
        }}
        onOwnerReady={() => void markOwnerReady()}
        onDriverArrived={() => void markDriverArrived()}
        onDriverReady={() => void markDriverReady()}
        onCancel={() => void cancel()}
        onEdit={onEditSpot}
        onWithdraw={onWithdrawSpot}
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
  },
  pickBanner: {
    top: 88,
    backgroundColor: "rgba(27,154,170,0.95)",
  },
  bannerText: { color: "#F4F7FA", fontSize: 13 },
  link: { color: "#fff", fontWeight: "700", fontSize: 13 },
  accountFab: {
    position: "absolute",
    right: 20,
    bottom: 164,
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
    bottom: 100,
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
    bottom: 36,
    backgroundColor: "#1B9AAA",
    borderRadius: 999,
    paddingHorizontal: 18,
    paddingVertical: 14,
    elevation: 4,
  },
  fabText: { color: "#fff", fontWeight: "700", fontSize: 15 },
});
