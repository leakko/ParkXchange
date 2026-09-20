import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type Href, useLocalSearchParams, useRouter } from "expo-router";
import { ActivityIndicator, Alert, Pressable, ScrollView, Text, View } from "react-native";

import {
  cancelReservation,
  getMe,
  getReservation,
  getSpot,
  reservationEnRoute,
  reservationReady,
  reservationUnready,
} from "@/api/client";
import { accountStyles } from "@/account/theme";
import { useSession } from "@/hooks/useSession";
import { useTranslation, type TranslationKey } from "@/i18n";
import { formatPoints } from "@/i18n/formatPoints";
import {
  driverCancelOutcome,
  driverNoShowDeadline,
  ownerNoShowDeadline,
} from "@/map/exchangeLeave";

function reservationStatusKey(status: string): TranslationKey {
  return `account.reservations.status.${status}` as TranslationKey;
}

export default function ReservationDetailScreen() {
  const { t, formatDateTime } = useTranslation();
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
  const spot = useQuery({
    queryKey: ["spot", reservation.data?.spot_id],
    queryFn: () => getSpot(String(reservation.data!.spot_id)),
    enabled: !!reservation.data?.spot_id,
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
        case "en-route":
          await reservationEnRoute(id);
          return { completed: false };
        case "ready":
          return reservationReady(id);
        case "unready":
          await reservationUnready(id);
          return { completed: false };
        case "cancel":
          await cancelReservation(id);
          return { completed: false };
      }
    },
    onSuccess: async (data) => {
      await invalidate();
      if (data?.completed) {
        Alert.alert(t("exchange.completed.title"), t("exchange.completed.message"));
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
  const theirReady = isOwner ? res.driver_ready_at : res.owner_ready_at;
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
    <ScrollView style={accountStyles.screen} contentContainerStyle={accountStyles.scroll}>
      <Text style={accountStyles.title}>
        {t("account.reservations.rowTitle", {
          points: formatPoints(res.price_cents),
          status: t(reservationStatusKey(res.status)),
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
      {spot.data?.properties.vehicle ? (
        <Text style={accountStyles.meta}>
          {spot.data.properties.vehicle.plate} · {spot.data.properties.vehicle.make_model}
        </Text>
      ) : null}

      {live && (isOwner || isDriver) ? (
        <View style={{ gap: 10, marginTop: 16 }}>
          {theirReady ? (
            <Text style={accountStyles.meta}>
              {isOwner
                ? t("exchange.status.driverReady")
                : t("exchange.status.ownerReady")}
            </Text>
          ) : (
            <Text style={accountStyles.meta}>{t("exchange.status.waitingOther")}</Text>
          )}
          {myReady && deadline ? (
            <Text style={accountStyles.meta}>
              {t("exchange.status.deadline", {
                datetime: formatDateTime(deadline.toISOString()),
              })}
            </Text>
          ) : null}

          <Pressable
            style={[accountStyles.secondary, action.isPending && { opacity: 0.6 }]}
            disabled={action.isPending}
            onPress={confirmEnRoute}
          >
            <Text style={accountStyles.secondaryText}>{t("exchange.actions.enRoute")}</Text>
          </Pressable>

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
                ? t("spotSheet.exchange.cancelConfirm.message")
                : {
                    fair: t("spotSheet.exchange.cancelConfirm.driverFair"),
                    late: t("spotSheet.exchange.cancelConfirm.driverLate"),
                    stall: t("spotSheet.exchange.cancelConfirm.driverStall"),
                  }[driverCancelOutcome(res)];
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
