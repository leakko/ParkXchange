function stripTrailingSlash(url: string): string {
  return url.replace(/\/$/, "");
}

/** Origin of the HTTP API (no path). */
export const apiUrl = stripTrailingSlash(
  process.env.EXPO_PUBLIC_API_URL ?? "http://10.0.2.2:8080",
);

/**
 * WebSocket origin. Accepts either `ws://host:port` or a full
 * `ws://host:port/v1/ws` from older .env files and normalises to the origin.
 */
export const wsUrl = stripTrailingSlash(
  (process.env.EXPO_PUBLIC_WS_URL ?? apiUrl.replace(/^http/, "ws")).replace(
    /\/v1\/ws\/?$/,
    "",
  ),
);

export const mapStyleUrl =
  process.env.EXPO_PUBLIC_MAP_STYLE_URL ?? "http://10.0.2.2:8090/style.json";

export const barcelonaCenter: [number, number] = [2.1734, 41.3851];
