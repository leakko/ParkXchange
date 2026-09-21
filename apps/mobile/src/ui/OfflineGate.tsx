import NetInfo from "@react-native-community/netinfo";
import { Ionicons } from "@expo/vector-icons";
import { useEffect, useState, type ReactNode } from "react";
import { Pressable, StyleSheet, Text, View } from "react-native";

import { useTranslation } from "@/i18n";

type Props = {
  children: ReactNode;
};

/**
 * Semi-blocking overlay when the device has no internet. Dimmed backdrop
 * absorbs presses so map/sheets do not fire API calls or raw network alerts.
 */
export function OfflineGate({ children }: Props) {
  const { t } = useTranslation();
  const [offline, setOffline] = useState(false);

  useEffect(() => {
    const apply = (connected: boolean | null, reachable: boolean | null) => {
      if (connected === false) {
        setOffline(true);
        return;
      }
      if (reachable === false) {
        setOffline(true);
        return;
      }
      if (connected === true) {
        setOffline(false);
      }
    };

    const unsub = NetInfo.addEventListener((state) => {
      apply(state.isConnected ?? null, state.isInternetReachable ?? null);
    });

    void NetInfo.fetch().then((state) => {
      apply(state.isConnected ?? null, state.isInternetReachable ?? null);
    });

    return unsub;
  }, []);

  return (
    <View style={styles.root}>
      {children}
      {offline ? (
        <Pressable
          style={styles.overlay}
          accessibilityViewIsModal
          accessibilityRole="alert"
          accessibilityLabel={t("offline.title")}
          onPress={() => undefined}
        >
          <View style={styles.card} pointerEvents="box-none">
            <Ionicons name="cloud-offline-outline" size={48} color="#F4F7FA" />
            <Text style={styles.title}>{t("offline.title")}</Text>
            <Text style={styles.body}>{t("offline.body")}</Text>
          </View>
        </Pressable>
      ) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  root: { flex: 1 },
  overlay: {
    ...StyleSheet.absoluteFill,
    backgroundColor: "rgba(11, 31, 51, 0.72)",
    justifyContent: "center",
    alignItems: "center",
    paddingHorizontal: 32,
    zIndex: 1000,
  },
  card: {
    alignItems: "center",
    gap: 12,
    maxWidth: 320,
  },
  title: {
    color: "#F4F7FA",
    fontSize: 20,
    fontWeight: "700",
    textAlign: "center",
  },
  body: {
    color: "#D6E2E9",
    fontSize: 15,
    lineHeight: 22,
    textAlign: "center",
  },
});
