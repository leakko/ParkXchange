import { Alert, Linking, Platform } from "react-native";

export type NavTarget = { lat: number; lon: number; label?: string };

export type NavigationCopy = {
  failedTitle: string;
  failedMessage: string;
};

function webFallback({ lat, lon }: NavTarget): string {
  return `https://www.google.com/maps/dir/?api=1&destination=${lat},${lon}`;
}

function candidates({ lat, lon }: NavTarget): { name: string; url: string }[] {
  const dest = `${lat},${lon}`;
  if (Platform.OS === "ios") {
    return [
      { name: "Apple Maps", url: `maps://?daddr=${dest}` },
      { name: "Google Maps", url: `comgooglemaps://?daddr=${dest}&directionsmode=driving` },
      { name: "Waze", url: `waze://?ll=${dest}&navigate=yes` },
      { name: "Browser", url: webFallback({ lat, lon }) },
    ];
  }
  return [
    { name: "Google Maps", url: `google.navigation:q=${dest}` },
    { name: "Waze", url: `waze://?ll=${dest}&navigate=yes` },
    { name: "Browser", url: webFallback({ lat, lon }) },
  ];
}

/** Open turn-by-turn navigation, preferring installed native apps. */
export async function openNavigation(
  target: NavTarget,
  copy: NavigationCopy,
): Promise<void> {
  for (const option of candidates(target)) {
    try {
      const supported = await Linking.canOpenURL(option.url);
      if (supported) {
        await Linking.openURL(option.url);
        return;
      }
    } catch {
      // try the next candidate
    }
  }
  Alert.alert(copy.failedTitle, copy.failedMessage);
}
