import {
  GoogleSignin,
  isErrorWithCode,
  isSuccessResponse,
  statusCodes,
} from "@react-native-google-signin/google-signin";
import { useCallback, useEffect, useRef } from "react";

import { loginWithGoogle } from "@/api/client";
import { applySession } from "@/hooks/useSession";

const webClientId = process.env.EXPO_PUBLIC_GOOGLE_WEB_CLIENT_ID ?? "";
const iosClientId = process.env.EXPO_PUBLIC_GOOGLE_IOS_CLIENT_ID ?? "";

let configured = false;

function ensureConfigured() {
  if (configured || !webClientId) {
    return;
  }
  GoogleSignin.configure({
    webClientId,
    iosClientId: iosClientId || undefined,
    offlineAccess: false,
  });
  configured = true;
}

export function googleSignInConfigured(): boolean {
  return Boolean(webClientId);
}

/** Native Google Sign-In → ID token → ParkXchange session. */
export function useGoogleSignIn(opts: {
  onError: (err: unknown) => void;
  onSuccess: () => void;
}) {
  const onErrorRef = useRef(opts.onError);
  const onSuccessRef = useRef(opts.onSuccess);
  onErrorRef.current = opts.onError;
  onSuccessRef.current = opts.onSuccess;

  useEffect(() => {
    ensureConfigured();
  }, []);

  const prompt = useCallback(async () => {
    ensureConfigured();
    if (!webClientId) {
      onErrorRef.current(new Error("Google Sign-In is not configured"));
      return;
    }
    try {
      await GoogleSignin.hasPlayServices({ showPlayServicesUpdateDialog: true });
      const response = await GoogleSignin.signIn();
      if (!isSuccessResponse(response)) {
        return;
      }
      const idToken = response.data.idToken;
      if (!idToken) {
        onErrorRef.current(new Error("Google did not return an ID token"));
        return;
      }
      const session = await loginWithGoogle(idToken);
      await applySession(session.access_token, session.refresh_token);
      onSuccessRef.current();
    } catch (err) {
      if (isErrorWithCode(err) && err.code === statusCodes.SIGN_IN_CANCELLED) {
        return;
      }
      onErrorRef.current(err);
    }
  }, []);

  return {
    ready: googleSignInConfigured(),
    prompt,
  };
}
