import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import {
  Animated,
  Pressable,
  StyleSheet,
  Text,
  View,
} from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";

import { accountColors } from "@/account/theme";

export type ToastPayload = {
  title: string;
  body?: string;
  /** Auto-dismiss after this many ms. Default 4500. */
  durationMs?: number;
};

type ToastApi = {
  show: (toast: ToastPayload) => void;
};

const ToastContext = createContext<ToastApi | null>(null);

type Queued = ToastPayload & { id: number };

/**
 * Soft in-app notice: does not block the UI. Auto-dismisses unless tapped away.
 * Use for exchange/offer updates while the app is foregrounded; keep Alert for
 * confirmations the user starts.
 */
export function ToastProvider({ children }: { children: ReactNode }) {
  const [current, setCurrent] = useState<Queued | null>(null);
  const queueRef = useRef<Queued[]>([]);
  const nextId = useRef(1);
  const opacity = useRef(new Animated.Value(0)).current;
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const insets = useSafeAreaInsets();

  const clearTimer = () => {
    if (timerRef.current) {
      clearTimeout(timerRef.current);
      timerRef.current = null;
    }
  };

  const hide = useCallback(() => {
    clearTimer();
    Animated.timing(opacity, {
      toValue: 0,
      duration: 180,
      useNativeDriver: true,
    }).start(({ finished }) => {
      if (!finished) {
        return;
      }
      setCurrent(null);
    });
  }, [opacity]);

  const present = useCallback(
    (toast: Queued) => {
      setCurrent(toast);
      opacity.setValue(0);
      Animated.timing(opacity, {
        toValue: 1,
        duration: 180,
        useNativeDriver: true,
      }).start();
      clearTimer();
      const ms = toast.durationMs ?? 4500;
      timerRef.current = setTimeout(() => hide(), ms);
    },
    [hide, opacity],
  );

  useEffect(() => {
    if (current) {
      return;
    }
    const next = queueRef.current.shift();
    if (next) {
      present(next);
    }
  }, [current, present]);

  useEffect(() => () => clearTimer(), []);

  const show = useCallback(
    (toast: ToastPayload) => {
      const queued: Queued = { ...toast, id: nextId.current++ };
      if (current) {
        queueRef.current.push(queued);
        return;
      }
      present(queued);
    },
    [current, present],
  );

  const api = useMemo(() => ({ show }), [show]);

  return (
    <ToastContext.Provider value={api}>
      {children}
      {current ? (
        <View
          pointerEvents="box-none"
          style={[styles.host, { top: insets.top + 12 }]}
        >
          <Animated.View style={{ opacity }}>
            <Pressable
              onPress={hide}
              style={styles.card}
              accessibilityRole="alert"
              accessibilityLiveRegion="polite"
            >
              <Text style={styles.title}>{current.title}</Text>
              {current.body ? (
                <Text style={styles.body}>{current.body}</Text>
              ) : null}
            </Pressable>
          </Animated.View>
        </View>
      ) : null}
    </ToastContext.Provider>
  );
}

export function useToast(): ToastApi {
  const ctx = useContext(ToastContext);
  if (!ctx) {
    throw new Error("useToast must be used within ToastProvider");
  }
  return ctx;
}

const styles = StyleSheet.create({
  host: {
    position: "absolute",
    left: 16,
    right: 16,
    zIndex: 2000,
    elevation: 20,
  },
  card: {
    backgroundColor: accountColors.card,
    borderColor: accountColors.border,
    borderWidth: 1,
    borderRadius: 14,
    paddingHorizontal: 16,
    paddingVertical: 14,
    gap: 4,
    shadowColor: "#000",
    shadowOpacity: 0.35,
    shadowRadius: 12,
    shadowOffset: { width: 0, height: 4 },
  },
  title: {
    color: accountColors.text,
    fontSize: 16,
    fontWeight: "700",
  },
  body: {
    color: accountColors.hint,
    fontSize: 14,
    lineHeight: 20,
  },
});
