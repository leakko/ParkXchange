import type { ExpoConfig } from "expo/config";

const locationPermission =
  "ParkXchange uses your location to show nearby parking spots and to announce the one you are leaving.";

const photoPermission =
  "ParkXchange uses your photo library so you can attach a picture of your vehicle.";

/**
 * Path to google-services.json.
 *
 * - EAS cloud builds: file env `GOOGLE_SERVICES_JSON` (secret) → absolute path on
 *   the builder. Do not fall back to `./google-services.json` here: that file is
 *   gitignored, and pointing at it during `eas build` upload prints a warning
 *   and still does not pack it from git.
 * - Local `expo run:android`: `GOOGLE_SERVICES_JSON=./google-services.json`
 *   in apps/mobile/.env.development (or a gitignored override).
 */
const googleServicesFile = process.env.GOOGLE_SERVICES_JSON?.trim() || undefined;

const config: ExpoConfig = {
  name: "ParkXchange",
  slug: "parkxchange",
  scheme: "parkxchange",
  version: "0.1.0",
  orientation: "portrait",
  icon: "./assets/images/icon.png",
  userInterfaceStyle: "automatic",
  newArchEnabled: true,
  ios: {
    bundleIdentifier: "com.parkxchange.mobile",
    infoPlist: {
      NSLocationWhenInUseUsageDescription: locationPermission,
      LSApplicationQueriesSchemes: ["comgooglemaps", "waze", "maps"],
    },
  },
  android: {
    package: "com.parkxchange.mobile",
    ...(googleServicesFile ? { googleServicesFile } : {}),
    softwareKeyboardLayoutMode: "resize",
    adaptiveIcon: {
      backgroundColor: "#0B1F33",
      foregroundImage: "./assets/images/android-icon-foreground.png",
      backgroundImage: "./assets/images/android-icon-background.png",
      monochromeImage: "./assets/images/android-icon-monochrome.png",
    },
    predictiveBackGestureEnabled: false,
  },
  plugins: [
    "expo-router",
    "@maplibre/maplibre-react-native",
    "@react-native-community/datetimepicker",
    [
      "expo-location",
      {
        locationWhenInUsePermission: locationPermission,
        locationAlwaysAndWhenInUsePermission:
          "ParkXchange uses your location in the background during an active exchange to remind you when you arrive at the meeting point.",
        isAndroidBackgroundLocationEnabled: true,
        isAndroidForegroundServiceEnabled: true,
        isIosBackgroundLocationEnabled: true,
      },
    ],
    [
      "expo-image-picker",
      {
        photosPermission: photoPermission,
      },
    ],
    "expo-secure-store",
    "expo-localization",
    "expo-web-browser",
    "@react-native-google-signin/google-signin",
    [
      "expo-notifications",
      {
        defaultChannel: "reconfirm",
      },
    ],
    [
      "expo-splash-screen",
      {
        backgroundColor: "#0B1F33",
        image: "./assets/images/splash-icon.png",
        imageWidth: 76,
      },
    ],
    "./plugins/withMapQueries",
  ],
  experiments: {
    typedRoutes: true,
  },
  extra: {
    mapStyleUrl: process.env.EXPO_PUBLIC_MAP_STYLE_URL,
    apiUrl: process.env.EXPO_PUBLIC_API_URL,
    eas: {
      projectId: "6e924fb7-f674-48a0-9a3b-7df7400c1ba9",
    },
  },
};

export default config;
