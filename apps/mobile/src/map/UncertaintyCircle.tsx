import { GeoJSONSource, Layer } from "@maplibre/maplibre-react-native";
import type { Feature, Polygon } from "geojson";
import { useMemo } from "react";

const RADIUS_METRES = 30;
const STEPS = 48;
const METRES_PER_DEGREE_LAT = 111_320;
/** Classic parking-sign blue. */
const PARKING_BLUE = "#1A73E8";

type Props = {
  lon: number;
  lat: number;
};

/** Approximate circle polygon in WGS84 for a fixed ground radius. */
function circlePolygon(lon: number, lat: number, radiusMetres: number): Feature<Polygon> {
  const cosLat = Math.max(Math.cos((lat * Math.PI) / 180), 1e-6);
  const dLat = radiusMetres / METRES_PER_DEGREE_LAT;
  const dLon = radiusMetres / (METRES_PER_DEGREE_LAT * cosLat);
  const ring: [number, number][] = [];
  for (let i = 0; i <= STEPS; i++) {
    const theta = (i / STEPS) * 2 * Math.PI;
    ring.push([lon + dLon * Math.sin(theta), lat + dLat * Math.cos(theta)]);
  }
  return {
    type: "Feature",
    properties: {},
    geometry: { type: "Polygon", coordinates: [ring] },
  };
}

/**
 * Translucent 30 m uncertainty area shown while a selected spot is still
 * approximate (`exact_location: false`).
 */
export function UncertaintyCircle({ lon, lat }: Props) {
  const data = useMemo(() => circlePolygon(lon, lat, RADIUS_METRES), [lon, lat]);

  return (
    <GeoJSONSource id="spot-uncertainty" data={data}>
      <Layer
        id="spot-uncertainty-fill"
        type="fill"
        source="spot-uncertainty"
        paint={{
          "fill-color": PARKING_BLUE,
          "fill-opacity": 0.22,
        }}
      />
      <Layer
        id="spot-uncertainty-outline"
        type="line"
        source="spot-uncertainty"
        paint={{
          "line-color": PARKING_BLUE,
          "line-width": 1.5,
          "line-opacity": 0.6,
        }}
      />
    </GeoJSONSource>
  );
}
