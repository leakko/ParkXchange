import { GeoJSONSource, Layer } from "@maplibre/maplibre-react-native";
import type { FeatureCollection } from "geojson";

import { nearestSpotIdAtTouch } from "@/map/nearestSpotAtTouch";

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
        const native = event.nativeEvent as {
          lngLat?: [number, number];
          features: Parameters<typeof nearestSpotIdAtTouch>[1];
        };
        const id = nearestSpotIdAtTouch(native.lngLat ?? null, native.features, data);
        if (id) {
          onPressFeature(id);
        }
      }}
    >
      <Layer
        id="spots-offered-points"
        type="circle"
        source="spots-offered"
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
