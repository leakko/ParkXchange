import * as SecureStore from "expo-secure-store";

const ACCESS_KEY = "parkxchange.access_token";
const REFRESH_KEY = "parkxchange.refresh_token";

const clearedListeners = new Set<() => void>();

/** Subscribe to session wipes triggered outside React (e.g. apiFetch 401). */
export function onSessionCleared(listener: () => void): () => void {
  clearedListeners.add(listener);
  return () => {
    clearedListeners.delete(listener);
  };
}

function notifyCleared() {
  for (const listener of clearedListeners) {
    listener();
  }
}

export async function getAccessToken(): Promise<string | null> {
  return SecureStore.getItemAsync(ACCESS_KEY);
}

export async function getRefreshToken(): Promise<string | null> {
  return SecureStore.getItemAsync(REFRESH_KEY);
}

export async function setSession(accessToken: string, refreshToken: string): Promise<void> {
  await SecureStore.setItemAsync(ACCESS_KEY, accessToken);
  await SecureStore.setItemAsync(REFRESH_KEY, refreshToken);
}

export async function clearSession(): Promise<void> {
  await SecureStore.deleteItemAsync(ACCESS_KEY);
  await SecureStore.deleteItemAsync(REFRESH_KEY);
}

/** Clear tokens and tell useSession the UI is signed out. */
export async function clearSessionAndNotify(): Promise<void> {
  await clearSession();
  notifyCleared();
}
