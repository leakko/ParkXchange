import { useLocalSearchParams, useRouter } from "expo-router";
import { useEffect, useRef, useState } from "react";
import { ActivityIndicator, Alert, StyleSheet, Text, View } from "react-native";

import { confirmEmail } from "@/api/client";
import { apiErrorMessage } from "@/api/errors";
import { AuthScroll } from "@/auth/AuthScroll";
import { accountColors } from "@/account/theme";
import { useTranslation } from "@/i18n";

export default function VerifyEmailScreen() {
  const { t } = useTranslation();
  const router = useRouter();
  const { token } = useLocalSearchParams<{ token?: string }>();
  const [busy, setBusy] = useState(true);
  const ran = useRef(false);

  useEffect(() => {
    if (ran.current) {
      return;
    }
    ran.current = true;
    const raw = typeof token === "string" ? token.trim() : "";
    if (!raw) {
      setBusy(false);
      Alert.alert(t("common.error"), t("auth.error.resetTokenInvalid"), [
        { text: t("common.ok"), onPress: () => router.replace("/auth/login" as never) },
      ]);
      return;
    }
    void (async () => {
      try {
        await confirmEmail(raw);
        Alert.alert(t("auth.verify.success"), t("auth.verify.successBody"), [
          { text: t("common.ok"), onPress: () => router.replace("/" as never) },
        ]);
      } catch (err) {
        Alert.alert(t("common.error"), apiErrorMessage(err, t), [
          { text: t("common.ok"), onPress: () => router.replace("/auth/login" as never) },
        ]);
      } finally {
        setBusy(false);
      }
    })();
  }, [router, t, token]);

  return (
    <AuthScroll>
      <View style={styles.box}>
        {busy ? (
          <ActivityIndicator color={accountColors.accent} />
        ) : (
          <Text style={styles.text}>{t("auth.verify.openTitle")}</Text>
        )}
      </View>
    </AuthScroll>
  );
}

const styles = StyleSheet.create({
  box: { paddingVertical: 48, alignItems: "center" },
  text: { color: accountColors.text, fontSize: 16 },
});
