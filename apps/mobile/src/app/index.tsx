import BottomSheet from "@gorhom/bottom-sheet";
import { Camera, Map, type MapRef } from "@maplibre/maplibre-react-native";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ActivityIndicator, StyleSheet, Text, View } from "react-native";

import type { SpotFeature } from "@/api/client";
import { barcelonaCenter, mapStyleUrl } from "@/config";
import {
  defaultTimeWindow,
  useDiscovery,
  type Viewport,
} from "@/hooks/useDiscovery";
import { useDevSession } from "@/hooks/useDevSession";
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

  const { collection, featureById, isLoading, error } = useDiscovery(
    viewport,
    ready,
  );

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
      {error ? (
        <View style={[styles.banner, { top: 88 }]}>
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

      <SpotSheet ref={sheetRef} spot={selected} />
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
  bannerText: { color: "#F4F7FA", fontSize: 13 },
});
