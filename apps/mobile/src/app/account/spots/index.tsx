import { fetchMySpots, listOffers, withdrawSpot } from "@/api/client";
import { notifySpotWithdrawn } from "@/map/spotWithdrawHandoff";
import { fetchActiveReservations, fetchReservations, type ReservationResponse } from "@/api/client";
import { StreetAddressMeta } from "@/account/StreetAddressMeta";
import { accountStyles } from "@/account/theme";
import { sortSpotsNewestFirst } from "@/account/listOrdering";
import { useSession } from "@/hooks/useSession";
import { useTranslation } from "@/i18n";
import { sizeClassLabel, spotStatusLabel } from "@/i18n/catalogLabels";
import { formatPoints, formatSignedPoints } from "@/i18n/formatPoints";
import { reservationPointsDelta } from "@/map/reservationPoints";
import { useQueries, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Ionicons } from "@expo/vector-icons";
import { type Href, useRouter } from "expo-router";
import { ActivityIndicator, FlatList, Pressable, Text, View } from "react-native";

import { useConfirm } from "@/ui/ConfirmModal";

function isLiveReservation(status: string): boolean {
  return status === "pending" || status === "confirmed" || status === "arrived";
}

function reservationForSpot(
  spotId: string,
  list: ReservationResponse[] | undefined,
): ReservationResponse | undefined {
  return (list ?? []).find((r) => String(r.spot_id) === spotId && isLiveReservation(r.status));
}

export default function MySpotsScreen() {
  const { t, formatDateTime } = useTranslation();
  const { confirm, alert } = useConfirm();
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
    pendingOfferCountBySpot.set(id, offers.filter((o) => o.status === "pending").length);
  });

  const orderedSpots = sortSpotsNewestFirst(
    (spots.data?.features ?? []).map((item) => ({
      item,
      fallback_at: item.properties.listed_until,
      preferred_departure_at: item.properties.preferred_departure_at,
    })),
  ).map(({ item }) => item);

  const withdraw = useMutation({
    mutationFn: (id: string) => withdrawSpot(id),
    onSuccess: async (_data, id) => {
      notifySpotWithdrawn(id);
      await queryClient.invalidateQueries({ queryKey: ["spots", "mine"] });
    },
    onError: async (err) => {
      await alert({
        title: t("account.spots.withdrawFailed.title"),
        message: err instanceof Error ? err.message : t("common.error"),
        confirmLabel: t("common.ok"),
      });
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
      reservationForSpot(spotId, active.data) ?? reservationForSpot(spotId, reservations.data);
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
        data={orderedSpots}
        keyExtractor={(item) => String(item.id)}
        ListEmptyComponent={<Text style={accountStyles.empty}>{t("account.spots.empty")}</Text>}
        renderItem={({ item }) => {
          const id = String(item.id);
          const canEdit = item.properties.status === "available";
          const reserved =
            item.properties.status === "reserved" || item.properties.status === "handover";
          const reservation =
            reservationForSpot(id, active.data) ?? reservationForSpot(id, reservations.data);
          const pendingOfferCount = pendingOfferCountBySpot.get(id) ?? 0;
          const needsAttention =
            item.properties.status === "available" ||
            item.properties.status === "reserved" ||
            item.properties.status === "handover";

          return (
            <View
              style={[accountStyles.rowCard, needsAttention ? accountStyles.rowAttention : null]}
            >
              <View
                style={{
                  flexDirection: "row",
                  alignItems: "flex-start",
                  justifyContent: "space-between",
                  gap: 8,
                }}
              >
                <View style={{ flex: 1 }}>
                  {reserved && reservation ? (
                    <>
                      <Text style={accountStyles.rowTitle}>
                        {t("account.spots.exchangeAt", {
                          datetime: formatDateTime(reservation.exchange_at),
                        })}
                      </Text>
                      <Text style={accountStyles.rowMetaTight}>
                        {t("account.spots.spotTitle", {
                          points: formatSignedPoints(
                            reservationPointsDelta(reservation, reservation.owner_id),
                          ),
                          status: spotStatusLabel(t, item.properties.status),
                        })}
                      </Text>
                    </>
                  ) : (
                    <Text style={accountStyles.rowTitle}>
                      {t("account.spots.spotTitle", {
                        points: formatPoints(item.properties.price_cents),
                        status: spotStatusLabel(t, item.properties.status),
                      })}
                    </Text>
                  )}
                </View>
                {item.geometry.coordinates[0] != null && item.geometry.coordinates[1] != null ? (
                  <Pressable
                    accessibilityRole="button"
                    accessibilityLabel={t("account.spots.showOnMap")}
                    onPress={() => {
                      const lon = item.geometry.coordinates[0]!;
                      const lat = item.geometry.coordinates[1]!;
                      const q = new URLSearchParams({
                        focusLon: String(lon),
                        focusLat: String(lat),
                        focusSpot: id,
                      });
                      if (!needsAttention) {
                        q.set("focusHistory", "1");
                      }
                      router.replace(`/?${q.toString()}` as Href);
                    }}
                    style={{
                      width: 36,
                      height: 36,
                      borderRadius: 18,
                      backgroundColor: "#16324F",
                      alignItems: "center",
                      justifyContent: "center",
                    }}
                  >
                    <Ionicons name="locate" size={20} color="#F4F7FA" />
                  </Pressable>
                ) : null}
              </View>
              <Text style={accountStyles.rowMetaTight}>
                {item.properties.vehicle
                  ? `${item.properties.vehicle.plate} · ${item.properties.vehicle.make_model}`
                  : sizeClassLabel(t, item.properties.size_class)}
              </Text>
              <StreetAddressMeta
                lon={item.geometry.coordinates[0]}
                lat={item.geometry.coordinates[1]}
                hint={item.properties.address_hint}
              />
              {canEdit ? (
                <Text style={accountStyles.rowMetaTight}>
                  {t("account.spots.listedUntil", {
                    datetime: formatDateTime(item.properties.listed_until),
                  })}
                </Text>
              ) : null}
              {reserved && !reservation ? (
                <Text style={accountStyles.rowMetaTight}>
                  {t("account.spots.reservedNoReservation")}
                </Text>
              ) : null}
              {canEdit || reserved ? (
              <View style={accountStyles.rowActions}>
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
                      <Text style={accountStyles.secondaryText}>{t("account.spots.edit")}</Text>
                    </Pressable>
                    <Pressable
                      style={[accountStyles.danger, { flexGrow: 1, minWidth: "45%" }]}
                      disabled={withdraw.isPending}
                      onPress={async () => {
                        const ok = await confirm({
                          title: t("account.spots.withdraw.confirmTitle"),
                          message: t("account.spots.withdraw.confirmMessage"),
                          cancelLabel: t("common.cancel"),
                          confirmLabel: t("account.spots.withdraw.action"),
                          destructive: true,
                        });
                        if (ok) {
                          withdraw.mutate(id);
                        }
                      }}
                    >
                      <Text style={accountStyles.dangerText}>
                        {t("account.spots.withdraw.action")}
                      </Text>
                    </Pressable>
                  </>
                ) : null}
                {reserved ? (
                  <>
                    <Pressable
                      style={[accountStyles.primary, { flex: 1 }]}
                      onPress={() => openExchange(id)}
                    >
                      <Text style={accountStyles.primaryText}>
                        {t("account.spots.openExchange")}
                      </Text>
                    </Pressable>
                    <Pressable
                      style={[accountStyles.danger, { flex: 1 }]}
                      disabled={withdraw.isPending}
                      onPress={async () => {
                        const ok = await confirm({
                          title: t("account.spots.withdraw.confirmTitle"),
                          message: t("account.spots.withdraw.confirmMessage"),
                          cancelLabel: t("common.cancel"),
                          confirmLabel: t("account.spots.withdraw.action"),
                          destructive: true,
                        });
                        if (ok) {
                          withdraw.mutate(id);
                        }
                      }}
                    >
                      <Text style={accountStyles.dangerText}>
                        {t("account.spots.withdraw.action")}
                      </Text>
                    </Pressable>
                  </>
                ) : null}
              </View>
              ) : null}
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
