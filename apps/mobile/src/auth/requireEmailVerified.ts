import { ApiError, getMe, resendEmailVerification } from "@/api/client";
import { apiErrorMessage } from "@/api/errors";
import type { TranslationKey } from "@/i18n";
import { appAlert, appConfirm } from "@/ui/ConfirmModal";

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
    await appAlert({
      title: t("common.error"),
      message: apiErrorMessage(err, t),
      confirmLabel: t("common.ok"),
    });
    return false;
  }

  const resend = await appConfirm({
    title: t("auth.verify.title"),
    message: t("auth.verify.body"),
    cancelLabel: t("common.cancel"),
    confirmLabel: t("auth.verify.resend"),
  });
  if (resend) {
    try {
      await resendEmailVerification();
      await appAlert({
        title: t("auth.verify.title"),
        message: t("auth.verify.sent"),
        confirmLabel: t("common.ok"),
      });
    } catch (err) {
      await appAlert({
        title: t("common.error"),
        message: apiErrorMessage(err, t),
        confirmLabel: t("common.ok"),
      });
    }
  }
  return false;
}
