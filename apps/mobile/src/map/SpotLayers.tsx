import {
  GeoJSONSource,
  Images,
  Layer,
} from "@maplibre/maplibre-react-native";
import type { FilterSpecification } from "@maplibre/maplibre-gl-style-spec";
import type { FeatureCollection } from "geojson";

type Props = {
  data: FeatureCollection;
  onPressFeature: (id: string) => void;
};

const unclustered: FilterSpecification = ["!", ["has", "point_count"]];

const regularFilter: FilterSpecification = [
  "all",
  unclustered,
  ["!=", ["to-boolean", ["get", "leaving_now"]], true],
];

const leavingNowFilter: FilterSpecification = [
  "all",
  unclustered,
  ["==", ["to-boolean", ["get", "leaving_now"]], true],
];

/**
 * Clustered discovery markers: parking-P symbols (orange when leaving-now).
 * Uncertainty radius is drawn separately on selection — not as soft blobs.
 */
export function SpotLayers({ data, onPressFeature }: Props) {
  return (
    <>
      <Images
        images={{
          "spot-parking-p": require("../../assets/images/spot-parking-p.png"),
          "spot-parking-p-leaving": require("../../assets/images/spot-parking-p-leaving.png"),
        }}
      />
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
          id="spots-clusters"
          type="circle"
          source="spots"
          filter={["has", "point_count"]}
          layerIndex={901}
          paint={{
            "circle-color": "#1A73E8",
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
          id="spots-parking-p"
          type="symbol"
          source="spots"
          filter={regularFilter}
          layerIndex={906}
          layout={{
            "icon-image": "spot-parking-p",
            "icon-size": 0.45,
            "icon-allow-overlap": true,
            "icon-ignore-placement": true,
          }}
        />
        <Layer
          id="spots-parking-p-leaving"
          type="symbol"
          source="spots"
          filter={leavingNowFilter}
          layerIndex={907}
          layout={{
            "icon-image": "spot-parking-p-leaving",
            "icon-size": 0.45,
            "icon-allow-overlap": true,
            "icon-ignore-placement": true,
          }}
        />
      </GeoJSONSource>
    </>
  );
}
