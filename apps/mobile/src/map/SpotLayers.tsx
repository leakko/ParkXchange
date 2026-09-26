import {
  GeoJSONSource,
  Images,
  Layer,
} from "@maplibre/maplibre-react-native";
import type { FilterSpecification } from "@maplibre/maplibre-gl-style-spec";
import type { FeatureCollection } from "geojson";

import { nearestSpotIdAtTouch } from "@/map/nearestSpotAtTouch";

type Props = {
  data: FeatureCollection;
  onPressFeature: (id: string) => void;
};

const unclustered: FilterSpecification = ["!", ["has", "point_count"]];

const leavingNowFilter: FilterSpecification = [
  "all",
  unclustered,
  ["==", ["to-boolean", ["get", "leaving_now"]], true],
];

/** Preferred departure (timed) — blue P, matches “Salida próxima”. */
const soonFilter: FilterSpecification = [
  "all",
  unclustered,
  ["!=", ["to-boolean", ["get", "leaving_now"]], true],
  ["!=", ["to-boolean", ["get", "flexible"]], true],
];

/** Flexible / other — grey P, matches filter “Resto”. */
const flexibleFilter: FilterSpecification = [
  "all",
  unclustered,
  ["!=", ["to-boolean", ["get", "leaving_now"]], true],
  ["==", ["to-boolean", ["get", "flexible"]], true],
];

/** Match own-spot visual footprint (~22–24 px). */
const PARKING_ICON_SIZE = 0.28;

/**
 * Clustered discovery markers: Maps-style parking-P symbols.
 * Uncertainty radius is drawn separately on selection.
 */
export function SpotLayers({ data, onPressFeature }: Props) {
  return (
    <>
      <Images
        images={{
          "spot-parking-p": require("../../assets/images/spot-parking-p.png"),
          "spot-parking-p-leaving": require("../../assets/images/spot-parking-p-leaving.png"),
          "spot-parking-p-other": require("../../assets/images/spot-parking-p-other.png"),
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
          id="spots-clusters"
          type="circle"
          source="spots"
          filter={["has", "point_count"]}
          paint={{
            "circle-color": "#1A73E8",
            "circle-radius": ["step", ["get", "point_count"], 14, 25, 18, 100, 22],
            "circle-stroke-width": 2,
            "circle-stroke-color": "#ffffff",
          }}
        />
        <Layer
          id="spots-cluster-count"
          type="symbol"
          source="spots"
          filter={["has", "point_count"]}
          layout={{
            "text-font": ["Noto Sans Bold"],
            "text-field": ["to-string", ["get", "point_count"]],
            "text-size": 11,
            "text-allow-overlap": true,
          }}
          paint={{ "text-color": "#ffffff" }}
        />
        <Layer
          id="spots-parking-p-soon"
          type="symbol"
          source="spots"
          filter={soonFilter}
          layout={{
            "icon-image": "spot-parking-p",
            "icon-size": PARKING_ICON_SIZE,
            "icon-allow-overlap": true,
            "icon-ignore-placement": true,
          }}
        />
        <Layer
          id="spots-parking-p-other"
          type="symbol"
          source="spots"
          filter={flexibleFilter}
          layout={{
            "icon-image": "spot-parking-p-other",
            "icon-size": PARKING_ICON_SIZE,
            "icon-allow-overlap": true,
            "icon-ignore-placement": true,
          }}
        />
        <Layer
          id="spots-parking-p-leaving"
          type="symbol"
          source="spots"
          filter={leavingNowFilter}
          layout={{
            "icon-image": "spot-parking-p-leaving",
            "icon-size": PARKING_ICON_SIZE,
            "icon-allow-overlap": true,
            "icon-ignore-placement": true,
          }}
        />
      </GeoJSONSource>
    </>
  );
}
