import AsyncStorage from "@react-native-async-storage/async-storage";

const KEY = "parkxchange.locationAssistance";

/** Default on: assisted one-shot geofence after Yendo. */
export async function getLocationAssistanceEnabled(): Promise<boolean> {
  const raw = await AsyncStorage.getItem(KEY);
  if (raw === null) {
    return true;
  }
  return raw === "1" || raw === "true";
}

export async function setLocationAssistanceEnabled(on: boolean): Promise<void> {
  await AsyncStorage.setItem(KEY, on ? "1" : "0");
}
