export type FollowState = {
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
      return { followUser: true, locationGranted: true };
    case "location_denied":
      return { followUser: false, locationGranted: false };
    case "user_gesture":
      return state.followUser ? { ...state, followUser: false } : state;
    case "recenter":
      return state.locationGranted ? { ...state, followUser: true } : state;
  }
}

export function trackUserLocationMode(
  followUser: boolean,
): "default" | undefined {
  return followUser ? "default" : undefined;
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
