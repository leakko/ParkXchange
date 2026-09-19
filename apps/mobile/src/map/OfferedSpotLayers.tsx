import { GeoJSONSource, Layer } from "@maplibre/maplibre-react-native";
import type { FeatureCollection } from "geojson";

type Props = {
  data: FeatureCollection;
  onPressFeature: (id: string) => void;
};

/**
 * Unclustered markers for spots where the signed-in user has a pending offer.
 */
export function OfferedSpotLayers({ data, onPressFeature }: Props) {
  return (
    <GeoJSONSource
      id="spots-offered"
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
        id="spots-offered-points"
        type="circle"
        source="spots-offered"
        layerIndex={907}
        paint={{
          "circle-color": "#F4A261",
          "circle-radius": 10,
          "circle-stroke-width": 2,
          "circle-stroke-color": "#ffffff",
        }}
      />
    </GeoJSONSource>
  );
}
