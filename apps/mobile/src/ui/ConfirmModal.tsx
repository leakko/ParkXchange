import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type MutableRefObject,
  type ReactNode,
} from "react";
import { Alert as RNAlert, Modal, Pressable, StyleSheet, Text, View } from "react-native";

import { accountColors, accountStyles } from "@/account/theme";

export type ConfirmRequest = {
  title: string;
  message: string;
  cancelLabel: string;
  confirmLabel: string;
  /** Danger styling on the confirm button (delete / cancel exchange). */
  destructive?: boolean;
};

export type AlertRequest = {
  title: string;
  message: string;
  confirmLabel: string;
};

type ConfirmApi = {
  /** Two-button dialog. Resolves `true` if confirm, `false` if cancel / dismiss. */
  confirm: (req: ConfirmRequest) => Promise<boolean>;
  /** Single OK dialog. Resolves when dismissed. */
  alert: (req: AlertRequest) => Promise<void>;
};

const ConfirmContext = createContext<ConfirmApi | null>(null);

/** Set while ConfirmProvider is mounted — for non-React call sites (geofence). */
let confirmBridge: ConfirmApi | null = null;

export function getConfirmBridge(): ConfirmApi | null {
  return confirmBridge;
}

type PendingConfirm = ConfirmRequest & {
  kind: "confirm";
  resolve: (ok: boolean) => void;
};

type PendingAlert = AlertRequest & {
  kind: "alert";
  resolve: () => void;
};

type Pending = PendingConfirm | PendingAlert;

type HostApi = {
  enqueue: (item: Pending) => void;
};

/**
 * In-app dialogs matching the account-delete look. Prefer this over system
 * Alert.alert for exchange confirms and notification-adjacent prompts.
 *
 * Dialog visibility state lives in ConfirmHost (sibling of children) so opening
 * a modal does not re-render the map / SpotSheet tree under the provider.
 */
export function ConfirmProvider({ children }: { children: ReactNode }) {
  const hostRef = useRef<HostApi | null>(null);

  const api = useMemo<ConfirmApi>(
    () => ({
      confirm: (req) =>
        new Promise<boolean>((resolve) => {
          const host = hostRef.current;
          if (!host) {
            resolve(false);
            return;
          }
          host.enqueue({ ...req, kind: "confirm", resolve });
        }),
      alert: (req) =>
        new Promise<void>((resolve) => {
          const host = hostRef.current;
          if (!host) {
            resolve();
            return;
          }
          host.enqueue({ ...req, kind: "alert", resolve });
        }),
    }),
    [],
  );

  useEffect(() => {
    confirmBridge = api;
    return () => {
      if (confirmBridge === api) {
        confirmBridge = null;
      }
    };
  }, [api]);

  return (
    <ConfirmContext.Provider value={api}>
      {children}
      <ConfirmHost hostRef={hostRef} />
    </ConfirmContext.Provider>
  );
}

function ConfirmHost({
  hostRef,
}: {
  hostRef: MutableRefObject<HostApi | null>;
}) {
  const [pending, setPending] = useState<Pending | null>(null);
  const queueRef = useRef<Pending[]>([]);

  const enqueue = useCallback((item: Pending) => {
    setPending((current) => {
      if (current) {
        queueRef.current.push(item);
        return current;
      }
      return item;
    });
  }, []);

  const finish = useCallback((result: boolean) => {
    setPending((current) => {
      if (!current) {
        return null;
      }
      const next = queueRef.current.shift() ?? null;
      // Resolve after this updater so the Modal can unmount before the caller
      // opens another dialog (e.g. far-away confirm after ready confirm).
      queueMicrotask(() => {
        if (current.kind === "confirm") {
          current.resolve(result);
        } else {
          current.resolve();
        }
      });
      return next;
    });
  }, []);

  useEffect(() => {
    hostRef.current = { enqueue };
    return () => {
      hostRef.current = null;
    };
  }, [enqueue, hostRef]);

  if (pending == null) {
    return null;
  }

  return (
    <Modal
      visible
      transparent
      animationType="fade"
      statusBarTranslucent
      onRequestClose={() => finish(false)}
    >
      <View style={styles.backdrop}>
        <View style={styles.card} accessibilityViewIsModal>
          <Text style={styles.title}>{pending.title}</Text>
          <Text style={styles.body}>{pending.message}</Text>
          {pending.kind === "confirm" ? (
            <>
              <Pressable
                style={[
                  pending.destructive
                    ? accountStyles.danger
                    : accountStyles.primary,
                  { marginTop: 8 },
                ]}
                onPress={() => finish(true)}
              >
                <Text
                  style={
                    pending.destructive
                      ? accountStyles.dangerText
                      : accountStyles.primaryText
                  }
                >
                  {pending.confirmLabel}
                </Text>
              </Pressable>
              <Pressable
                style={styles.cancelHit}
                onPress={() => finish(false)}
              >
                <Text style={styles.cancelText}>{pending.cancelLabel}</Text>
              </Pressable>
            </>
          ) : (
            <Pressable
              style={[accountStyles.primary, { marginTop: 8 }]}
              onPress={() => finish(true)}
            >
              <Text style={accountStyles.primaryText}>
                {pending.confirmLabel}
              </Text>
            </Pressable>
          )}
        </View>
      </View>
    </Modal>
  );
}

export function useConfirm(): ConfirmApi {
  const ctx = useContext(ConfirmContext);
  if (!ctx) {
    throw new Error("useConfirm must be used within ConfirmProvider");
  }
  return ctx;
}

/** Imperative helpers for modules outside React (fall back to system Alert). */
export async function appAlert(req: AlertRequest): Promise<void> {
  const bridge = getConfirmBridge();
  if (bridge) {
    await bridge.alert(req);
    return;
  }
  await new Promise<void>((resolve) => {
    RNAlert.alert(req.title, req.message, [
      { text: req.confirmLabel, onPress: () => resolve() },
    ]);
  });
}

export async function appConfirm(req: ConfirmRequest): Promise<boolean> {
  const bridge = getConfirmBridge();
  if (bridge) {
    return bridge.confirm(req);
  }
  return new Promise((resolve) => {
    RNAlert.alert(
      req.title,
      req.message,
      [
        {
          text: req.cancelLabel,
          style: "cancel",
          onPress: () => resolve(false),
        },
        {
          text: req.confirmLabel,
          style: req.destructive ? "destructive" : "default",
          onPress: () => resolve(true),
        },
      ],
      { cancelable: true, onDismiss: () => resolve(false) },
    );
  });
}

const styles = StyleSheet.create({
  backdrop: {
    flex: 1,
    backgroundColor: "rgba(0,0,0,0.55)",
    justifyContent: "center",
    padding: 24,
  },
  card: {
    backgroundColor: accountColors.card,
    borderRadius: 16,
    padding: 20,
    borderWidth: 1,
    borderColor: accountColors.border,
    gap: 8,
  },
  title: {
    color: accountColors.text,
    fontSize: 18,
    fontWeight: "700",
  },
  body: {
    color: accountColors.muted,
    fontSize: 14,
    lineHeight: 20,
    marginBottom: 8,
  },
  cancelHit: {
    paddingVertical: 10,
    alignItems: "center",
  },
  cancelText: {
    color: accountColors.muted,
    fontSize: 15,
    fontWeight: "600",
    textAlign: "center",
  },
});
