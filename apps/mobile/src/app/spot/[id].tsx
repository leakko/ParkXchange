import { useFocusEffect } from "@react-navigation/native";
import { type Href, Stack, useLocalSearchParams, useRouter } from "expo-router";
import { useCallback, useMemo, useState } from "react";
import {
  ActivityIndicator,
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  View,
} from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";

import { accountColors } from "@/account/theme";
import {
  createOffer,
  fetchActiveReservations,
  getSpot,
  listMyOffers,
  listVehicles,
  withdrawOffer,
  withdrawSpot,
  type OfferResponse,
  type SpotFeature,
  type VehicleResponse,
} from "@/api/client";
import { apiErrorMessage } from "@/api/errors";
import { ensureEmailVerified } from "@/auth/requireEmailVerified";
import { useSession } from "@/hooks/useSession";
import { useActiveReservation } from "@/hooks/useSpotActions";
import { useTranslation } from "@/i18n";
import { SpotSheetBody } from "@/map/SpotSheetBody";
import { useConfirm } from "@/ui/ConfirmModal";

/**
 * Native form-sheet / modal spot detail. Replaces the in-map gorhom SpotSheet.
 */
export default function SpotDetailScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const spotId = id ? String(id) : "";
  const router = useRouter();
  const insets = useSafeAreaInsets();
  const { t } = useTranslation();
  const { confirm, alert } = useConfirm();
  const { signedIn } = useSession();

  const [spot, setSpot] = useState<SpotFeature | null>(null);
  const [loading, setLoading] = useState(true);
  const [vehicles, setVehicles] = useState<VehicleResponse[]>([]);
  const [pendingOffer, setPendingOffer] = useState<OfferResponse | null>(null);
  const [offerBusy, setOfferBusy] = useState(false);
  const [makingOffer, setMakingOffer] = useState(false);

  const {
    active,
    isOwner,
    isDriver,
    busy: exchangeBusy,
    markEnRoute,
    markReady,
    clearReady,
    cancel,
    refresh: refreshActive,
  } = useActiveReservation(signedIn);

  const activeForSpot =
    active && spotId && String(active.spot_id) === spotId ? active : null;

  const title = useMemo(() => {
    if (spot?.properties.is_mine) {
      return t("spotSheet.yourListing");
    }
    return spot?.properties.owner_name ?? t("spotSheet.nav.title");
  }, [spot, t]);

  const refreshSpot = useCallback(async () => {
    if (!spotId) {
      setSpot(null);
      setLoading(false);
      return;
    }
    try {
      const feature = await getSpot(spotId);
      setSpot(feature);
    } catch {
      setSpot(null);
    } finally {
      setLoading(false);
    }
  }, [spotId]);

  const refreshOffers = useCallback(async () => {
    if (!signedIn || !spotId) {
      setPendingOffer(null);
      return;
    }
    try {
      const mine = await listMyOffers();
      setPendingOffer(
        mine.find(
          (o) => String(o.spot_id) === spotId && o.status === "pending",
        ) ?? null,
      );
    } catch {
      setPendingOffer(null);
    }
  }, [signedIn, spotId]);

  const refreshVehicles = useCallback(async () => {
    if (!signedIn) {
      setVehicles([]);
      return;
    }
    try {
      setVehicles(await listVehicles());
    } catch {
      setVehicles([]);
    }
  }, [signedIn]);

  useFocusEffect(
    useCallback(() => {
      setLoading(true);
      void refreshSpot();
      void refreshOffers();
      void refreshVehicles();
      void refreshActive();
    }, [refreshSpot, refreshOffers, refreshVehicles, refreshActive]),
  );

  const requireSignIn = useCallback(() => {
    router.replace(`/auth/login?returnTo=${encodeURIComponent(`/spot/${spotId}`)}` as Href);
  }, [router, spotId]);

  const requireEmailVerified = useCallback(async (): Promise<boolean> => {
    return ensureEmailVerified({
      t,
      onUnauthorized: requireSignIn,
    });
  }, [requireSignIn, t]);

  const close = useCallback(() => {
    if (router.canGoBack()) {
      router.back();
    } else {
      router.replace("/" as Href);
    }
  }, [router]);

  return (
    <>
      <Stack.Screen
        options={{
          title,
          headerShown: true,
          headerStyle: { backgroundColor: accountColors.bg },
          headerTintColor: accountColors.text,
          headerTitleStyle: {
            color: accountColors.text,
            fontWeight: "600",
          },
          headerShadowVisible: false,
          contentStyle: { backgroundColor: accountColors.bg },
          // Native sheet: peek first (map still readable), expand when wanted.
          presentation: "formSheet",
          sheetAllowedDetents: [0.36, 0.85],
          sheetInitialDetentIndex: 0,
          sheetGrabberVisible: true,
          // Peek stays undimmed so other pins remain visible on the map.
          sheetLargestUndimmedDetentIndex: 0,
          headerRight: () => (
            <Pressable
              onPress={close}
              hitSlop={12}
              accessibilityRole="button"
              accessibilityLabel={t("common.close")}
            >
              <Text style={styles.close}>{t("common.close")}</Text>
            </Pressable>
          ),
        }}
      />

      {loading && !spot ? (
        <View style={styles.center}>
          <ActivityIndicator color={accountColors.accent} />
        </View>
      ) : !spot ? (
        <View style={styles.center}>
          <Text style={styles.missing}>{t("spotSheet.nav.missing")}</Text>
          <Pressable style={styles.closeBtn} onPress={close}>
            <Text style={styles.closeBtnText}>{t("common.close")}</Text>
          </Pressable>
        </View>
      ) : (
        <ScrollView
          style={styles.fill}
          contentContainerStyle={[
            styles.body,
            {
              paddingBottom: 24 + insets.bottom,
              ...(makingOffer ? { paddingBottom: 96 + insets.bottom } : null),
            },
          ]}
          keyboardShouldPersistTaps="handled"
          keyboardDismissMode="on-drag"
          bounces={false}
          overScrollMode="never"
        >
          <SpotSheetBody
            spot={spot}
            active={activeForSpot}
            pendingOffer={pendingOffer}
            vehicles={vehicles}
            isOwner={!!activeForSpot && isOwner}
            isDriver={!!activeForSpot && isDriver}
            busy={exchangeBusy || offerBusy}
            makingOffer={makingOffer}
            setMakingOffer={setMakingOffer}
            onExpandSheet={() => {
              /* native sheet is already near-full; scroll covers the form */
            }}
            onMakeOffer={async (s, vehicleId, exchangeAt, amountCents) => {
              if (!signedIn) {
                requireSignIn();
                return;
              }
              if (!(await requireEmailVerified())) {
                return;
              }
              setOfferBusy(true);
              try {
                await createOffer(String(s.id), {
                  vehicle_id: vehicleId,
                  exchange_at: exchangeAt,
                  amount_cents: amountCents,
                });
                await alert({
                  title: t("map.alert.offerSent.title"),
                  message: t("map.alert.offerSent.message"),
                  confirmLabel: t("common.ok"),
                });
                await Promise.all([refreshSpot(), refreshOffers(), refreshActive()]);
              } catch (err) {
                await alert({
                  title: t("map.alert.offerFailed.title"),
                  message: apiErrorMessage(err, t),
                  confirmLabel: t("common.ok"),
                });
              } finally {
                setOfferBusy(false);
              }
            }}
            onWithdrawOffer={async (offer) => {
              setOfferBusy(true);
              try {
                await withdrawOffer(offer.id);
                await refreshOffers();
              } catch (err) {
                await alert({
                  title: t("spotSheet.offer.withdrawFailed.title"),
                  message: apiErrorMessage(err, t),
                  confirmLabel: t("common.ok"),
                });
                throw err;
              } finally {
                setOfferBusy(false);
              }
            }}
            onAddVehicle={() => {
              router.push("/account/vehicles/new?from=offer" as Href);
            }}
            onEnRoute={() => void markEnRoute()}
            onReady={() => void markReady()}
            onUnready={() => void clearReady()}
            onCancel={() => {
              void cancel().then(() => {
                void refreshActive();
                void refreshSpot();
              });
            }}
            onEdit={(s) => {
              router.push(`/account/spots/${String(s.id)}` as Href);
            }}
            onViewOffers={(s) => {
              router.push(`/account/spots/${String(s.id)}` as Href);
            }}
            onWithdraw={(s) => {
              void (async () => {
                const ok = await confirm({
                  title: t("map.alert.withdrawListing.title"),
                  message: t("map.alert.withdrawListing.message"),
                  cancelLabel: t("common.cancel"),
                  confirmLabel: t("map.alert.withdraw.confirm"),
                  destructive: true,
                });
                if (!ok) {
                  return;
                }
                try {
                  await withdrawSpot(String(s.id));
                  close();
                } catch (err) {
                  await alert({
                    title: t("map.alert.withdrawFailed.title"),
                    message:
                      err instanceof Error ? err.message : t("common.error"),
                    confirmLabel: t("common.ok"),
                  });
                }
              })();
            }}
            onManageExchange={() => {
              void (async () => {
                await refreshActive();
                try {
                  const list = await fetchActiveReservations();
                  const match = list.find(
                    (r) => String(r.spot_id) === spotId,
                  );
                  if (match) {
                    router.push(
                      `/account/reservations/${match.id}` as Href,
                    );
                    return;
                  }
                } catch {
                  /* fall through */
                }
                router.push("/account/reservations" as Href);
              })();
            }}
          />
        </ScrollView>
      )}
    </>
  );
}

const styles = StyleSheet.create({
  fill: { flex: 1, backgroundColor: accountColors.bg },
  body: { paddingHorizontal: 20, paddingTop: 8, gap: 6 },
  center: {
    flex: 1,
    alignItems: "center",
    justifyContent: "center",
    padding: 24,
    backgroundColor: accountColors.bg,
    gap: 16,
  },
  missing: { color: accountColors.muted, fontSize: 15, textAlign: "center" },
  close: {
    color: accountColors.accent,
    fontSize: 16,
    fontWeight: "600",
    paddingHorizontal: 4,
  },
  closeBtn: {
    backgroundColor: accountColors.accent,
    borderRadius: 12,
    paddingHorizontal: 20,
    paddingVertical: 12,
  },
  closeBtnText: { color: "#fff", fontWeight: "600", fontSize: 15 },
});
