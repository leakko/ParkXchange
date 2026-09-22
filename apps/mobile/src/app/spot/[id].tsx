import { Ionicons } from "@expo/vector-icons";
import { type Href, useLocalSearchParams, useRouter } from "expo-router";
import { useCallback, useEffect, useMemo, useState } from "react";
import {
  ActivityIndicator,
  Pressable,
  StyleSheet,
  Text,
  View,
} from "react-native";
import { ScrollView } from "react-native-gesture-handler";
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
import { firstGivenName } from "@/i18n/catalogLabels";
import { SpotSheetBody } from "@/map/SpotSheetBody";
import {
  peekOpenSpot,
  subscribeOpenSpot,
} from "@/map/spotSheetHandoff";
import { useConfirm } from "@/ui/ConfirmModal";

/**
 * Spot form sheet: resize only via the top grabber.
 * Body uses RNGH ScrollView so vertical pans stay with content scroll and never
 * hand off to the form sheet (RN ScrollView + formSheet still nested-scrolls).
 */
export default function SpotDetailScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const routeId = id ? String(id) : "";
  const router = useRouter();
  const insets = useSafeAreaInsets();
  const { t } = useTranslation();
  const { confirm, alert } = useConfirm();
  const { signedIn } = useSession();

  const [spot, setSpot] = useState<SpotFeature | null>(() =>
    routeId ? peekOpenSpot(routeId) : null,
  );
  const spotId = spot ? String(spot.id) : routeId;
  const [loading, setLoading] = useState(() => !spot);
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
    if (!spot) {
      return t("spotSheet.nav.title");
    }
    if (spot.properties.is_mine) {
      return t("spotSheet.yourListing");
    }
    return t("spotSheet.publishedBy", {
      name: firstGivenName(spot.properties.owner_name),
    });
  }, [spot, t]);

  const ratingLabel =
    spot?.properties.owner_rating != null
      ? t("spotSheet.rating", {
          score: spot.properties.owner_rating.toFixed(1),
        })
      : null;

  useEffect(() => {
    return subscribeOpenSpot((next) => {
      setSpot(next);
      setLoading(false);
      setMakingOffer(false);
      setPendingOffer(null);
      const nextId = String(next.id);
      if (nextId !== routeId) {
        router.setParams({ id: nextId });
      }
    });
  }, [routeId, router]);

  const refreshSpot = useCallback(async (id: string) => {
    if (!id) {
      return;
    }
    try {
      setSpot(await getSpot(id));
    } catch {
      /* keep painted feature */
    } finally {
      setLoading(false);
    }
  }, []);

  const refreshOffers = useCallback(async (id: string) => {
    if (!signedIn || !id) {
      setPendingOffer(null);
      return;
    }
    try {
      const mine = await listMyOffers();
      setPendingOffer(
        mine.find(
          (o) => String(o.spot_id) === id && o.status === "pending",
        ) ?? null,
      );
    } catch {
      setPendingOffer(null);
    }
  }, [signedIn]);

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

  useEffect(() => {
    if (!spotId) {
      setLoading(false);
      return;
    }
    void refreshSpot(spotId);
    void refreshOffers(spotId);
    void refreshVehicles();
    void refreshActive();
  }, [spotId, refreshSpot, refreshOffers, refreshVehicles, refreshActive]);

  const requireSignIn = useCallback(() => {
    router.replace(
      `/auth/login?returnTo=${encodeURIComponent(`/spot/${spotId}`)}` as Href,
    );
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
    <View style={[styles.fill, { paddingTop: 4 }]}>
      {/* Fat drag chrome — only this strip (plus system grabber) resizes. */}
      <View style={styles.grabberZone} accessibilityRole="adjustable">
        <View style={styles.grabber} />
      </View>

      <View style={styles.header}>
        <View style={styles.titleRow}>
          <Text style={styles.title} numberOfLines={1}>
            {title}
          </Text>
          {ratingLabel ? (
            <Text style={styles.rating} numberOfLines={1}>
              {ratingLabel}
            </Text>
          ) : null}
        </View>
        <Pressable
          onPress={close}
          hitSlop={12}
          accessibilityRole="button"
          accessibilityLabel={t("common.close")}
          style={styles.closeHit}
        >
          <Ionicons name="close" size={26} color={accountColors.text} />
        </Pressable>
      </View>

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
          bounces
          alwaysBounceVertical={false}
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
              /* resize only via grabber */
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
                await Promise.all([
                  refreshSpot(spotId),
                  refreshOffers(spotId),
                  refreshActive(),
                ]);
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
                await refreshOffers(spotId);
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
                void refreshSpot(spotId);
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
    </View>
  );
}

const styles = StyleSheet.create({
  fill: { flex: 1, backgroundColor: accountColors.bg },
  grabberZone: {
    alignItems: "center",
    justifyContent: "center",
    paddingTop: 8,
    paddingBottom: 12,
    minHeight: 36,
  },
  grabber: {
    width: 56,
    height: 8,
    borderRadius: 4,
    backgroundColor: "#6B8494",
  },
  header: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    paddingHorizontal: 20,
    paddingBottom: 8,
    gap: 10,
  },
  titleRow: {
    flex: 1,
    flexDirection: "row",
    alignItems: "center",
    gap: 8,
    minWidth: 0,
  },
  title: {
    flexShrink: 1,
    color: accountColors.text,
    fontSize: 16,
    fontWeight: "700",
  },
  rating: {
    color: accountColors.muted,
    fontSize: 13,
    fontWeight: "600",
    flexShrink: 0,
  },
  body: { paddingHorizontal: 20, paddingTop: 2, gap: 8 },
  center: {
    flex: 1,
    alignItems: "center",
    justifyContent: "center",
    padding: 24,
    gap: 16,
  },
  missing: { color: accountColors.muted, fontSize: 15, textAlign: "center" },
  closeHit: {
    padding: 2,
  },
  closeBtn: {
    backgroundColor: accountColors.accent,
    borderRadius: 12,
    paddingHorizontal: 20,
    paddingVertical: 12,
  },
  closeBtnText: { color: "#fff", fontWeight: "600", fontSize: 15 },
});
