import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type Href, useLocalSearchParams, useRouter } from "expo-router";
import { useEffect, useState } from "react";
import { ActivityIndicator, Pressable, ScrollView, Text, View } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";

import {
  cancelReservation,
  getMe,
  getReservation,
  getUserProfile,
  peerVehiclePhotoUrl,
  reservationEnRoute,
  reservationReady,
  reservationUnready,
  ApiError,
} from "@/api/client";
import { accountStyles } from "@/account/theme";
import { useSession } from "@/hooks/useSession";
import { useTranslation } from "@/i18n";
import { formatSignedPoints } from "@/i18n/formatPoints";
import { reservationStatusLabel } from "@/i18n/catalogLabels";
import { ExchangeStatusPanel } from "@/map/ExchangeStatusPanel";
import { PeerVehiclePanel } from "@/map/PeerVehiclePanel";
import { useConfirm } from "@/ui/ConfirmModal";
import {
  RateExchangeModal,
  isRatingDismissed,
} from "@/ui/RateExchangeModal";
import {
  completedMessageKey,
  driverCancelMessageKey,
  ownerCancelMessageKey,
} from "@/map/exchangeCopy";
import {
  driverNoShowDeadline,
  ownerNoShowDeadline,
  shouldShowNoShowDeadline,
} from "@/map/exchangeLeave";
import { reservationPointsDelta } from "@/map/reservationPoints";
import { armGeofenceForReservation, disarmArrivalGeofence, clearArrivalPromptFired } from "@/push/geofence";

export default function ReservationDetailScreen() {
  const { t, formatDateTime } = useTranslation();
  const { confirm, alert } = useConfirm();
  const insets = useSafeAreaInsets();
  const { id } = useLocalSearchParams<{ id: string }>();
  const router = useRouter();
  const { signedIn } = useSession();
  const queryClient = useQueryClient();
  const [rateOpen, setRateOpen] = useState(false);

  const me = useQuery({
    queryKey: ["me"],
    queryFn: getMe,
    enabled: signedIn,
  });
  const reservation = useQuery({
    queryKey: ["reservations", id],
    queryFn: () => getReservation(id),
    enabled: signedIn && !!id,
    // Live exchanges need polling; a hard miss (404 / forbidden-as-missing)
    // must not keep hammering the API every 5s.
    refetchInterval: (q) =>
      q.state.error || q.state.status === "error" ? false : 5_000,
    retry: (count, err) => {
      if (err instanceof ApiError && (err.status === 404 || err.status === 403)) {
        return false;
      }
      return count < 2;
    },
  });

  const peerId =
    reservation.data && me.data
      ? reservation.data.owner_id === me.data.id
        ? reservation.data.driver_id
        : reservation.data.owner_id
      : null;
  const peerProfile = useQuery({
    queryKey: ["user-profile", peerId],
    queryFn: () => getUserProfile(String(peerId)),
    enabled: signedIn && !!peerId,
  });

  const invalidate = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ["reservations"] }),
      queryClient.invalidateQueries({ queryKey: ["reservations", id] }),
      queryClient.invalidateQueries({ queryKey: ["spots", "mine"] }),
    ]);
  };

  const action = useMutation({
    mutationFn: async (kind: "en-route" | "ready" | "unready" | "cancel") => {
      if (!id) {
        throw new Error(t("account.reservations.notFound"));
      }
      switch (kind) {
        case "en-route": {
          const { currentLatLon } = await import("@/push/locationSeed");
          const here = await currentLatLon();
          await reservationEnRoute(
            id,
            here ? { latitude: here.latitude, longitude: here.longitude } : null,
          );
          await armGeofenceForReservation(id);
          return { completed: false };
        }
        case "ready": {
          const result = await reservationReady(id);
          await disarmArrivalGeofence();
          return result;
        }
        case "unready":
          await reservationUnready(id);
          return { completed: false };
        case "cancel":
          await cancelReservation(id);
          await disarmArrivalGeofence();
          await clearArrivalPromptFired(id);
          return { completed: false };
      }
    },
    onSuccess: async (data) => {
      await invalidate();
      if (data?.completed && reservation.data && me.data) {
        const owner = reservation.data.owner_id === me.data.id;
        await alert({
          title: t("exchange.completed.title"),
          message: t(completedMessageKey(owner)),
          confirmLabel: t("common.ok"),
        });
        const fresh = await queryClient.fetchQuery({
          queryKey: ["reservations", id],
          queryFn: () => getReservation(id!),
        });
        if (fresh.can_rate && !(await isRatingDismissed(fresh.id))) {
          setTimeout(() => setRateOpen(true), 10_000);
        }
      }
    },
    onError: async (err) => {
      await alert({
        title: t("exchange.actionFailed.title"),
        message: err instanceof Error ? err.message : t("common.error"),
        confirmLabel: t("common.ok"),
      });
    },
  });

  useEffect(() => {
    const current = reservation.data;
    if (!current || current.status !== "completed" || !current.can_rate) {
      if (current && !current.can_rate) {
        setRateOpen(false);
      }
      return;
    }
    let cancelled = false;
    void isRatingDismissed(current.id).then((dismissed) => {
      if (!cancelled && !dismissed) {
        setRateOpen(true);
      }
    });
    return () => {
      cancelled = true;
    };
  }, [reservation.data?.id, reservation.data?.status, reservation.data?.can_rate]);

  if (!signedIn || reservation.isLoading || me.isLoading) {
    return (
      <View style={[accountStyles.screen, { justifyContent: "center", alignItems: "center" }]}>
        <ActivityIndicator color="#F4F7FA" />
      </View>
    );
  }

  if (reservation.error || !reservation.data || !me.data) {
    return (
      <View style={[accountStyles.screen, accountStyles.scroll]}>
        <Text style={accountStyles.error}>
          {reservation.error instanceof Error
            ? reservation.error.message
            : t("account.reservations.notFound")}
        </Text>
        <Pressable
          style={[accountStyles.primary, { marginTop: 16 }]}
          onPress={() => router.replace("/" as Href)}
        >
          <Text style={accountStyles.primaryText}>{t("account.reservations.openMap")}</Text>
        </Pressable>
      </View>
    );
  }

  const res = reservation.data;
  const userId = me.data.id;
  const isOwner = res.owner_id === userId;
  const isDriver = res.driver_id === userId;
  const live =
    res.status === "pending" || res.status === "confirmed" || res.status === "arrived";
  const myReady = isOwner ? res.owner_ready_at : res.driver_ready_at;
  const myEnRoute = isOwner ? res.owner_en_route_at : res.driver_en_route_at;
  const deadline = isOwner ? driverNoShowDeadline(res) : ownerNoShowDeadline(res);
  const peerName =
    peerProfile.data?.display_name?.trim() ||
    t("account.reservations.peerFallback");

  const confirmReady = () => {
    void (async () => {
      const message = isOwner
        ? t("exchange.confirm.ownerReady")
        : t("exchange.confirm.driverReady");
      const ok = await confirm({
        title: t("exchange.confirm.title"),
        message,
        cancelLabel: t("common.cancel"),
        confirmLabel: t("common.confirm"),
      });
      if (ok) action.mutate("ready");
    })();
  };

  const confirmUnready = () => {
    void (async () => {
      const ok = await confirm({
        title: t("exchange.confirm.title"),
        message: t("exchange.confirm.unready"),
        cancelLabel: t("common.cancel"),
        confirmLabel: t("common.confirm"),
      });
      if (ok) action.mutate("unready");
    })();
  };

  const confirmEnRoute = () => {
    void (async () => {
      const ok = await confirm({
        title: t("exchange.confirm.title"),
        message: t("exchange.confirm.enRoute"),
        cancelLabel: t("common.cancel"),
        confirmLabel: t("common.confirm"),
      });
      if (ok) action.mutate("en-route");
    })();
  };

  return (
    <>
    <ScrollView
      style={accountStyles.screen}
      contentContainerStyle={[
        accountStyles.scroll,
        { paddingBottom: 40 + insets.bottom },
      ]}
    >
      <Text style={accountStyles.title}>
        {t("account.reservations.rowTitle", {
          points: formatSignedPoints(reservationPointsDelta(res, userId)),
          status: reservationStatusLabel(t, res.status),
        })}
      </Text>
      <Text style={accountStyles.meta}>
        {isOwner
          ? t("account.reservations.role.owner")
          : t("account.reservations.role.driver")}
      </Text>
      <Text style={accountStyles.meta}>
        {t("account.reservations.exchangeAt", {
          datetime: formatDateTime(res.exchange_at),
        })}
      </Text>

      <View style={{ marginTop: 12, gap: 4 }}>
        <Text style={accountStyles.rowTitle}>{peerName}</Text>
        {peerId ? (
          <Pressable
            onPress={() => router.push(`/user/${peerId}` as Href)}
            accessibilityRole="link"
          >
            <Text style={[accountStyles.link, { textDecorationLine: "underline" }]}>
              {t("profile.public.view")}
            </Text>
          </Pressable>
        ) : null}
      </View>

      {res.status === "completed" && res.can_rate ? (
        <Pressable
          style={[accountStyles.primary, { marginTop: 12 }]}
          onPress={() => setRateOpen(true)}
        >
          <Text style={accountStyles.primaryText}>{t("rating.cta")}</Text>
        </Pressable>
      ) : null}

      {live && (isOwner || isDriver) ? (
        <View style={{ gap: 10, marginTop: 16 }}>
          <PeerVehiclePanel
            vehicle={isOwner ? res.driver_vehicle : res.owner_vehicle}
            counterpart
            photoUrl={peerVehiclePhotoUrl(String(res.id))}
          />
          <ExchangeStatusPanel
            res={res}
            iAmOwner={isOwner}
            deadlineLabel={
              myReady && deadline && shouldShowNoShowDeadline(res)
                ? t("exchange.status.deadline", {
                    datetime: formatDateTime(deadline.toISOString()),
                  })
                : null
            }
          />

          {!myReady && !myEnRoute ? (
            <Pressable
              style={[accountStyles.secondary, action.isPending && { opacity: 0.6 }]}
              disabled={action.isPending}
              onPress={confirmEnRoute}
            >
              <Text style={accountStyles.secondaryText}>{t("exchange.actions.enRoute")}</Text>
            </Pressable>
          ) : null}

          {!myReady ? (
            <Pressable
              style={[accountStyles.primary, action.isPending && { opacity: 0.6 }]}
              disabled={action.isPending}
              onPress={confirmReady}
            >
              {action.isPending ? (
                <ActivityIndicator color="#fff" />
              ) : (
                <Text style={accountStyles.primaryText}>
                  {isOwner
                    ? t("exchange.actions.ownerReady")
                    : t("exchange.actions.driverReady")}
                </Text>
              )}
            </Pressable>
          ) : (
            <Pressable
              style={[accountStyles.secondary, action.isPending && { opacity: 0.6 }]}
              disabled={action.isPending}
              onPress={confirmUnready}
            >
              <Text style={accountStyles.secondaryText}>{t("exchange.actions.unready")}</Text>
            </Pressable>
          )}

          <Pressable
            style={accountStyles.danger}
            disabled={action.isPending}
            onPress={() => {
              void (async () => {
                const message = isOwner
                  ? t(ownerCancelMessageKey(res))
                  : t(driverCancelMessageKey(res));
                const ok = await confirm({
                  title: t("spotSheet.exchange.cancelConfirm.title"),
                  message,
                  cancelLabel: t("common.cancel"),
                  confirmLabel: t("spotSheet.exchange.cancelConfirm.confirm"),
                  destructive: true,
                });
                if (ok) action.mutate("cancel");
              })();
            }}
          >
            <Text style={accountStyles.dangerText}>{t("spotSheet.exchange.cancel")}</Text>
          </Pressable>
          <Pressable
            style={accountStyles.secondary}
            onPress={() => router.replace("/" as Href)}
          >
            <Text style={accountStyles.secondaryText}>
              {t("account.reservations.openMap")}
            </Text>
          </Pressable>
        </View>
      ) : null}

    </ScrollView>
    <RateExchangeModal
      reservationId={res.id}
      visible={rateOpen}
      onClose={() => setRateOpen(false)}
      onSubmitted={() => {
        setRateOpen(false);
        void invalidate();
      }}
    />
    </>
  );
}
