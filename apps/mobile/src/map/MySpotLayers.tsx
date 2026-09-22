import {
  GeoJSONSource,
  Images,
  Layer,
} from "@maplibre/maplibre-react-native";
import type { FeatureCollection } from "geojson";

type Props = {
  data: FeatureCollection;
  onPressFeature: (id: string) => void;
};

/**
 * Unclustered markers for the signed-in user's own spots (car icon). Kept on a
 * separate source so they are never absorbed into the orange cluster bubbles.
 */
export function MySpotLayers({ data, onPressFeature }: Props) {
  return (
    <>
      <Images
        images={{
          "spot-mine-car": require("../../assets/images/spot-mine-car.png"),
        }}
      />
      <GeoJSONSource
        id="spots-mine"
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
          id="spots-mine-points"
          type="circle"
          source="spots-mine"
          layerIndex={904}
          paint={{
            "circle-color": "#1B9AAA",
            "circle-radius": 11,
            "circle-stroke-width": 2,
            "circle-stroke-color": "#ffffff",
          }}
        />
        <Layer
          id="spots-mine-icon"
          type="symbol"
          source="spots-mine"
          layerIndex={905}
          layout={{
            "icon-image": "spot-mine-car",
            "icon-size": 0.35,
            "icon-allow-overlap": true,
            "icon-ignore-placement": true,
          }}
        />
      </GeoJSONSource>
    </>
  );
}
