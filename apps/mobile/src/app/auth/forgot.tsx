import { useRouter } from "expo-router";
import { useState } from "react";
import { ActivityIndicator, Pressable, Text, View } from "react-native";

import { forgotPassword } from "@/api/client";
import { accountColors, accountStyles } from "@/account/theme";
import { AuthScroll } from "@/auth/AuthScroll";
import { AuthTextInput } from "@/auth/AuthTextInput";
import { authErrorMessage } from "@/auth/errors";
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
      setError(authErrorMessage(err, t));
    } finally {
      setBusy(false);
    }
  };

  return (
    <AuthScroll>
      <Text style={accountStyles.title}>{t("auth.forgot.title")}</Text>
      <Text style={accountStyles.meta}>{t("auth.forgot.subtitle")}</Text>

      {done ? (
        <Text style={accountStyles.meta}>{t("auth.forgot.sent")}</Text>
      ) : (
        <>
          <View style={accountStyles.field}>
            <Text style={accountStyles.label}>{t("auth.email")}</Text>
            <AuthTextInput
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
    </AuthScroll>
  );
}
