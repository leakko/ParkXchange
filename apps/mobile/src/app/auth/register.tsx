import { type Href, useLocalSearchParams, useRouter } from "expo-router";
import { useCallback, useState } from "react";
import {
  ActivityIndicator,
  Pressable,
  Text,
  View,
} from "react-native";

import { register } from "@/api/client";
import type { SessionResponse } from "@/api/client";
import { accountColors, accountStyles } from "@/account/theme";
import { AuthScroll } from "@/auth/AuthScroll";
import { AuthTextInput } from "@/auth/AuthTextInput";
import { authErrorMessage } from "@/auth/errors";
import { apiFieldErrors } from "@/auth/fieldErrors";
import { GoogleButton } from "@/auth/GoogleButton";
import { useGoogleSignIn, googleSignInConfigured } from "@/auth/google";
import { scheduleLoginGrantToast } from "@/auth/loginGrantToast";
import { normalizePhoneInput } from "@/auth/phone";
import { PasswordField } from "@/auth/PasswordField";
import { applySession } from "@/hooks/useSession";
import { useTranslation } from "@/i18n";
import { useToast } from "@/ui/toast";

function returnPath(raw: string | string[] | undefined): Href {
  const value = Array.isArray(raw) ? raw[0] : raw;
  if (value && value.startsWith("/")) {
    return value as Href;
  }
  return "/";
}

export default function RegisterScreen() {
  const { t } = useTranslation();
  const { show } = useToast();
  const router = useRouter();
  const params = useLocalSearchParams<{ returnTo?: string }>();
  const [displayName, setDisplayName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [phone, setPhone] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});

  const onGoogleError = useCallback(
    (err: unknown) => {
      setError(authErrorMessage(err, t));
      setFieldErrors({});
      setBusy(false);
    },
    [t],
  );
  const finish = useCallback(
    (session?: SessionResponse) => {
      scheduleLoginGrantToast(session, show, t);
      const path = returnPath(params.returnTo);
      router.dismissTo(path);
    },
    [params.returnTo, router, show, t],
  );
  const google = useGoogleSignIn({ onError: onGoogleError, onSuccess: finish });

  const clearField = (key: string) => {
    setFieldErrors((prev) => {
      if (!prev[key]) {
        return prev;
      }
      const next = { ...prev };
      delete next[key];
      return next;
    });
  };

  const onSubmit = async () => {
    setBusy(true);
    setError(null);
    setFieldErrors({});
    try {
      const payload: {
        email: string;
        password: string;
        display_name: string;
        phone?: string;
      } = {
        email: email.trim(),
        password,
        display_name: displayName.trim(),
      };
      const normalisedPhone = normalizePhoneInput(phone);
      if (normalisedPhone) {
        payload.phone = normalisedPhone;
      }
      const session = await register(payload);
      await applySession(session.access_token, session.refresh_token);
      finish(session);
    } catch (err) {
      const fields = apiFieldErrors(err, t);
      setFieldErrors(fields);
      setError(
        Object.keys(fields).length > 0
          ? t("auth.error.validation")
          : authErrorMessage(err, t),
      );
    } finally {
      setBusy(false);
    }
  };

  return (
    <AuthScroll>
      <Text style={accountStyles.title}>{t("auth.register.title")}</Text>
      <Text style={accountStyles.meta}>{t("auth.register.subtitle")}</Text>
      <Text style={[accountStyles.meta, { marginBottom: 8 }]}>
        {t("auth.verify.note")}
      </Text>

      <View style={accountStyles.field}>
        <Text style={accountStyles.label}>{t("auth.displayName")}</Text>
        <AuthTextInput
          invalid={!!fieldErrors.display_name}
          value={displayName}
          onChangeText={(v) => {
            setDisplayName(v);
            clearField("display_name");
          }}
        />
        {fieldErrors.display_name ? (
          <Text style={accountStyles.fieldError}>{fieldErrors.display_name}</Text>
        ) : null}
      </View>
      <View style={accountStyles.field}>
        <Text style={accountStyles.label}>{t("auth.email")}</Text>
        <AuthTextInput
          autoCapitalize="none"
          keyboardType="email-address"
          invalid={!!fieldErrors.email}
          value={email}
          onChangeText={(v) => {
            setEmail(v);
            clearField("email");
          }}
        />
        {fieldErrors.email ? (
          <Text style={accountStyles.fieldError}>{fieldErrors.email}</Text>
        ) : null}
      </View>
      <View style={accountStyles.field}>
        <Text style={accountStyles.label}>{t("auth.password")}</Text>
        <PasswordField
          invalid={!!fieldErrors.password}
          value={password}
          onChangeText={(v) => {
            setPassword(v);
            clearField("password");
          }}
        />
        {fieldErrors.password ? (
          <Text style={accountStyles.fieldError}>{fieldErrors.password}</Text>
        ) : null}
      </View>
      <View style={accountStyles.field}>
        <Text style={accountStyles.label}>{t("auth.phoneOptional")}</Text>
        <AuthTextInput
          keyboardType="phone-pad"
          placeholder={t("auth.phone.placeholder")}
          placeholderTextColor={accountColors.window}
          invalid={!!fieldErrors.phone}
          value={phone}
          onChangeText={(v) => {
            setPhone(v);
            clearField("phone");
          }}
        />
        {fieldErrors.phone ? (
          <Text style={accountStyles.fieldError}>{fieldErrors.phone}</Text>
        ) : (
          <Text style={accountStyles.meta}>{t("auth.phone.hint")}</Text>
        )}
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
          <Text style={accountStyles.primaryText}>{t("auth.register.submit")}</Text>
        )}
      </Pressable>

      {googleSignInConfigured() ? (
        <GoogleButton
          disabled={busy || !google.ready}
          onPress={() => {
            setBusy(true);
            setError(null);
            setFieldErrors({});
            void google.prompt().finally(() => setBusy(false));
          }}
        />
      ) : null}

      <Pressable
        onPress={() =>
          router.push(
            `/auth/login?returnTo=${encodeURIComponent(
              Array.isArray(params.returnTo) ? params.returnTo[0] ?? "/" : params.returnTo ?? "/",
            )}` as Href,
          )
        }
      >
        <Text style={[accountStyles.link, { marginTop: 16 }]}>{t("auth.register.toLogin")}</Text>
      </Pressable>
    </AuthScroll>
  );
}
