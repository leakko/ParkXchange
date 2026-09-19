import { GeoJSONSource, Layer } from "@maplibre/maplibre-react-native";
import type { FeatureCollection } from "geojson";

type Props = {
  data: FeatureCollection;
  onPressFeature: (id: string) => void;
  /** Owner keeps the teal “mine” look; driver gets a distinct exchange pin. */
  role: "owner" | "driver";
};

/**
 * Exact exchange pin for the caller's active reservation.
 * Only parties receive these coordinates (via getSpot / active reservation).
 */
export function ExchangeLayers({ data, onPressFeature, role }: Props) {
  const color = role === "owner" ? "#1B9AAA" : "#E76F51";

  return (
    <GeoJSONSource
      id={`spots-exchange-${role}`}
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
      <Layer
        id={`spots-exchange-${role}-halo`}
        type="circle"
        source={`spots-exchange-${role}`}
        layerIndex={role === "owner" ? 910 : 913}
        paint={{
          "circle-color": color,
          "circle-radius": 18,
          "circle-opacity": 0.28,
        }}
      />
      <Layer
        id={`spots-exchange-${role}-points`}
        type="circle"
        source={`spots-exchange-${role}`}
        layerIndex={role === "owner" ? 911 : 914}
        paint={{
          "circle-color": color,
          "circle-radius": 12,
          "circle-stroke-width": 2.5,
          "circle-stroke-color": "#ffffff",
        }}
      />
    </GeoJSONSource>
  );
}
