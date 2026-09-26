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

/**
 * Own listings: person icon (orange tint via leaving-now circle underlay).
 * Handshake badge when the viewer has an active agreement on the spot.
 */
export function MySpotLayers({ data, onPressFeature }: Props) {
  return (
    <>
      <Images
        images={{
          "spot-mine-person": require("../../assets/images/spot-mine-person.png"),
          "spot-handshake-badge": require("../../assets/images/spot-handshake-badge.png"),
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
          id="spots-mine-underlay"
          type="circle"
          source="spots-mine"
          filter={regularMineFilter}
          layerIndex={908}
          paint={{
            "circle-color": "#1B9AAA",
            "circle-radius": 14,
            "circle-opacity": 0.35,
          }}
        />
        <Layer
          id="spots-mine-leaving-underlay"
          type="circle"
          source="spots-mine"
          filter={leavingNowFilter}
          layerIndex={909}
          paint={{
            "circle-color": "#E85D04",
            "circle-radius": 14,
            "circle-opacity": 0.4,
          }}
        />
        <Layer
          id="spots-mine-icon"
          type="symbol"
          source="spots-mine"
          layerIndex={910}
          layout={{
            "icon-image": "spot-mine-person",
            "icon-size": 0.4,
            "icon-allow-overlap": true,
            "icon-ignore-placement": true,
          }}
        />
        <Layer
          id="spots-mine-handshake"
          type="symbol"
          source="spots-mine"
          filter={agreedFilter}
          layerIndex={911}
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
