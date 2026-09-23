import type { SessionResponse } from "@/api/client";
import type { ToastPayload } from "@/ui/toast";

/** True when this session credited the weekly (or first) login bonus. */
export function sessionGrantedLoginPoints(session: SessionResponse): boolean {
  return (session.login_grant_cents ?? 0) > 0;
}

/** Let login navigation / map settle before the teaching toast appears. */
const LOGIN_GRANT_TOAST_DELAY_MS = 5000;

type ShowToast = (toast: ToastPayload) => void;

/** Queue the login-grant toast after a short delay when the session paid the bonus. */
export function scheduleLoginGrantToast(
  session: SessionResponse | undefined,
  show: ShowToast,
  copy: { title: string; body: string },
): void {
  if (!session || !sessionGrantedLoginPoints(session)) {
    return;
  }
  setTimeout(() => {
    show({
      title: copy.title,
      body: copy.body,
      durationMs: 5500,
    });
  }, LOGIN_GRANT_TOAST_DELAY_MS);
}
