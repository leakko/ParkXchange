import {
  GeoJSONSource,
  Images,
  Layer,
} from "@maplibre/maplibre-react-native";
import type { FeatureCollection } from "geojson";

type Props = {
  data: FeatureCollection;
};

/**
 * Temporary, translucent pin when focusing a historical exchange from account.
 * Same icon language as the driver exchange pin, but smaller and faded so it
 * reads as “was here”, not a live listing.
 */
export function HistoryGhostLayers({ data }: Props) {
  return (
    <>
      <Images
        images={{
          "spot-history-ghost": require("../../assets/images/spot-mine-person.png"),
        }}
      />
      <GeoJSONSource id="spots-history-ghost" data={data}>
        <Layer
          id="spots-history-ghost-halo"
          type="circle"
          source="spots-history-ghost"
          paint={{
            "circle-color": "#E76F51",
            "circle-radius": 12,
            "circle-opacity": 0.18,
          }}
        />
        <Layer
          id="spots-history-ghost-points"
          type="circle"
          source="spots-history-ghost"
          paint={{
            "circle-color": "#E76F51",
            "circle-radius": 7,
            "circle-opacity": 0.45,
            "circle-stroke-width": 1.5,
            "circle-stroke-color": "rgba(255,255,255,0.55)",
          }}
        />
        <Layer
          id="spots-history-ghost-icon"
          type="symbol"
          source="spots-history-ghost"
          layout={{
            "icon-image": "spot-history-ghost",
            "icon-size": 0.22,
            "icon-allow-overlap": true,
            "icon-ignore-placement": true,
          }}
          paint={{
            "icon-opacity": 0.55,
          }}
        />
      </GeoJSONSource>
    </>
  );
}
