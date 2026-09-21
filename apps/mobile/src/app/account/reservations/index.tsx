import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type Href, useRouter } from "expo-router";
import { useState } from "react";
import {
  ActivityIndicator,
  Alert,
  Pressable,
  RefreshControl,
  Text,
  View,
} from "react-native";

import {
  createOffer,
  fetchActiveReservations,
  fetchReservations,
  getMe,
  listMyOffers,
  withdrawOffer,
  type OfferResponse,
  type ReservationResponse,
} from "@/api/client";
import { accountStyles } from "@/account/theme";
import { AuthScroll } from "@/auth/AuthScroll";
import { AuthTextInput } from "@/auth/AuthTextInput";
import { useSession } from "@/hooks/useSession";
import { useTranslation, type TranslationKey } from "@/i18n";
import { formatPoints, formatSignedPoints, parsePointsInput } from "@/i18n/formatPoints";
import { reservationPointsDelta } from "@/map/reservationPoints";
import { DateTimeField } from "@/ui/DateTimeField";

function reservationStatusKey(status: string): TranslationKey {
  const key = `account.reservations.status.${status}` as TranslationKey;
  return key;
}

function offerStatusKey(status: string): TranslationKey {
  const key = `account.reservations.offerStatus.${status}` as TranslationKey;
  return key;
}

function mergeReservations(
  list: ReservationResponse[] | undefined,
  active: ReservationResponse[] | undefined,
): ReservationResponse[] {
  const byId = new Map<string, ReservationResponse>();
  for (const r of list ?? []) {
    byId.set(r.id, r);
  }
  for (const r of active ?? []) {
    byId.set(r.id, r);
  }
  return [...byId.values()].sort((a, b) =>
    String(b.created_at).localeCompare(String(a.created_at)),
  );
}

export default function MyReservationsScreen() {
  const { t, formatDateTime } = useTranslation();
  const router = useRouter();
  const { signedIn } = useSession();
  const queryClient = useQueryClient();

  const me = useQuery({
    queryKey: ["me"],
    queryFn: getMe,
    enabled: signedIn,
  });
  const reservations = useQuery({
    queryKey: ["reservations"],
    queryFn: fetchReservations,
    enabled: signedIn,
    retry: false,
  });
  const active = useQuery({
    queryKey: ["reservations", "active"],
    queryFn: fetchActiveReservations,
    enabled: signedIn,
  });
  const offers = useQuery({
    queryKey: ["offers", "mine"],
    queryFn: listMyOffers,
    enabled: signedIn,
    retry: false,
  });

  const withdraw = useMutation({
    mutationFn: (id: string) => withdrawOffer(id),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["offers", "mine"] });
    },
    onError: (err) => {
      Alert.alert(
        t("account.reservations.withdrawOfferFailed"),
        err instanceof Error ? err.message : t("common.error"),
      );
    },
  });

  const [editingOfferId, setEditingOfferId] = useState<string | null>(null);
  const [editAmount, setEditAmount] = useState("");
  const [editExchangeAt, setEditExchangeAt] = useState(() => new Date());
  const [editBusy, setEditBusy] = useState(false);

  const beginEditOffer = (offer: OfferResponse) => {
    setEditingOfferId(offer.id);
    setEditAmount(formatPoints(offer.amount_cents));
    setEditExchangeAt(new Date(offer.exchange_at));
  };

  const saveEditedOffer = async (offer: OfferResponse) => {
    const points = parsePointsInput(editAmount);
    if (points == null || !Number.isFinite(editExchangeAt.getTime())) {
      Alert.alert(t("announce.alert.invalidPrice.title"), t("announce.alert.invalidPrice.message"));
      return;
    }
    setEditBusy(true);
    try {
      await withdrawOffer(offer.id);
      await createOffer(String(offer.spot_id), {
        vehicle_id: offer.vehicle_id,
        exchange_at: editExchangeAt.toISOString(),
        amount_cents: points,
      });
      setEditingOfferId(null);
      await queryClient.invalidateQueries({ queryKey: ["offers", "mine"] });
    } catch (err) {
      Alert.alert(
        t("account.reservations.editOfferFailed"),
        err instanceof Error ? err.message : t("common.error"),
      );
    } finally {
      setEditBusy(false);
    }
  };

  if (!signedIn || reservations.isLoading || offers.isLoading || me.isLoading || active.isLoading) {
    return (
      <View style={[accountStyles.screen, { justifyContent: "center", alignItems: "center" }]}>
        <ActivityIndicator color="#F4F7FA" />
      </View>
    );
  }

  const userId = me.data?.id ?? "";
  const pendingOffers = (offers.data ?? []).filter((o) => o.status === "pending");
  const list = mergeReservations(reservations.data, active.data);

  type Row =
    | { kind: "section"; title: string; id: string }
    | { kind: "reservation"; item: ReservationResponse }
    | { kind: "offer"; item: OfferResponse }
    | { kind: "empty"; message: string; id: string };

  const rows: Row[] = [
    { kind: "section", title: t("account.reservations.offersSection"), id: "offers-h" },
  ];
  if (pendingOffers.length === 0) {
    rows.push({
      kind: "empty",
      message: t("account.reservations.offersEmpty"),
      id: "offers-empty",
    });
  } else {
    for (const item of pendingOffers) {
      rows.push({ kind: "offer", item });
    }
  }
  rows.push({
    kind: "section",
    title: t("account.reservations.reservationsSection"),
    id: "res-h",
  });
  if (list.length === 0) {
    rows.push({
      kind: "empty",
      message: t("account.reservations.empty"),
      id: "res-empty",
    });
  } else {
    for (const item of list) {
      rows.push({ kind: "reservation", item });
    }
  }

  return (
    <AuthScroll
      refreshControl={
        <RefreshControl
          refreshing={
            reservations.isFetching || offers.isFetching || active.isFetching
          }
          onRefresh={() => {
            void reservations.refetch();
            void offers.refetch();
            void active.refetch();
          }}
          tintColor="#F4F7FA"
        />
      }
    >
      {rows.map((row) => {
        if (row.kind === "section") {
          return (
            <Text
              key={row.id}
              style={[accountStyles.sectionTitle, { marginTop: 8 }]}
            >
              {row.title}
            </Text>
          );
        }
        if (row.kind === "empty") {
          return (
            <Text key={row.id} style={accountStyles.empty}>
              {row.message}
            </Text>
          );
        }
        if (row.kind === "offer") {
          const offer = row.item;
          const editing = editingOfferId === offer.id;
          return (
            <View
              key={`o-${offer.id}`}
              style={[
                accountStyles.row,
                { marginBottom: 8, flexDirection: "column", alignItems: "stretch" },
              ]}
            >
              <Text style={accountStyles.rowTitle}>
                {t("account.reservations.offerTitle", {
                  points: formatSignedPoints(
                    offer.status === "pending"
                      ? -Math.abs(offer.amount_cents)
                      : 0,
                  ),
                  status: t(offerStatusKey(offer.status)),
                })}
              </Text>
              <Text style={accountStyles.rowMeta}>
                {t("account.reservations.exchangeAt", {
                  datetime: formatDateTime(offer.exchange_at),
                })}
              </Text>
              {editing ? (
                <View style={{ gap: 8, marginTop: 10 }}>
                  <Text style={accountStyles.label}>{t("announce.guidePrice")}</Text>
                  <AuthTextInput
                    value={editAmount}
                    onChangeText={setEditAmount}
                    keyboardType="number-pad"
                    placeholderTextColor="#7A93A0"
                  />
                  <DateTimeField value={editExchangeAt} onChange={setEditExchangeAt} />
                  <Pressable
                    style={accountStyles.primary}
                    disabled={editBusy}
                    onPress={() => void saveEditedOffer(offer)}
                  >
                    <Text style={accountStyles.primaryText}>
                      {t("account.reservations.editOffer.save")}
                    </Text>
                  </Pressable>
                  <Pressable onPress={() => setEditingOfferId(null)}>
                    <Text style={accountStyles.link}>{t("common.cancel")}</Text>
                  </Pressable>
                </View>
              ) : (
                <View style={{ flexDirection: "row", gap: 8, marginTop: 10 }}>
                  <Pressable
                    style={[accountStyles.secondary, { flex: 1 }]}
                    onPress={() => beginEditOffer(offer)}
                  >
                    <Text style={accountStyles.secondaryText}>
                      {t("account.reservations.editOffer")}
                    </Text>
                  </Pressable>
                  <Pressable
                    style={[accountStyles.danger, { flex: 1 }]}
                    disabled={withdraw.isPending}
                    onPress={() => withdraw.mutate(offer.id)}
                  >
                    <Text style={accountStyles.dangerText}>
                      {t("account.reservations.withdrawOffer")}
                    </Text>
                  </Pressable>
                </View>
              )}
            </View>
          );
        }

        const res = row.item;
        const isOwner = res.owner_id === userId;
        const live =
          res.status === "pending" ||
          res.status === "confirmed" ||
          res.status === "arrived";
        return (
          <Pressable
            key={`r-${res.id}`}
            style={[
              accountStyles.row,
              { marginBottom: 8, flexDirection: "column", alignItems: "stretch" },
            ]}
            onPress={() =>
              router.push(`/account/reservations/${res.id}` as Href)
            }
          >
            <Text style={accountStyles.rowTitle}>
              {t("account.reservations.rowTitle", {
                points: formatSignedPoints(
                  userId
                    ? reservationPointsDelta(res, userId)
                    : res.price_cents,
                ),
                status: t(reservationStatusKey(res.status)),
              })}
            </Text>
            <Text style={accountStyles.rowMeta}>
              {isOwner
                ? t("account.reservations.role.owner")
                : t("account.reservations.role.driver")}
              {live ? " · " : ""}
              {live ? t("account.reservations.openMap") : ""}
            </Text>
            <Text style={accountStyles.rowMeta}>
              {t("account.reservations.exchangeAt", {
                datetime: formatDateTime(res.exchange_at),
              })}
            </Text>
          </Pressable>
        );
      })}
      {reservations.error && active.error ? (
        <Text style={accountStyles.error}>
          {reservations.error instanceof Error
            ? reservations.error.message
            : t("account.reservations.loadFailed")}
        </Text>
      ) : null}
    </AuthScroll>
  );
}
