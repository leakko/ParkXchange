import { GeoJSONSource, Layer } from "@maplibre/maplibre-react-native";
import type { FeatureCollection } from "geojson";

import type { AddressSuggestion } from "@/map/geocode";

type Props = {
  hits: AddressSuggestion[];
  selectedId?: string | null;
  onPressHit: (id: string) => void;
};

export function searchHitsToCollection(
  hits: AddressSuggestion[],
  selectedId?: string | null,
): FeatureCollection {
  return {
    type: "FeatureCollection",
    features: hits.map((h, index) => ({
      type: "Feature",
      // MapLibre prefers numeric feature ids; uniqueness lives in properties.
      id: index,
      geometry: { type: "Point", coordinates: [h.lon, h.lat] },
      properties: {
        id: h.id,
        label: h.label,
        selected: h.id === selectedId,
      },
    })),
  };
}

/** Distinct cyan pins for place-search hits (not parking spots). */
export function SearchPlaceLayers({ hits, selectedId, onPressHit }: Props) {
  if (hits.length === 0) {
    return null;
  }
  const data = searchHitsToCollection(hits, selectedId);
  return (
    <GeoJSONSource
      id="search-places"
      data={data}
      onPress={(event) => {
        event.stopPropagation();
        const feature = event.nativeEvent.features[0];
        const id = String(
          (feature?.properties as { id?: string } | null)?.id ??
            feature?.id ??
            "",
        );
        if (id) {
          onPressHit(id);
        }
      }}
    >
      <Layer
        id="search-places-halo"
        type="circle"
        source="search-places"
        layerIndex={920}
        paint={{
          "circle-color": "#00BBF9",
          "circle-radius": 18,
          "circle-opacity": 0.25,
          "circle-blur": 0.4,
        }}
      />
      <Layer
        id="search-places-core"
        type="circle"
        source="search-places"
        layerIndex={921}
        paint={{
          "circle-color": [
            "case",
            ["get", "selected"],
            "#FFE66D",
            "#00BBF9",
          ],
          "circle-radius": ["case", ["get", "selected"], 11, 8],
          "circle-stroke-width": 2,
          "circle-stroke-color": "#ffffff",
        }}
      />
    </GeoJSONSource>
  );
}
