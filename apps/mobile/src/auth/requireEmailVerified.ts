import { Alert } from "react-native";

import { ApiError, getMe, resendEmailVerification } from "@/api/client";
import { apiErrorMessage } from "@/api/errors";
import type { TranslationKey } from "@/i18n";

type Translate = (
  key: TranslationKey,
  params?: Record<string, string | number>,
) => string;

type Options = {
  t: Translate;
  onUnauthorized?: () => void;
};

/** Returns true when the signed-in user may announce or reserve. */
export async function ensureEmailVerified({
  t,
  onUnauthorized,
}: Options): Promise<boolean> {
  try {
    const me = await getMe();
    if (me.email_verified) {
      return true;
    }
  } catch (err) {
    if (err instanceof ApiError && err.code === "unauthorized") {
      onUnauthorized?.();
      return false;
    }
    Alert.alert(t("common.error"), apiErrorMessage(err, t));
    return false;
  }

  Alert.alert(t("auth.verify.title"), t("auth.verify.body"), [
    { text: t("common.cancel"), style: "cancel" },
    {
      text: t("auth.verify.resend"),
      onPress: () => {
        void (async () => {
          try {
            await resendEmailVerification();
            Alert.alert(t("auth.verify.title"), t("auth.verify.sent"));
          } catch (err) {
            Alert.alert(t("common.error"), apiErrorMessage(err, t));
          }
        })();
      },
    },
  ]);
  return false;
}
