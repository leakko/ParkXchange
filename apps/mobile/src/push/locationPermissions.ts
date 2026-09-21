import * as Location from "expo-location";
import AsyncStorage from "@react-native-async-storage/async-storage";
import { Alert, Platform } from "react-native";

import { isFreshFix } from "@/push/locationFreshness";

const ALWAYS_PROMPTED_KEY = "parkxchange.location.alwaysPrompted.v1";

type TFn = (key: string, params?: Record<string, string | number>) => string;

export async function getFreshPosition(
  accuracy: Location.LocationAccuracy = Location.Accuracy.High,
): Promise<Location.LocationObject | null> {
  try {
    const position = await Location.getCurrentPositionAsync({ accuracy });
    if (isFreshFix(position)) {
      return position;
    }
    const retry = await Location.getCurrentPositionAsync({
      accuracy: Location.Accuracy.BestForNavigation,
    });
    return retry;
  } catch {
    return null;
  }
}

export async function hasAlwaysLocation(): Promise<boolean> {
  if (Platform.OS === "web") {
    return false;
  }
  try {
    const bg = await Location.getBackgroundPermissionsAsync();
    return bg.status === "granted";
  } catch {
    return false;
  }
}

/**
 * Explain why we need “Allow all the time”, then request FG → BG.
 * Returns whether background (“always”) was granted.
 */
export async function ensureAlwaysLocation(opts: {
  t: TFn;
  forceExplain?: boolean;
}): Promise<boolean> {
  if (Platform.OS === "web") {
    return false;
  }

  const already = await hasAlwaysLocation();
  if (already) {
    return true;
  }

  const prompted = (await AsyncStorage.getItem(ALWAYS_PROMPTED_KEY)) === "1";
  if (!prompted || opts.forceExplain) {
    await new Promise<void>((resolve) => {
      Alert.alert(
        opts.t("location.always.title"),
        opts.t("location.always.message"),
        [{ text: opts.t("common.ok"), onPress: () => resolve() }],
        { cancelable: false },
      );
    });
    await AsyncStorage.setItem(ALWAYS_PROMPTED_KEY, "1");
  }

  const fg = await Location.requestForegroundPermissionsAsync();
  if (!fg.granted) {
    return false;
  }

  try {
    const bg = await Location.requestBackgroundPermissionsAsync();
    return bg.status === "granted";
  } catch {
    return false;
  }
}

/** First-launch / cold path: explain + request once per install. */
export async function promptAlwaysLocationOnFirstOpen(t: TFn): Promise<void> {
  if (Platform.OS === "web") {
    return;
  }
  const prompted = (await AsyncStorage.getItem(ALWAYS_PROMPTED_KEY)) === "1";
  if (prompted) {
    return;
  }
  await ensureAlwaysLocation({ t, forceExplain: true });
}
