import { fetchMySpots, listOffers, withdrawSpot } from "@/api/client";
import {
  fetchActiveReservations,
  fetchReservations,
  type ReservationResponse,
} from "@/api/client";
import { accountStyles } from "@/account/theme";
import { useSession } from "@/hooks/useSession";
import { useTranslation } from "@/i18n";
import { formatPoints } from "@/i18n/formatPoints";
import { useQueries, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type Href, useRouter } from "expo-router";
import {
  ActivityIndicator,
  Alert,
  FlatList,
  Pressable,
  Text,
  View,
} from "react-native";

function isLiveReservation(status: string): boolean {
  return status === "pending" || status === "confirmed" || status === "arrived";
}

function reservationForSpot(
  spotId: string,
  list: ReservationResponse[] | undefined,
): ReservationResponse | undefined {
  return (list ?? []).find(
    (r) => String(r.spot_id) === spotId && isLiveReservation(r.status),
  );
}

export default function MySpotsScreen() {
  const { t, formatDateTime } = useTranslation();
  const router = useRouter();
  const { signedIn } = useSession();
  const queryClient = useQueryClient();
  const spots = useQuery({
    queryKey: ["spots", "mine"],
    queryFn: fetchMySpots,
    enabled: signedIn,
  });
  const reservations = useQuery({
    queryKey: ["reservations"],
    queryFn: fetchReservations,
    enabled: signedIn,
  });
  const active = useQuery({
    queryKey: ["reservations", "active"],
    queryFn: fetchActiveReservations,
    enabled: signedIn,
  });

  const availableIds = (spots.data?.features ?? [])
    .filter((f) => f.properties.status === "available")
    .map((f) => String(f.id));
  const offerQueries = useQueries({
    queries: availableIds.map((id) => ({
      queryKey: ["offers", id] as const,
      queryFn: () => listOffers(id),
      enabled: signedIn && availableIds.length > 0,
    })),
  });
  const pendingOfferCountBySpot = new Map<string, number>();
  availableIds.forEach((id, index) => {
    const offers = offerQueries[index]?.data ?? [];
    pendingOfferCountBySpot.set(
      id,
      offers.filter((o) => o.status === "pending").length,
    );
  });

  const withdraw = useMutation({
    mutationFn: (id: string) => withdrawSpot(id),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["spots", "mine"] });
    },
    onError: (err) => {
      Alert.alert(
        t("account.spots.withdrawFailed.title"),
        err instanceof Error ? err.message : t("common.error"),
      );
    },
  });

  if (!signedIn || spots.isLoading) {
    return (
      <View style={[accountStyles.screen, { justifyContent: "center", alignItems: "center" }]}>
        <ActivityIndicator color="#F4F7FA" />
      </View>
    );
  }

  const openExchange = (spotId: string) => {
    const res =
      reservationForSpot(spotId, active.data) ??
      reservationForSpot(spotId, reservations.data);
    if (res) {
      router.push(`/account/reservations/${res.id}` as Href);
      return;
    }
    // Active query may be stale right after accept — refetch then navigate.
    void (async () => {
      try {
        const [againActive, againList] = await Promise.all([
          active.refetch(),
          reservations.refetch(),
        ]);
        const found =
          reservationForSpot(spotId, againActive.data) ??
          reservationForSpot(spotId, againList.data);
        if (found) {
          router.push(`/account/reservations/${found.id}` as Href);
          return;
        }
      } catch {
        /* fall through */
      }
      router.push("/account/reservations" as Href);
    })();
  };

  return (
    <View style={accountStyles.screen}>
      <FlatList
        contentContainerStyle={accountStyles.scroll}
        data={spots.data?.features ?? []}
        keyExtractor={(item) => String(item.id)}
        ListEmptyComponent={
          <Text style={accountStyles.empty}>{t("account.spots.empty")}</Text>
        }
        renderItem={({ item }) => {
          const id = String(item.id);
          const canEdit = item.properties.status === "available";
          const reserved =
            item.properties.status === "reserved" ||
            item.properties.status === "handover";
          const reservation =
            reservationForSpot(id, active.data) ??
            reservationForSpot(id, reservations.data);
          const pendingOfferCount = pendingOfferCountBySpot.get(id) ?? 0;

          return (
            <View
              style={[
                accountStyles.row,
                { marginBottom: 8, flexDirection: "column", alignItems: "stretch" },
              ]}
            >
              {reserved && reservation ? (
                <>
                  <Text style={accountStyles.title}>
                    {t("account.spots.exchangeAt", {
                      datetime: formatDateTime(reservation.exchange_at),
                    })}
                  </Text>
                  <Text style={accountStyles.rowMeta}>
                    {t("account.spots.spotTitle", {
                      points: formatPoints(reservation.price_cents),
                      status: item.properties.status,
                    })}
                  </Text>
                </>
              ) : (
                <Text style={accountStyles.rowTitle}>
                  {t("account.spots.spotTitle", {
                    points: formatPoints(item.properties.price_cents),
                    status: item.properties.status,
                  })}
                </Text>
              )}
              <Text style={accountStyles.rowMeta}>
                {item.properties.vehicle
                  ? `${item.properties.vehicle.plate} · ${item.properties.vehicle.make_model}`
                  : item.properties.size_class}
              </Text>
              {canEdit ? (
                <Text style={accountStyles.rowMeta}>
                  {t("account.spots.listedUntil", {
                    datetime: formatDateTime(item.properties.listed_until),
                  })}
                </Text>
              ) : null}
              {reserved && !reservation ? (
                <Text style={accountStyles.rowMeta}>
                  {t("account.spots.reservedNoReservation")}
                </Text>
              ) : null}
              <View style={{ flexDirection: "row", flexWrap: "wrap", gap: 8, marginTop: 10 }}>
                {canEdit ? (
                  <>
                    <Pressable
                      style={[accountStyles.primary, { flexGrow: 1, minWidth: "45%" }]}
                      onPress={() => router.push(`/account/spots/${id}` as Href)}
                    >
                      <Text style={accountStyles.primaryText}>
                        {pendingOfferCount > 0
                          ? t("spotSheet.viewOffers", { count: pendingOfferCount })
                          : t("spotSheet.viewOffersEmpty")}
                      </Text>
                    </Pressable>
                    <Pressable
                      style={[accountStyles.secondary, { flexGrow: 1, minWidth: "45%" }]}
                      onPress={() => router.push(`/account/spots/${id}` as Href)}
                    >
                      <Text style={accountStyles.secondaryText}>
                        {t("account.spots.edit")}
                      </Text>
                    </Pressable>
                    <Pressable
                      style={[accountStyles.danger, { flexGrow: 1, minWidth: "45%" }]}
                      disabled={withdraw.isPending}
                      onPress={() => {
                        Alert.alert(
                          t("account.spots.withdraw.confirmTitle"),
                          t("account.spots.withdraw.confirmMessage"),
                          [
                            { text: t("common.cancel"), style: "cancel" },
                            {
                              text: t("account.spots.withdraw.action"),
                              style: "destructive",
                              onPress: () => withdraw.mutate(id),
                            },
                          ],
                        );
                      }}
                    >
                      <Text style={accountStyles.dangerText}>
                        {t("account.spots.withdraw.action")}
                      </Text>
                    </Pressable>
                  </>
                ) : null}
                {reserved ? (
                  <Pressable
                    style={[accountStyles.primary, { flex: 1 }]}
                    onPress={() => openExchange(id)}
                  >
                    <Text style={accountStyles.primaryText}>
                      {t("account.spots.openExchange")}
                    </Text>
                  </Pressable>
                ) : null}
              </View>
            </View>
          );
        }}
        refreshing={
          spots.isFetching ||
          reservations.isFetching ||
          active.isFetching ||
          offerQueries.some((q) => q.isFetching)
        }
        onRefresh={() => {
          void spots.refetch();
          void reservations.refetch();
          void active.refetch();
          for (const q of offerQueries) {
            void q.refetch();
          }
        }}
      />
      {spots.error ? (
        <Text style={[accountStyles.error, { padding: 20 }]}>
          {spots.error instanceof Error ? spots.error.message : t("account.spots.loadFailed")}
        </Text>
      ) : null}
    </View>
  );
}
