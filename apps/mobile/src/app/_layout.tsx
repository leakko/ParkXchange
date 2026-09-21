import { BottomSheetModalProvider } from "@gorhom/bottom-sheet";
import { LogManager } from "@maplibre/maplibre-react-native";
import {
  DarkTheme,
  ThemeProvider,
  type Theme,
} from "@react-navigation/native";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { Stack } from "expo-router";
import { StatusBar } from "expo-status-bar";
import * as SystemUI from "expo-system-ui";
import { useEffect, useState } from "react";
import { LogBox } from "react-native";
import { GestureHandlerRootView } from "react-native-gesture-handler";
import { SafeAreaProvider } from "react-native-safe-area-context";

import { accountColors } from "@/account/theme";
import { I18nProvider } from "@/i18n";
import { useOwnerOfferAlerts } from "@/hooks/useOwnerOfferAlerts";
import { ExchangePushBootstrap } from "@/push/ExchangePushBootstrap";
import { OfflineGate } from "@/ui/OfflineGate";
import { ToastProvider } from "@/ui/toast";

// Intermittent MapLibre tile/glyph stream errors on emulators are noisy but
// non-fatal; the map still renders.
LogBox.ignoreLogs(["MapLibre Native", "unexpected end of stream"]);

// OpenFreeMap vector tiles occasionally ship degenerate line features; MapLibre
// logs them as WARN but still paints the rest of the style.
LogManager.onLog(({ message }) => message.includes("Invalid geometry in line layer"));

/** Default navigator chrome is white; without this, slide transitions flash a white strip. */
const navigationTheme: Theme = {
  ...DarkTheme,
  colors: {
    ...DarkTheme.colors,
    background: accountColors.bg,
    card: accountColors.bg,
    border: accountColors.border,
    primary: accountColors.accent,
    text: accountColors.text,
  },
};

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

  useEffect(() => {
    void SystemUI.setBackgroundColorAsync(accountColors.bg);
  }, []);

  return (
    <GestureHandlerRootView
      style={{ flex: 1, backgroundColor: accountColors.bg }}
    >
      <SafeAreaProvider>
        <I18nProvider>
          <QueryClientProvider client={queryClient}>
            <ThemeProvider value={navigationTheme}>
              <ToastProvider>
                <OfflineGate>
                  <BottomSheetModalProvider>
                    <StatusBar style="light" />
                    <OwnerOfferAlerts />
                    <ExchangePushBootstrap />
                    <Stack
                      screenOptions={{
                        headerShown: false,
                        contentStyle: { backgroundColor: accountColors.bg },
                        animation: "slide_from_right",
                      }}
                    />
                  </BottomSheetModalProvider>
                </OfflineGate>
              </ToastProvider>
            </ThemeProvider>
          </QueryClientProvider>
        </I18nProvider>
      </SafeAreaProvider>
    </GestureHandlerRootView>
  );
}
