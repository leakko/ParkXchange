import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type Href, useLocalSearchParams, useRouter } from "expo-router";
import { ActivityIndicator, Alert, Pressable, ScrollView, Text, View } from "react-native";

import {
  cancelReservation,
  clearDriverArrived,
  driverArrived,
  driverConfirmEntered,
  driverReady,
  driverReportOwnerNoShow,
  getMe,
  getReservation,
  getSpot,
  ownerReady,
} from "@/api/client";
import { accountStyles } from "@/account/theme";
import { useSession } from "@/hooks/useSession";
import { useTranslation, type TranslationKey } from "@/i18n";
import { formatPoints } from "@/i18n/formatPoints";
import {
  driverCanResolveStalledOwner,
  driverCancelOutcome,
  ownerCanLeave,
  ownerLeaveDeadline,
  ownerLeaveWithoutReadyAt,
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
    mutationFn: async (
      kind:
        | "owner-ready"
        | "driver-arrived"
        | "clear-driver-arrived"
        | "driver-ready"
        | "confirm-entered"
        | "report-no-show"
        | "cancel",
    ) => {
      if (!id) {
        throw new Error(t("account.reservations.notFound"));
      }
      switch (kind) {
        case "owner-ready":
          await ownerReady(id);
          break;
        case "driver-arrived":
          await driverArrived(id);
          break;
        case "clear-driver-arrived":
          await clearDriverArrived(id);
          break;
        case "driver-ready":
          await driverReady(id);
          break;
        case "confirm-entered":
          await driverConfirmEntered(id);
          break;
        case "report-no-show":
          await driverReportOwnerNoShow(id);
          break;
        case "cancel":
          await cancelReservation(id);
          break;
      }
    },
    onSuccess: async (_data, kind) => {
      await invalidate();
      if (kind === "owner-ready" || kind === "confirm-entered") {
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
  const canLeave = ownerCanLeave(res);
  const canResolveStall = driverCanResolveStalledOwner(res);

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

      {live ? (
        <View style={{ gap: 10, marginTop: 16 }}>
          {isOwner && res.driver_arrived_at && !res.driver_ready_at ? (
            <Text style={accountStyles.meta}>{t("spotSheet.exchange.driverIsHere")}</Text>
          ) : null}
          {isOwner && !res.driver_arrived_at && !res.driver_ready_at ? (
            <Text style={accountStyles.meta}>{t("spotSheet.exchange.waitingDriver")}</Text>
          ) : null}
          {isOwner && res.driver_ready_at ? (
            <Text style={accountStyles.meta}>
              {t("spotSheet.exchange.ownerLeavingSoon", {
                datetime: formatDateTime(
                  (ownerLeaveDeadline(res) ?? new Date()).toISOString(),
                ),
              })}
            </Text>
          ) : null}
          {isOwner ? (
            <Pressable
              style={[accountStyles.primary, !canLeave && { opacity: 0.45 }]}
              disabled={action.isPending}
              onPress={() => {
                if (canLeave) {
                  action.mutate("owner-ready");
                  return;
                }
                Alert.alert(
                  t("spotSheet.exchange.ownerLeaveBlocked.title"),
                  t("spotSheet.exchange.ownerLeaveBlocked.waitingDriver", {
                    datetime: formatDateTime(ownerLeaveWithoutReadyAt(res).toISOString()),
                  }),
                );
              }}
            >
              <Text style={accountStyles.primaryText}>
                {t("spotSheet.exchange.ownerReady")}
              </Text>
            </Pressable>
          ) : null}
          {isDriver && !res.driver_arrived_at && !res.driver_ready_at ? (
            <Pressable
              style={[accountStyles.primary, action.isPending && { opacity: 0.6 }]}
              disabled={action.isPending}
              onPress={() => action.mutate("driver-arrived")}
            >
              {action.isPending ? (
                <ActivityIndicator color="#fff" />
              ) : (
                <Text style={accountStyles.primaryText}>
                  {t("spotSheet.exchange.driverArrived")}
                </Text>
              )}
            </Pressable>
          ) : null}
          {isDriver &&
          (!!res.driver_arrived_at || !!res.driver_ready_at) &&
          !canResolveStall ? (
            <>
              <Text style={accountStyles.meta}>
                {t("spotSheet.exchange.driverArrivedOn")}
              </Text>
              <Pressable
                style={[accountStyles.secondary, action.isPending && { opacity: 0.6 }]}
                disabled={action.isPending}
                onPress={() => action.mutate("clear-driver-arrived")}
              >
                {action.isPending ? (
                  <ActivityIndicator color="#F4F7FA" />
                ) : (
                  <Text style={accountStyles.secondaryText}>
                    {t("spotSheet.exchange.driverArrivedClear")}
                  </Text>
                )}
              </Pressable>
            </>
          ) : null}
          {isDriver && canResolveStall ? (
            <>
              <Text style={accountStyles.meta}>{t("spotSheet.exchange.stallHelp")}</Text>
              <Pressable
                style={accountStyles.primary}
                disabled={action.isPending}
                onPress={() => {
                  Alert.alert(
                    t("exchange.stall.confirmTitle"),
                    t("exchange.stall.confirmMessage"),
                    [
                      { text: t("common.cancel"), style: "cancel" },
                      {
                        text: t("common.confirm"),
                        onPress: () => action.mutate("confirm-entered"),
                      },
                    ],
                  );
                }}
              >
                <Text style={accountStyles.primaryText}>
                  {t("spotSheet.exchange.stallConfirmEntered")}
                </Text>
              </Pressable>
              <Pressable
                style={accountStyles.danger}
                disabled={action.isPending}
                onPress={() => {
                  Alert.alert(
                    t("exchange.stall.reportTitle"),
                    t("exchange.stall.reportMessage"),
                    [
                      { text: t("common.cancel"), style: "cancel" },
                      {
                        text: t("spotSheet.exchange.stallReportNoShow"),
                        style: "destructive",
                        onPress: () => action.mutate("report-no-show"),
                      },
                    ],
                  );
                }}
              >
                <Text style={accountStyles.dangerText}>
                  {t("spotSheet.exchange.stallReportNoShow")}
                </Text>
              </Pressable>
            </>
          ) : null}
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
              Alert.alert(
                t("spotSheet.exchange.cancelConfirm.title"),
                message,
                [
                  { text: t("common.cancel"), style: "cancel" },
                  {
                    text: t("spotSheet.exchange.cancelConfirm.confirm"),
                    style: "destructive",
                    onPress: () => action.mutate("cancel"),
                  },
                ],
              );
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
