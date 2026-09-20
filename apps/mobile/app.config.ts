import type { ExpoConfig } from "expo/config";

const locationPermission =
  "ParkXchange uses your location to show nearby parking spots and to announce the one you are leaving.";

const photoPermission =
  "ParkXchange uses your photo library so you can attach a picture of your vehicle.";

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
