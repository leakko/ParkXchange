import { useRouter } from "expo-router";
import { useState } from "react";
import { ActivityIndicator, Pressable, ScrollView, Text, TextInput, View } from "react-native";

import { forgotPassword } from "@/api/client";
import { accountColors, accountStyles } from "@/account/theme";
import { useTranslation } from "@/i18n";

export default function ForgotPasswordScreen() {
  const { t } = useTranslation();
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const onSubmit = async () => {
    setBusy(true);
    setError(null);
    try {
      await forgotPassword(email.trim());
      setDone(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("common.error"));
    } finally {
      setBusy(false);
    }
  };

  return (
    <ScrollView style={accountStyles.screen} contentContainerStyle={accountStyles.scroll}>
      <Text style={accountStyles.title}>{t("auth.forgot.title")}</Text>
      <Text style={accountStyles.meta}>{t("auth.forgot.subtitle")}</Text>

      {done ? (
        <Text style={accountStyles.meta}>{t("auth.forgot.sent")}</Text>
      ) : (
        <>
          <View style={accountStyles.field}>
            <Text style={accountStyles.label}>{t("auth.email")}</Text>
            <TextInput
              style={accountStyles.input}
              autoCapitalize="none"
              keyboardType="email-address"
              value={email}
              onChangeText={setEmail}
            />
          </View>
          {error ? <Text style={accountStyles.error}>{error}</Text> : null}
          <Pressable
            style={[accountStyles.primary, busy && { opacity: 0.6 }]}
            disabled={busy}
            onPress={() => {
              void onSubmit();
            }}
          >
            {busy ? (
              <ActivityIndicator color={accountColors.text} />
            ) : (
              <Text style={accountStyles.primaryText}>{t("auth.forgot.submit")}</Text>
            )}
          </Pressable>
        </>
      )}

      <Pressable onPress={() => router.back()}>
        <Text style={[accountStyles.link, { marginTop: 16 }]}>{t("auth.backToLogin")}</Text>
      </Pressable>
    </ScrollView>
  );
}
