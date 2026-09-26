import {
  GeoJSONSource,
  Images,
  Layer,
} from "@maplibre/maplibre-react-native";
import type { FilterSpecification } from "@maplibre/maplibre-gl-style-spec";
import type { FeatureCollection } from "geojson";
import { useMemo } from "react";

import { MapPulseRingLayer } from "@/map/MapPulseRingLayer";
import { nearestSpotIdAtTouch } from "@/map/nearestSpotAtTouch";

type Props = {
  data: FeatureCollection;
  onPressFeature: (id: string) => void;
};

const leavingNowFilter: FilterSpecification = [
  "==",
  ["to-boolean", ["get", "leaving_now"]],
  true,
];

const regularMineFilter: FilterSpecification = [
  "!=",
  ["to-boolean", ["get", "leaving_now"]],
  true,
];

const agreedFilter: FilterSpecification = [
  "==",
  ["to-boolean", ["get", "has_agreement"]],
  true,
];

function hasAgreedFeature(data: FeatureCollection): boolean {
  return data.features.some((f) => Boolean(f.properties?.has_agreement));
}

/**
 * Own listings: solid teal/orange disc + person icon (fully opaque).
 * Pulsing ring when the viewer has an active agreement on the spot.
 */
export function MySpotLayers({ data, onPressFeature }: Props) {
  const pulseEnabled = useMemo(() => hasAgreedFeature(data), [data]);

  return (
    <>
      <Images
        images={{
          "spot-mine-person": require("../../assets/images/spot-mine-person.png"),
        }}
      />
      <GeoJSONSource
        id="spots-mine"
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
        <MapPulseRingLayer
          id="spots-mine-pulse"
          sourceId="spots-mine"
          color="#1B9AAA"
          enabled={pulseEnabled}
          filter={agreedFilter}
        />
        <Layer
          id="spots-mine-underlay"
          type="circle"
          source="spots-mine"
          filter={regularMineFilter}
          paint={{
            "circle-color": "#1B9AAA",
            "circle-radius": 11,
            "circle-opacity": 1,
            "circle-stroke-width": 2.5,
            "circle-stroke-color": "#ffffff",
          }}
        />
        <Layer
          id="spots-mine-leaving-underlay"
          type="circle"
          source="spots-mine"
          filter={leavingNowFilter}
          paint={{
            "circle-color": "#E85D04",
            "circle-radius": 11,
            "circle-opacity": 1,
            "circle-stroke-width": 2.5,
            "circle-stroke-color": "#ffffff",
          }}
        />
        <Layer
          id="spots-mine-icon"
          type="symbol"
          source="spots-mine"
          layout={{
            "icon-image": "spot-mine-person",
            "icon-size": 0.32,
            "icon-allow-overlap": true,
            "icon-ignore-placement": true,
          }}
        />
      </GeoJSONSource>
    </>
  );
}
