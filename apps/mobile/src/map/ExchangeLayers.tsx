import {
  GeoJSONSource,
  Images,
  Layer,
} from "@maplibre/maplibre-react-native";
import type { FeatureCollection } from "geojson";

type Props = {
  data: FeatureCollection;
  onPressFeature: (id: string) => void;
  /** Owner keeps the person look; driver gets the parking P. */
  role: "owner" | "driver";
};

/**
 * Exact exchange pin for the caller's active reservation, with handshake badge.
 */
export function ExchangeLayers({ data, onPressFeature, role }: Props) {
  const sourceId = `spots-exchange-${role}`;
  const iconId = role === "owner" ? "spot-exchange-person" : "spot-exchange-p";

  return (
    <>
      <Images
        images={{
          "spot-exchange-person": require("../../assets/images/spot-mine-person.png"),
          "spot-exchange-p": require("../../assets/images/spot-parking-p.png"),
          "spot-handshake-badge": require("../../assets/images/spot-handshake-badge.png"),
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
        <Layer
          id={`${sourceId}-halo`}
          type="circle"
          source={sourceId}
          layerIndex={role === "owner" ? 912 : 916}
          paint={{
            "circle-color": role === "owner" ? "#1B9AAA" : "#1A73E8",
            "circle-radius": 18,
            "circle-opacity": 0.28,
          }}
        />
        <Layer
          id={`${sourceId}-icon`}
          type="symbol"
          source={sourceId}
          layerIndex={role === "owner" ? 913 : 917}
          layout={{
            "icon-image": iconId,
            "icon-size": 0.42,
            "icon-allow-overlap": true,
            "icon-ignore-placement": true,
          }}
        />
        <Layer
          id={`${sourceId}-handshake`}
          type="symbol"
          source={sourceId}
          layerIndex={role === "owner" ? 914 : 918}
          layout={{
            "icon-image": "spot-handshake-badge",
            "icon-size": 0.35,
            "icon-offset": [14, 14],
            "icon-allow-overlap": true,
            "icon-ignore-placement": true,
          }}
        />
      </GeoJSONSource>
    </>
  );
}
