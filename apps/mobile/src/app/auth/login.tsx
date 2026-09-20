import { type Href, useLocalSearchParams, useRouter } from "expo-router";
import { useCallback, useState } from "react";
import {
  ActivityIndicator,
  Pressable,
  Text,
  View,
} from "react-native";

import { login } from "@/api/client";
import { accountColors, accountStyles } from "@/account/theme";
import { AuthScroll } from "@/auth/AuthScroll";
import { AuthTextInput } from "@/auth/AuthTextInput";
import { authErrorMessage } from "@/auth/errors";
import { GoogleButton } from "@/auth/GoogleButton";
import { useGoogleSignIn, googleSignInConfigured } from "@/auth/google";
import { PasswordField } from "@/auth/PasswordField";
import { applySession } from "@/hooks/useSession";
import { useTranslation } from "@/i18n";

function returnPath(raw: string | string[] | undefined): Href {
  const value = Array.isArray(raw) ? raw[0] : raw;
  if (value && value.startsWith("/")) {
    return value as Href;
  }
  return "/";
}

export default function LoginScreen() {
  const { t } = useTranslation();
  const router = useRouter();
  const params = useLocalSearchParams<{ returnTo?: string }>();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const onGoogleError = useCallback(
    (err: unknown) => {
      setError(authErrorMessage(err, t));
      setBusy(false);
    },
    [t],
  );
  const finish = useCallback(() => {
    router.replace(returnPath(params.returnTo));
  }, [params.returnTo, router]);
  const google = useGoogleSignIn({ onError: onGoogleError, onSuccess: finish });

  const onSubmit = async () => {
    setBusy(true);
    setError(null);
    try {
      const session = await login(email.trim(), password);
      await applySession(session.access_token, session.refresh_token);
      finish();
    } catch (err) {
      setError(authErrorMessage(err, t));
    } finally {
      setBusy(false);
    }
  };

  return (
    <AuthScroll>
      <Text style={accountStyles.title}>{t("auth.login.title")}</Text>
      <Text style={accountStyles.meta}>{t("auth.login.subtitle")}</Text>

      <View style={accountStyles.field}>
        <Text style={accountStyles.label}>{t("auth.email")}</Text>
        <AuthTextInput
          autoCapitalize="none"
          autoComplete="email"
          keyboardType="email-address"
          value={email}
          onChangeText={setEmail}
        />
      </View>
      <View style={accountStyles.field}>
        <Text style={accountStyles.label}>{t("auth.password")}</Text>
        <PasswordField value={password} onChangeText={setPassword} />
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
          <Text style={accountStyles.primaryText}>{t("auth.login.submit")}</Text>
        )}
      </Pressable>

      {googleSignInConfigured() ? (
        <GoogleButton
          disabled={busy || !google.ready}
          onPress={() => {
            setBusy(true);
            setError(null);
            void google.prompt().finally(() => setBusy(false));
          }}
        />
      ) : null}

      <Pressable
        onPress={() =>
          router.push(
            `/auth/register?returnTo=${encodeURIComponent(
              Array.isArray(params.returnTo) ? params.returnTo[0] ?? "/" : params.returnTo ?? "/",
            )}` as Href,
          )
        }
      >
        <Text style={[accountStyles.link, { marginTop: 16 }]}>{t("auth.login.toRegister")}</Text>
      </Pressable>
      <Pressable onPress={() => router.push("/auth/forgot" as Href)}>
        <Text style={accountStyles.link}>{t("auth.login.forgot")}</Text>
      </Pressable>
    </AuthScroll>
  );
}
