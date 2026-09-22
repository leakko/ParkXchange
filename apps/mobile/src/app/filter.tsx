import { Ionicons } from "@expo/vector-icons";
import { type Href, useRouter } from "expo-router";
import { useCallback, useState } from "react";
import { Pressable, StyleSheet, Text, View } from "react-native";
import { ScrollView } from "react-native-gesture-handler";
import { useSafeAreaInsets } from "react-native-safe-area-context";

import { accountColors } from "@/account/theme";
import { useTranslation } from "@/i18n";
import { defaultMapFilter } from "@/map/mapFilter";
import { MapFilterSheetBody } from "@/map/MapFilterSheetBody";
import {
  peekMapFilter,
  publishMapFilter,
} from "@/map/mapFilterHandoff";

/**
 * Map filter as the same native form sheet used for spot detail.
 */
export default function MapFilterScreen() {
  const router = useRouter();
  const insets = useSafeAreaInsets();
  const { t } = useTranslation();
  const [value, setValue] = useState(() => peekMapFilter());

  const close = useCallback(() => {
    if (router.canGoBack()) {
      router.back();
    } else {
      router.replace("/" as Href);
    }
  }, [router]);

  const applyAndClose = useCallback(
    (next: typeof value) => {
      publishMapFilter(next);
      setValue(next);
      close();
    },
    [close],
  );

  const resetAndClose = useCallback(() => {
    applyAndClose(defaultMapFilter());
  }, [applyAndClose]);

  return (
    <View style={[styles.fill, { paddingTop: 4 }]}>
      <View style={styles.grabberZone} accessibilityRole="adjustable">
        <View style={styles.grabber} />
      </View>

      <View style={styles.header}>
        <Text style={styles.title} numberOfLines={1}>
          {t("map.filter.title")}
        </Text>
        <Pressable
          onPress={close}
          hitSlop={12}
          accessibilityRole="button"
          accessibilityLabel={t("common.close")}
          style={styles.closeHit}
        >
          <Ionicons name="close" size={26} color={accountColors.text} />
        </Pressable>
      </View>

      <ScrollView
        style={styles.fill}
        contentContainerStyle={[
          styles.body,
          { paddingBottom: 32 + insets.bottom },
        ]}
        keyboardShouldPersistTaps="handled"
        keyboardDismissMode="on-drag"
        bounces
        alwaysBounceVertical={false}
      >
        <MapFilterSheetBody
          value={value}
          onApply={applyAndClose}
          onReset={resetAndClose}
        />
      </ScrollView>
    </View>
  );
}

const styles = StyleSheet.create({
  fill: { flex: 1, backgroundColor: accountColors.bg },
  grabberZone: {
    alignItems: "center",
    paddingTop: 6,
    paddingBottom: 10,
  },
  grabber: {
    width: 48,
    height: 5,
    borderRadius: 3,
    backgroundColor: "#526D82",
  },
  header: {
    flexDirection: "row",
    alignItems: "center",
    paddingHorizontal: 16,
    paddingBottom: 8,
    gap: 8,
  },
  title: {
    flex: 1,
    color: accountColors.text,
    fontSize: 18,
    fontWeight: "700",
  },
  closeHit: { padding: 4 },
  body: {
    paddingHorizontal: 16,
    paddingTop: 4,
  },
});
