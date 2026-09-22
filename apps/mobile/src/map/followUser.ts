export type FollowState = {
  /**
   * Continuous camera follow is intentionally unused: flaky GPS was yanking
   * the map. Kept for API compatibility; always stays false after grant.
   */
  followUser: boolean;
  locationGranted: boolean;
};

export type FollowAction =
  | { type: "location_granted" }
  | { type: "location_denied" }
  | { type: "user_gesture" }
  | { type: "recenter" };

export function initialFollowState(): FollowState {
  return { followUser: false, locationGranted: false };
}

export function followReducer(
  state: FollowState,
  action: FollowAction,
): FollowState {
  switch (action.type) {
    case "location_granted":
      // Permission only — do not enable continuous follow.
      return { followUser: false, locationGranted: true };
    case "location_denied":
      return { followUser: false, locationGranted: false };
    case "user_gesture":
      return state.followUser ? { ...state, followUser: false } : state;
    case "recenter":
      // Locate FAB does a one-shot easeTo; never re-enable tracking.
      return state;
  }
}

/** Continuous trackUserLocation is disabled; locate is one-shot only. */
export function trackUserLocationMode(
  _followUser: boolean,
): "default" | undefined {
  return undefined;
}

/**
 * MapLibre's LocationComponent errors with "Failed to obtain last location
 * update" (and often "Invalid geometry in line layer") if it is enabled before
 * the OS has any cached fix. Gate the puck and camera tracking on a real fix.
 */
export function locationComponentReady(
  locationGranted: boolean,
  coords: [number, number] | null,
): boolean {
  return locationGranted && coords != null;
}

export function resolveInitialView(args: {
  granted: boolean;
  coords: [number, number] | null;
  fallbackCenter: [number, number];
  userZoom: number;
  fallbackZoom: number;
}): { center: [number, number]; zoom: number } {
  if (args.granted && args.coords) {
    return { center: args.coords, zoom: args.userZoom };
  }
  return { center: args.fallbackCenter, zoom: args.fallbackZoom };
}
