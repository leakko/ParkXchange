import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type Href, useLocalSearchParams, useRouter } from "expo-router";
import { ActivityIndicator, Alert, Pressable, ScrollView, Text, View } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";

import {
  cancelReservation,
  getMe,
  getReservation,
  reservationEnRoute,
  reservationReady,
  reservationUnready,
} from "@/api/client";
import { accountStyles } from "@/account/theme";
import { useSession } from "@/hooks/useSession";
import { useTranslation } from "@/i18n";
import { formatSignedPoints } from "@/i18n/formatPoints";
import { reservationStatusLabel } from "@/i18n/catalogLabels";
import { ExchangeStatusPanel } from "@/map/ExchangeStatusPanel";
import { PeerVehiclePanel } from "@/map/PeerVehiclePanel";
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
  const insets = useSafeAreaInsets();
  const { id } = useLocalSearchParams<{ id: string }>();
  const router = useRouter();
  const { signedIn } = useSession();
  const queryClient = useQueryClient();

  const me = useQuery({
    queryKey: ["me"],
    queryFn: getMe,
    enabled: signedIn,
  });
  const reservation = useQuery({
    queryKey: ["reservations", id],
    queryFn: () => getReservation(id),
    enabled: signedIn && !!id,
    refetchInterval: 5_000,
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
          await reservationEnRoute(id);
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
        Alert.alert(
          t("exchange.completed.title"),
          t(completedMessageKey(owner)),
        );
      }
    },
    onError: (err) => {
      Alert.alert(
        t("exchange.actionFailed.title"),
        err instanceof Error ? err.message : t("common.error"),
      );
    },
  });

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

  const confirmReady = () => {
    const message = isOwner
      ? t("exchange.confirm.ownerReady")
      : t("exchange.confirm.driverReady");
    Alert.alert(t("exchange.confirm.title"), message, [
      { text: t("common.cancel"), style: "cancel" },
      {
        text: t("common.confirm"),
        onPress: () => action.mutate("ready"),
      },
    ]);
  };

  const confirmUnready = () => {
    Alert.alert(t("exchange.confirm.title"), t("exchange.confirm.unready"), [
      { text: t("common.cancel"), style: "cancel" },
      {
        text: t("common.confirm"),
        onPress: () => action.mutate("unready"),
      },
    ]);
  };

  const confirmEnRoute = () => {
    Alert.alert(t("exchange.confirm.title"), t("exchange.confirm.enRoute"), [
      { text: t("common.cancel"), style: "cancel" },
      {
        text: t("common.confirm"),
        onPress: () => action.mutate("en-route"),
      },
    ]);
  };

  return (
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

      {live && (isOwner || isDriver) ? (
        <View style={{ gap: 10, marginTop: 16 }}>
          <PeerVehiclePanel
            vehicle={isOwner ? res.driver_vehicle : res.owner_vehicle}
            counterpart
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
              const message = isOwner
                ? t(ownerCancelMessageKey(res))
                : t(driverCancelMessageKey(res));
              Alert.alert(t("spotSheet.exchange.cancelConfirm.title"), message, [
                { text: t("common.cancel"), style: "cancel" },
                {
                  text: t("spotSheet.exchange.cancelConfirm.confirm"),
                  style: "destructive",
                  onPress: () => action.mutate("cancel"),
                },
              ]);
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
  );
}
