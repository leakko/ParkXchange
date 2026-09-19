import { type Href, useLocalSearchParams, useRouter } from "expo-router";
import { useState } from "react";
import { ActivityIndicator, Pressable, ScrollView, Text, TextInput, View } from "react-native";

import { resetPassword } from "@/api/client";
import { accountColors, accountStyles } from "@/account/theme";
import { useTranslation } from "@/i18n";

export default function ResetPasswordScreen() {
  const { t } = useTranslation();
  const router = useRouter();
  const params = useLocalSearchParams<{ token?: string }>();
  const token = Array.isArray(params.token) ? params.token[0] : params.token ?? "";
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const onSubmit = async () => {
    if (password !== confirm) {
      setError(t("auth.reset.mismatch"));
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await resetPassword(token, password);
      router.replace("/auth/login" as Href);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("common.error"));
    } finally {
      setBusy(false);
    }
  };

  return (
    <ScrollView style={accountStyles.screen} contentContainerStyle={accountStyles.scroll}>
      <Text style={accountStyles.title}>{t("auth.reset.title")}</Text>
      <Text style={accountStyles.meta}>{t("auth.reset.subtitle")}</Text>

      <View style={accountStyles.field}>
        <Text style={accountStyles.label}>{t("auth.password")}</Text>
        <TextInput
          style={accountStyles.input}
          secureTextEntry
          value={password}
          onChangeText={setPassword}
        />
      </View>
      <View style={accountStyles.field}>
        <Text style={accountStyles.label}>{t("auth.reset.confirm")}</Text>
        <TextInput
          style={accountStyles.input}
          secureTextEntry
          value={confirm}
          onChangeText={setConfirm}
        />
      </View>

      {error ? <Text style={accountStyles.error}>{error}</Text> : null}

      <Pressable
        style={[accountStyles.primary, busy && { opacity: 0.6 }]}
        disabled={busy || !token}
        onPress={() => {
          void onSubmit();
        }}
      >
        {busy ? (
          <ActivityIndicator color={accountColors.text} />
        ) : (
          <Text style={accountStyles.primaryText}>{t("auth.reset.submit")}</Text>
        )}
      </Pressable>
    </ScrollView>
  );
}
