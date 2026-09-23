import { GeoJSONSource, Layer } from "@maplibre/maplibre-react-native";
import type { FilterSpecification } from "@maplibre/maplibre-gl-style-spec";
import type { FeatureCollection } from "geojson";

type Props = {
  data: FeatureCollection;
  onPressFeature: (id: string) => void;
};

/** Approximate listings: soft halo + diffuse core (not a hard POI pin). */
const approxFilter: FilterSpecification = [
  "all",
  ["!", ["has", "point_count"]],
  ["!=", ["to-boolean", ["get", "exact_location"]], true],
  ["!=", ["to-boolean", ["get", "leaving_now"]], true],
];

/** Reserved / revealed listings among others: crisp exact pin. */
const exactFilter: FilterSpecification = [
  "all",
  ["!", ["has", "point_count"]],
  ["==", ["to-boolean", ["get", "exact_location"]], true],
  ["!=", ["to-boolean", ["get", "leaving_now"]], true],
];

/** «Me voy ya» approx — same soft blob, orange. */
const leavingNowApproxFilter: FilterSpecification = [
  "all",
  ["!", ["has", "point_count"]],
  ["==", ["to-boolean", ["get", "leaving_now"]], true],
  ["!=", ["to-boolean", ["get", "exact_location"]], true],
];

/** «Me voy ya» exact — same crisp pin, orange. */
const leavingNowExactFilter: FilterSpecification = [
  "all",
  ["!", ["has", "point_count"]],
  ["==", ["to-boolean", ["get", "leaving_now"]], true],
  ["==", ["to-boolean", ["get", "exact_location"]], true],
];

/**
 * Clustered spot markers. Mount only after the first non-empty FeatureCollection
 * is ready: creating the native source with empty data then swapping in hundreds
 * of points leaves the layers blank on MapLibre RN 11 / Android.
 *
 * Pre-reserve spots render as soft blobs so they read as "near here", not a
 * precise street address. Exact pins appear only when exact_location is true.
 * Leaving-now listings use the same shapes in orange.
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
        event.stopPropagation();
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
        id="spots-approx-halo"
        type="circle"
        source="spots"
        filter={approxFilter}
        layerIndex={899}
        paint={{
          "circle-color": "#FF006E",
          "circle-radius": 16,
          "circle-opacity": 0.22,
          "circle-blur": 0.65,
        }}
      />
      <Layer
        id="spots-leaving-now-approx-halo"
        type="circle"
        source="spots"
        filter={leavingNowApproxFilter}
        layerIndex={900}
        paint={{
          "circle-color": "#E85D04",
          "circle-radius": 16,
          "circle-opacity": 0.22,
          "circle-blur": 0.65,
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
          "text-font": ["Noto Sans Bold"],
          "text-field": ["to-string", ["get", "point_count"]],
          "text-size": 12,
          "text-allow-overlap": true,
        }}
        paint={{ "text-color": "#ffffff" }}
      />
      <Layer
        id="spots-approx-core"
        type="circle"
        source="spots"
        filter={approxFilter}
        layerIndex={903}
        paint={{
          "circle-color": "#FF006E",
          "circle-radius": 5,
          "circle-opacity": 0.55,
          "circle-blur": 0.35,
          "circle-stroke-width": 0,
        }}
      />
      <Layer
        id="spots-leaving-now-approx-core"
        type="circle"
        source="spots"
        filter={leavingNowApproxFilter}
        layerIndex={904}
        paint={{
          "circle-color": "#E85D04",
          "circle-radius": 5,
          "circle-opacity": 0.55,
          "circle-blur": 0.35,
          "circle-stroke-width": 0,
        }}
      />
      <Layer
        id="spots-exact-points"
        type="circle"
        source="spots"
        filter={exactFilter}
        layerIndex={906}
        paint={{
          "circle-color": "#FF006E",
          "circle-radius": 7,
          "circle-stroke-width": 1.5,
          "circle-stroke-color": "#ffffff",
        }}
      />
      <Layer
        id="spots-leaving-now-exact"
        type="circle"
        source="spots"
        filter={leavingNowExactFilter}
        layerIndex={907}
        paint={{
          "circle-color": "#E85D04",
          "circle-radius": 7,
          "circle-stroke-width": 1.5,
          "circle-stroke-color": "#ffffff",
        }}
      />
    </GeoJSONSource>
  );
}
