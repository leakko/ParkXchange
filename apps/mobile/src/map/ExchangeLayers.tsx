import {
  GeoJSONSource,
  Images,
  Layer,
} from "@maplibre/maplibre-react-native";
import type { FeatureCollection } from "geojson";

type Props = {
  data: FeatureCollection;
  onPressFeature: (id: string) => void;
  /** Owner keeps the teal “mine” look; driver gets a distinct exchange pin. */
  role: "owner" | "driver";
};

/**
 * Exact exchange pin for the caller's active reservation.
 * Only parties receive these coordinates (via getSpot / active reservation).
 */
export function ExchangeLayers({ data, onPressFeature, role }: Props) {
  const color = role === "owner" ? "#1B9AAA" : "#E76F51";
  const sourceId = `spots-exchange-${role}`;
  const iconId = role === "owner" ? "spot-mine-car" : "spot-exchange-person";

  return (
    <>
      <Images
        images={{
          "spot-mine-car": require("../../assets/images/spot-mine-car.png"),
          "spot-exchange-person": require("../../assets/images/spot-mine-person.png"),
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
          layerIndex={role === "owner" ? 910 : 913}
          paint={{
            "circle-color": color,
            "circle-radius": 18,
            "circle-opacity": 0.28,
          }}
        />
        <Layer
          id={`${sourceId}-points`}
          type="circle"
          source={sourceId}
          layerIndex={role === "owner" ? 911 : 914}
          paint={{
            "circle-color": color,
            "circle-radius": 12,
            "circle-stroke-width": 2.5,
            "circle-stroke-color": "#ffffff",
          }}
        />
        <Layer
          id={`${sourceId}-icon`}
          type="symbol"
          source={sourceId}
          layerIndex={role === "owner" ? 912 : 915}
          layout={{
            "icon-image": iconId,
            "icon-size": 0.35,
            "icon-allow-overlap": true,
            "icon-ignore-placement": true,
          }}
        />
      </GeoJSONSource>
    </>
  );
}
