import BottomSheet from "@gorhom/bottom-sheet";
import {
  Camera,
  Map,
  type MapRef,
  type PressEvent,
} from "@maplibre/maplibre-react-native";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  ActivityIndicator,
  Alert,
  Pressable,
  StyleSheet,
  Text,
  View,
  type NativeSyntheticEvent,
} from "react-native";

import type { SpotFeature } from "@/api/client";
import { barcelonaCenter, mapStyleUrl } from "@/config";
import {
  defaultTimeWindow,
  useDiscovery,
  type Viewport,
} from "@/hooks/useDiscovery";
import { useDevSession } from "@/hooks/useDevSession";
import {
  announceAt,
  announceHere,
  useActiveReservation,
} from "@/hooks/useSpotActions";
import { SpotLayers } from "@/map/SpotLayers";
import { SpotSheet } from "@/map/SpotSheet";

const DEBOUNCE_MS = 350;

export default function MapScreen() {
  const mapRef = useRef<MapRef>(null);
  const sheetRef = useRef<BottomSheet>(null);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const timeWindow = useMemo(() => defaultTimeWindow(), []);

  const { ready, error: sessionError } = useDevSession();
  const [viewport, setViewport] = useState<Viewport | null>(null);
  const [selected, setSelected] = useState<SpotFeature | null>(null);
  const [mapReady, setMapReady] = useState(false);
  const [spotsArmed, setSpotsArmed] = useState(false);
  const [announcing, setAnnouncing] = useState(false);

  const { collection, featureById, isLoading, error, refetch } = useDiscovery(
    viewport,
    ready,
  );
  const {
    active,
    busy,
    claim,
    reconfirm,
    complete,
    cancel,
    refresh: refreshActive,
  } = useActiveReservation(ready);

  const spotData = useMemo(
    () => ({
      type: "FeatureCollection" as const,
      features: collection.features.map((feature) => ({
        type: "Feature" as const,
        properties: {
          id: String(feature.id ?? ""),
          price_cents: feature.properties.price_cents,
          status: feature.properties.status,
        },
        geometry: {
          type: "Point" as const,
          coordinates: [
            Number(feature.geometry.coordinates[0]),
            Number(feature.geometry.coordinates[1]),
          ] as [number, number],
        },
      })),
    }),
    [collection.features],
  );

  useEffect(() => {
    if (mapReady && spotData.features.length > 0) {
      setSpotsArmed(true);
    }
  }, [mapReady, spotData.features.length]);

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

  const onRegionDidChange = useCallback(() => {
    if (debounceRef.current) {
      clearTimeout(debounceRef.current);
    }
    debounceRef.current = setTimeout(() => {
      void publishViewport();
    }, DEBOUNCE_MS);
  }, [publishViewport]);

  const onPressFeature = useCallback(
    (id: string) => {
      const spot = featureById(id);
      setSelected(spot);
      sheetRef.current?.snapToIndex(0);
    },
    [featureById],
  );

  const doAnnounceHere = useCallback(async () => {
    Alert.alert("Announce a spot", "When does it become available?", [
      { text: "Cancel", style: "cancel" },
      {
        text: "Now",
        onPress: () => {
          void (async () => {
            setAnnouncing(true);
            try {
              const spot = await announceHere({
                priceCents: 150,
                durationMinutes: 30,
                availableInMinutes: 0,
              });
              setSelected(spot);
              setSpotsArmed(true);
              sheetRef.current?.snapToIndex(0);
              await refetch();
              Alert.alert("Announced", "Your spot is on the map.");
            } catch (err) {
              Alert.alert(
                "Announce failed",
                err instanceof Error ? err.message : "error",
              );
            } finally {
              setAnnouncing(false);
            }
          })();
        },
      },
      {
        text: "In 1 hour",
        onPress: () => {
          void (async () => {
            setAnnouncing(true);
            try {
              const spot = await announceHere({
                priceCents: 150,
                durationMinutes: 30,
                availableInMinutes: 60,
              });
              setSelected(spot);
              setSpotsArmed(true);
              sheetRef.current?.snapToIndex(0);
              await refetch();
              Alert.alert("Announced", "Your future spot is on the map.");
            } catch (err) {
              Alert.alert(
                "Announce failed",
                err instanceof Error ? err.message : "error",
              );
            } finally {
              setAnnouncing(false);
            }
          })();
        },
      },
    ]);
  }, [refetch]);

  const onLongPress = useCallback(
    (event: NativeSyntheticEvent<PressEvent>) => {
      const [lon, lat] = event.nativeEvent.lngLat;
      Alert.alert(
        "Announce here?",
        `Publish a spot at ${lat.toFixed(5)}, ${lon.toFixed(5)}`,
        [
          { text: "Cancel", style: "cancel" },
          {
            text: "Announce",
            onPress: () => {
              void (async () => {
                setAnnouncing(true);
                try {
                  const spot = await announceAt(lon, lat, {
                    priceCents: 150,
                    durationMinutes: 30,
                  });
                  setSelected(spot);
                  setSpotsArmed(true);
                  sheetRef.current?.snapToIndex(0);
                  await refetch();
                } catch (err) {
                  Alert.alert(
                    "Announce failed",
                    err instanceof Error ? err.message : "error",
                  );
                } finally {
                  setAnnouncing(false);
                }
              })();
            },
          },
        ],
      );
    },
    [refetch],
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
        onLongPress={onLongPress}
      >
        <Camera initialViewState={{ center: barcelonaCenter, zoom: 14 }} />
        {spotsArmed ? (
          <SpotLayers data={spotData} onPressFeature={onPressFeature} />
        ) : null}
      </Map>

      {(!ready || isLoading) && (
        <View style={styles.banner} pointerEvents="none">
          <ActivityIndicator color="#F4F7FA" />
          <Text style={styles.bannerText}>
            {!ready ? "Signing in..." : "Loading spots..."}
          </Text>
        </View>
      )}
      {ready && !isLoading ? (
        <View style={styles.banner} pointerEvents="none">
          <Text style={styles.bannerText}>
            {collection.features.length} spots
          </Text>
        </View>
      ) : null}
      {active ? (
        <View style={[styles.banner, styles.activeBanner]}>
          <Text style={styles.bannerText}>
            Active claim · reconfirm by{" "}
            {new Date(active.reconfirm_by).toLocaleTimeString()}
          </Text>
          <Pressable onPress={() => void reconfirm()} disabled={busy}>
            <Text style={styles.link}>Reconfirm</Text>
          </Pressable>
        </View>
      ) : null}
      {error ? (
        <View style={[styles.banner, { top: active ? 128 : 88 }]}>
          <Text style={styles.bannerText}>
            {error instanceof Error ? error.message : "failed to load spots"}
          </Text>
        </View>
      ) : null}
      {sessionError ? (
        <View style={styles.banner}>
          <Text style={styles.bannerText}>Session: {sessionError}</Text>
        </View>
      ) : null}

      <Pressable
        style={styles.fab}
        disabled={announcing || !ready}
        onPress={() => void doAnnounceHere()}
      >
        {announcing ? (
          <ActivityIndicator color="#fff" />
        ) : (
          <Text style={styles.fabText}>+ Announce</Text>
        )}
      </Pressable>

      <SpotSheet
        ref={sheetRef}
        spot={selected}
        active={active}
        busy={busy}
        onClaim={(spot) => {
          void claim(spot).then(() => refreshActive());
        }}
        onReconfirm={() => void reconfirm()}
        onComplete={() => void complete()}
        onCancel={() => void cancel()}
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
  bannerText: { color: "#F4F7FA", fontSize: 13 },
  link: { color: "#fff", fontWeight: "700", fontSize: 13 },
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
