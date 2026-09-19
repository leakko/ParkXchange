import * as Google from "expo-auth-session/providers/google";
import * as WebBrowser from "expo-web-browser";
import { useEffect, useRef } from "react";

import { loginWithGoogle } from "@/api/client";
import { applySession } from "@/hooks/useSession";

WebBrowser.maybeCompleteAuthSession();

const webClientId = process.env.EXPO_PUBLIC_GOOGLE_WEB_CLIENT_ID ?? "";
const androidClientId = process.env.EXPO_PUBLIC_GOOGLE_ANDROID_CLIENT_ID ?? "";
const iosClientId = process.env.EXPO_PUBLIC_GOOGLE_IOS_CLIENT_ID ?? "";

export function googleSignInConfigured(): boolean {
  return Boolean(webClientId);
}

/** Hook that returns a prompt function for Google Sign-In via ID token. */
export function useGoogleSignIn(opts: {
  onError: (message: string) => void;
  onSuccess: () => void;
}) {
  const onErrorRef = useRef(opts.onError);
  const onSuccessRef = useRef(opts.onSuccess);
  onErrorRef.current = opts.onError;
  onSuccessRef.current = opts.onSuccess;

  const config: Partial<{
    clientId: string;
    androidClientId: string;
    iosClientId: string;
    webClientId: string;
  }> = {};
  if (webClientId) {
    config.clientId = webClientId;
    config.webClientId = webClientId;
  }
  if (androidClientId) {
    config.androidClientId = androidClientId;
  }
  if (iosClientId) {
    config.iosClientId = iosClientId;
  }

  const [request, response, promptAsync] = Google.useIdTokenAuthRequest(config);

  useEffect(() => {
    if (response?.type !== "success") {
      return;
    }
    const idToken = response.params.id_token;
    if (!idToken) {
      onErrorRef.current("Google did not return an ID token");
      return;
    }
    void (async () => {
      try {
        const session = await loginWithGoogle(idToken);
        await applySession(session.access_token, session.refresh_token);
        onSuccessRef.current();
      } catch (err) {
        onErrorRef.current(err instanceof Error ? err.message : "Google sign-in failed");
      }
    })();
  }, [response]);

  return {
    ready: Boolean(request) && googleSignInConfigured(),
    prompt: () => promptAsync(),
  };
}
