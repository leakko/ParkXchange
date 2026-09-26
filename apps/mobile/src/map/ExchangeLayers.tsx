import {
  GeoJSONSource,
  Images,
  Layer,
} from "@maplibre/maplibre-react-native";
import type { FeatureCollection } from "geojson";

import { MapPulseRingLayer } from "@/map/MapPulseRingLayer";

type Props = {
  data: FeatureCollection;
  onPressFeature: (id: string) => void;
  /** Owner keeps the person look; driver gets the parking P. */
  role: "owner" | "driver";
};

/**
 * Exact exchange pin for the caller's active reservation.
 * Pulsing ring marks the meeting point without a second badge icon.
 */
export function ExchangeLayers({ data, onPressFeature, role }: Props) {
  const sourceId = `spots-exchange-${role}`;
  const iconId = role === "owner" ? "spot-exchange-person" : "spot-exchange-p";
  const accent = role === "owner" ? "#1B9AAA" : "#1A73E8";

  return (
    <>
      <Images
        images={{
          "spot-exchange-person": require("../../assets/images/spot-mine-person.png"),
          "spot-exchange-p": require("../../assets/images/spot-parking-p.png"),
        }}
      />
      <GeoJSONSource
        id={sourceId}
        data={data}
        onPress={(event) => {
          event.stopPropagation();
          const feature = event.nativeEvent.features[0];
          if (!feature) {
            return;
          }
          const properties = feature.properties as Record<string, unknown> | null;
          const id = String(properties?.id ?? feature.id ?? "");
          if (id) {
            onPressFeature(id);
          }
        }}
      >
        <MapPulseRingLayer
          id={`${sourceId}-pulse`}
          sourceId={sourceId}
          color={accent}
          enabled={data.features.length > 0}
        />
        <Layer
          id={`${sourceId}-halo`}
          type="circle"
          source={sourceId}
          paint={{
            "circle-color": accent,
            "circle-radius": 14,
            "circle-opacity": 1,
            "circle-stroke-width": 2.5,
            "circle-stroke-color": "#ffffff",
          }}
        />
        <Layer
          id={`${sourceId}-icon`}
          type="symbol"
          source={sourceId}
          layout={{
            "icon-image": iconId,
            "icon-size": role === "owner" ? 0.32 : 0.28,
            "icon-allow-overlap": true,
            "icon-ignore-placement": true,
          }}
        />
      </GeoJSONSource>
    </>
  );
}
