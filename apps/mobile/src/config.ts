export const mapStyleUrl =
  process.env.EXPO_PUBLIC_MAP_STYLE_URL ?? "https://demotiles.maplibre.org/style.json";

export const apiUrl = (process.env.EXPO_PUBLIC_API_URL ?? "http://10.0.2.2:8080").replace(
  /\/$/,
  "",
);

export const wsUrl = (process.env.EXPO_PUBLIC_WS_URL ?? apiUrl.replace(/^http/, "ws")).replace(
  /\/$/,
  "",
);

export const barcelonaCenter: [number, number] = [2.1734, 41.3851];
