import { BottomSheetModalProvider } from "@gorhom/bottom-sheet";
import { LogManager } from "@maplibre/maplibre-react-native";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { Stack } from "expo-router";
import { StatusBar } from "expo-status-bar";
import { useState } from "react";
import { LogBox } from "react-native";
import { GestureHandlerRootView } from "react-native-gesture-handler";
import { SafeAreaProvider } from "react-native-safe-area-context";

import { I18nProvider } from "@/i18n";
import { useOwnerOfferAlerts } from "@/hooks/useOwnerOfferAlerts";

// Intermittent MapLibre tile/glyph stream errors on emulators are noisy but
// non-fatal; the map still renders.
LogBox.ignoreLogs(["MapLibre Native", "unexpected end of stream"]);

// OpenFreeMap vector tiles occasionally ship degenerate line features; MapLibre
// logs them as WARN but still paints the rest of the style.
LogManager.onLog(({ message }) => message.includes("Invalid geometry in line layer"));

function OwnerOfferAlerts() {
  useOwnerOfferAlerts();
  return null;
}

export default function RootLayout() {
  const [queryClient] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: { retry: 1, refetchOnWindowFocus: false },
        },
      }),
  );

  return (
    <GestureHandlerRootView style={{ flex: 1 }}>
      <SafeAreaProvider>
        <I18nProvider>
          <QueryClientProvider client={queryClient}>
            <BottomSheetModalProvider>
              <StatusBar style="light" />
              <OwnerOfferAlerts />
              <Stack screenOptions={{ headerShown: false }} />
            </BottomSheetModalProvider>
          </QueryClientProvider>
        </I18nProvider>
      </SafeAreaProvider>
    </GestureHandlerRootView>
  );
}
