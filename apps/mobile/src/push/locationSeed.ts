import * as Location from "expo-location";

/** Best-effort GPS fix for seeding en-route / location posts. */
export async function currentLatLon(): Promise<{
  latitude: number;
  longitude: number;
} | null> {
  try {
    const permission = await Location.requestForegroundPermissionsAsync();
    if (!permission.granted) {
      return null;
    }
    const position = await Location.getCurrentPositionAsync({
      accuracy: Location.Accuracy.Balanced,
    });
    return {
      latitude: position.coords.latitude,
      longitude: position.coords.longitude,
    };
  } catch {
    return null;
  }
}
