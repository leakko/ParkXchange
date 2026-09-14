import { GeoJSONSource, Layer } from "@maplibre/maplibre-react-native";
import type { FeatureCollection } from "geojson";

type Props = {
  data: FeatureCollection;
  onPressFeature: (id: string) => void;
};

/**
 * Clustered spot markers. Mount only after the first non-empty FeatureCollection
 * is ready: creating the native source with empty data then swapping in hundreds
 * of points leaves the layers blank on MapLibre RN 11 / Android.
 */
export function SpotLayers({ data, onPressFeature }: Props) {
  return (
    <GeoJSONSource
      id="spots"
      data={data}
      cluster
      clusterRadius={42}
      clusterMaxZoom={14}
      onPress={(event) => {
        const feature = event.nativeEvent.features[0];
        if (!feature) {
          return;
        }
        const properties = feature.properties as Record<string, unknown> | null;
        if (properties?.cluster) {
          return;
        }
        const id = String(properties?.id ?? feature.id ?? "");
        if (id) {
          onPressFeature(id);
        }
      }}
    >
      <Layer
        id="spots-points"
        type="circle"
        source="spots"
        filter={["!", ["has", "point_count"]]}
        layerIndex={903}
        paint={{
          "circle-color": "#FF006E",
          "circle-radius": 7,
          "circle-stroke-width": 1.5,
          "circle-stroke-color": "#ffffff",
        }}
      />
      <Layer
        id="spots-clusters"
        type="circle"
        source="spots"
        filter={["has", "point_count"]}
        layerIndex={901}
        paint={{
          "circle-color": "#E85D04",
          "circle-radius": ["step", ["get", "point_count"], 16, 25, 22, 100, 28],
          "circle-stroke-width": 2,
          "circle-stroke-color": "#ffffff",
        }}
      />
      <Layer
        id="spots-cluster-count"
        type="symbol"
        source="spots"
        filter={["has", "point_count"]}
        layerIndex={902}
        layout={{
          "text-field": ["to-string", ["get", "point_count"]],
          "text-size": 12,
          "text-allow-overlap": true,
        }}
        paint={{ "text-color": "#ffffff" }}
      />
    </GeoJSONSource>
  );
}
