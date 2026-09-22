import { GeoJSONSource, Images, Layer } from "@maplibre/maplibre-react-native";
import type { FeatureCollection } from "geojson";

type Props = {
  /** Draft announce location [lon, lat]. */
  coords: [number, number] | null;
};

/**
 * Temporary pin while the user is announcing / picking a spot on the map.
 * Distinct from published mine markers and search hits.
 */
export function AnnounceDraftLayers({ coords }: Props) {
  if (!coords) {
    return null;
  }
  const [lon, lat] = coords;
  const data: FeatureCollection = {
    type: "FeatureCollection",
    features: [
      {
        type: "Feature",
        id: 0,
        geometry: { type: "Point", coordinates: [lon, lat] },
        properties: {},
      },
    ],
  };
  return (
    <>
      <Images
        images={{
          "announce-draft-pencil": require("../../assets/images/announce-draft-pencil.png"),
        }}
      />
      <GeoJSONSource id="announce-draft" data={data}>
        <Layer
          id="announce-draft-halo"
          type="circle"
          source="announce-draft"
          layerIndex={930}
          paint={{
            "circle-color": "#FFE66D",
            "circle-radius": 18,
            "circle-opacity": 0.35,
            "circle-blur": 0.35,
          }}
        />
        <Layer
          id="announce-draft-core"
          type="circle"
          source="announce-draft"
          layerIndex={931}
          paint={{
            "circle-color": "#F4A261",
            "circle-radius": 12,
            "circle-stroke-width": 2,
            "circle-stroke-color": "#ffffff",
          }}
        />
        <Layer
          id="announce-draft-icon"
          type="symbol"
          source="announce-draft"
          layerIndex={932}
          layout={{
            "icon-image": "announce-draft-pencil",
            "icon-size": 0.32,
            "icon-allow-overlap": true,
            "icon-ignore-placement": true,
          }}
        />
      </GeoJSONSource>
    </>
  );
}
