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
  process.env.EXPO_PUBLIC_MAP_STYLE_URL ??
  "https://tiles.openfreemap.org/styles/liberty";

/** Fallback map center when GPS and persisted home are unavailable: Sevilla. */
export const defaultMapCenter: [number, number] = [-5.97315, 37.37185];

/** Street-level zoom when the camera opens on the user. */
export const userZoom = 16;

/** City overview used when location is unavailable. */
export const fallbackZoom = 14;
