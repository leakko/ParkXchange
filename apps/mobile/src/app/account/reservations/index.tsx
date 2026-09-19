import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type Href, useRouter } from "expo-router";
import { useState } from "react";
import {
  ActivityIndicator,
  Alert,
  FlatList,
  Pressable,
  Text,
  TextInput,
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
import { useSession } from "@/hooks/useSession";
import { useTranslation, type TranslationKey } from "@/i18n";
import { formatPoints, parsePointsInput } from "@/i18n/formatPoints";
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
    <View style={accountStyles.screen}>
      <FlatList
        contentContainerStyle={accountStyles.scroll}
        data={rows}
        keyExtractor={(row) => {
          if (row.kind === "reservation") return `r-${row.item.id}`;
          if (row.kind === "offer") return `o-${row.item.id}`;
          return row.id;
        }}
        renderItem={({ item: row }) => {
          if (row.kind === "section") {
            return (
              <Text style={[accountStyles.sectionTitle, { marginTop: 8 }]}>
                {row.title}
              </Text>
            );
          }
          if (row.kind === "empty") {
            return <Text style={accountStyles.empty}>{row.message}</Text>;
          }
          if (row.kind === "offer") {
            const offer = row.item;
            const editing = editingOfferId === offer.id;
            return (
              <View
                style={[
                  accountStyles.row,
                  { marginBottom: 8, flexDirection: "column", alignItems: "stretch" },
                ]}
              >
                <Text style={accountStyles.rowTitle}>
                  {t("account.reservations.offerTitle", {
                    points: formatPoints(offer.amount_cents),
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
                    <TextInput
                      style={accountStyles.input}
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
                  points: formatPoints(res.price_cents),
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
        }}
        refreshing={
          reservations.isFetching || offers.isFetching || active.isFetching
        }
        onRefresh={() => {
          void reservations.refetch();
          void offers.refetch();
          void active.refetch();
        }}
      />
      {reservations.error && active.error ? (
        <Text style={[accountStyles.error, { padding: 20 }]}>
          {reservations.error instanceof Error
            ? reservations.error.message
            : t("account.reservations.loadFailed")}
        </Text>
      ) : null}
    </View>
  );
}
