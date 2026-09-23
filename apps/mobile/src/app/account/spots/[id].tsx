import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type Href, useLocalSearchParams, useRouter } from "expo-router";
import { useEffect, useState } from "react";
import {
  ActivityIndicator,
  Pressable,
  Switch,
  Text,
  View,
} from "react-native";

import {
  acceptOffer,
  fetchMySpots,
  listOffers,
  listVehicles,
  rejectOffer,
  updateSpot,
  withdrawSpot,
} from "@/api/client";
import { apiErrorMessage, apiErrorTitle } from "@/api/errors";
import { notifySpotWithdrawn } from "@/map/spotWithdrawHandoff";
import { accountStyles } from "@/account/theme";
import { AuthScroll } from "@/auth/AuthScroll";
import { AuthTextInput } from "@/auth/AuthTextInput";
import { ensureEmailVerified } from "@/auth/requireEmailVerified";
import { useSession } from "@/hooks/useSession";
import { useTranslation } from "@/i18n";
import { spotStatusLabel, firstGivenName } from "@/i18n/catalogLabels";
import { formatPoints, parsePointsInput } from "@/i18n/formatPoints";
import { matchesPreferredMinute } from "@/map/exchange";
import { useConfirm } from "@/ui/ConfirmModal";
import { DateTimeField } from "@/ui/DateTimeField";

function defaultPreferred(): Date {
  return new Date(Date.now() + 60 * 60 * 1000);
}

export default function EditSpotScreen() {
  const { t, formatDateTime } = useTranslation();
  const { confirm, alert } = useConfirm();
  const { id } = useLocalSearchParams<{ id: string }>();
  const router = useRouter();
  const { signedIn } = useSession();
  const queryClient = useQueryClient();

  const spots = useQuery({
    queryKey: ["spots", "mine"],
    queryFn: fetchMySpots,
    enabled: signedIn,
  });
  const vehicles = useQuery({
    queryKey: ["vehicles"],
    queryFn: listVehicles,
    enabled: signedIn,
  });
  const offers = useQuery({
    queryKey: ["offers", id],
    queryFn: () => listOffers(id),
    enabled: signedIn && !!id,
  });

  const spot = spots.data?.features.find((f) => String(f.id) === id);

  const [price, setPrice] = useState("");
  const [notes, setNotes] = useState("");
  const [vehicleId, setVehicleId] = useState("");
  const [hasPreferredTime, setHasPreferredTime] = useState(false);
  const [preferredTime, setPreferredTime] = useState(defaultPreferred);
  const [autoCancel, setAutoCancel] = useState(true);

  useEffect(() => {
    if (!spot) {
      return;
    }
    setPrice(formatPoints(spot.properties.price_cents));
    setNotes(spot.properties.notes ?? "");
    setVehicleId(spot.properties.vehicle?.id ?? "");
    setHasPreferredTime(!!spot.properties.preferred_departure_at);
    setPreferredTime(
      spot.properties.preferred_departure_at
        ? new Date(spot.properties.preferred_departure_at)
        : defaultPreferred(),
    );
    setAutoCancel(spot.properties.auto_cancel_no_show);
  }, [spot]);

  const save = useMutation({
    mutationFn: async () => {
      if (!id) {
        throw new Error(t("account.spots.edit.missingId"));
      }
      const points = parsePointsInput(price);
      if (points == null) {
        throw new Error(t("account.spots.edit.invalidPrice"));
      }
      const body: Parameters<typeof updateSpot>[1] = {
        price_cents: points,
        auto_cancel_no_show: autoCancel,
      };
      if (hasPreferredTime) {
        if (!Number.isFinite(preferredTime.getTime())) {
          throw new Error(t("account.spots.edit.invalidPreferredTime"));
        }
        body.preferred_departure_at = preferredTime.toISOString();
      } else {
        body.preferred_departure_at = null;
      }
      const trimmedNotes = notes.trim();
      if (trimmedNotes) {
        body.notes = trimmedNotes;
      }
      if (vehicleId) {
        body.vehicle_id = vehicleId;
      }
      return updateSpot(id, body);
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["spots", "mine"] });
      await alert({
        title: t("account.spots.edit.saved.title"),
        message: t("account.spots.edit.saved.message"),
        confirmLabel: t("common.ok"),
      });
      router.back();
    },
    onError: async (err) => {
      await alert({
        title: t("account.spots.edit.saveFailed.title"),
        message: err instanceof Error ? err.message : t("common.error"),
        confirmLabel: t("common.ok"),
      });
    },
  });

  const decideOffer = useMutation({
    mutationFn: async ({ offerId, accept }: { offerId: string; accept: boolean }) => {
      if (accept) {
        await acceptOffer(offerId);
      } else {
        await rejectOffer(offerId);
      }
    },
    onSuccess: async (_, variables) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["offers", id] }),
        queryClient.invalidateQueries({ queryKey: ["spots", "mine"] }),
      ]);
      if (variables.accept) {
        await alert({
          title: t("account.spots.offer.accepted.title"),
          message: t("account.spots.offer.accepted.message"),
          confirmLabel: t("common.ok"),
        });
      }
    },
    onError: async (err) => {
      await alert({
        title: apiErrorTitle(err, t, "account.spots.offer.updateFailed.title"),
        message: apiErrorMessage(err, t),
        confirmLabel: t("common.ok"),
      });
    },
  });

  const withdraw = useMutation({
    mutationFn: async () => {
      if (!id) {
        throw new Error(t("account.spots.edit.missingId"));
      }
      await withdrawSpot(id);
    },
    onSuccess: async () => {
      if (id) {
        notifySpotWithdrawn(id);
      }
      await queryClient.invalidateQueries({ queryKey: ["spots", "mine"] });
      router.back();
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

  if (!spot) {
    return (
      <View style={[accountStyles.screen, accountStyles.scroll]}>
        <Text style={accountStyles.error}>{t("account.spots.notFound")}</Text>
      </View>
    );
  }

  if (spot.properties.status !== "available") {
    return (
      <View style={[accountStyles.screen, accountStyles.scroll]}>
        <Text style={accountStyles.meta}>
          {t("account.spots.edit.notAvailable", {
            status: spotStatusLabel(t, spot.properties.status),
          })}
        </Text>
        <Pressable
          style={[accountStyles.danger, { marginTop: 16 }]}
          onPress={async () => {
            const ok = await confirm({
              title: t("account.spots.withdraw.confirmTitle"),
              message: t("account.spots.withdraw.confirmMessage"),
              cancelLabel: t("common.cancel"),
              confirmLabel: t("account.spots.withdraw.action"),
              destructive: true,
            });
            if (ok) {
              withdraw.mutate();
            }
          }}
        >
          <Text style={accountStyles.dangerText}>{t("account.spots.withdraw.action")}</Text>
        </Pressable>
      </View>
    );
  }

  return (
    <AuthScroll>
      <Text style={accountStyles.meta}>
        {t("account.spots.edit.listedUntilHint", {
          datetime: formatDateTime(spot.properties.listed_until),
        })}
      </Text>

      <View style={accountStyles.field}>
        <Text style={accountStyles.label}>{t("account.spots.edit.guidePrice")}</Text>
        <AuthTextInput
          value={price}
          onChangeText={setPrice}
          keyboardType="number-pad"
          placeholderTextColor="#7A93A0"
        />
      </View>
      <View style={accountStyles.field}>
        <Text style={accountStyles.label}>{t("account.spots.edit.notes")}</Text>
        <AuthTextInput
          style={{ minHeight: 72, textAlignVertical: "top" }}
          value={notes}
          onChangeText={setNotes}
          multiline
          placeholderTextColor="#7A93A0"
        />
      </View>
      <View style={[accountStyles.row, { marginBottom: 12 }]}>
        <Text style={accountStyles.rowTitle}>{t("account.spots.edit.preferredDeparture")}</Text>
        <Switch value={hasPreferredTime} onValueChange={setHasPreferredTime} />
      </View>
      {hasPreferredTime ? (
        <View style={accountStyles.field}>
          <DateTimeField value={preferredTime} onChange={setPreferredTime} />
        </View>
      ) : null}
      <View style={[accountStyles.row, { marginBottom: 12 }]}>
        <View style={{ flex: 1 }}>
          <Text style={accountStyles.rowTitle}>{t("account.spots.edit.autoCancelNoShow")}</Text>
          <Text style={accountStyles.rowMeta}>{t("account.spots.edit.autoCancelHelp")}</Text>
        </View>
        <Switch value={autoCancel} onValueChange={setAutoCancel} />
      </View>

      <View style={accountStyles.field}>
        <Text style={accountStyles.label}>{t("account.spots.edit.vehicle")}</Text>
        {(vehicles.data ?? []).map((v) => {
          const active = vehicleId === v.id;
          return (
            <Pressable
              key={v.id}
              style={[
                accountStyles.row,
                { marginBottom: 8 },
                active && { borderWidth: 1, borderColor: "#1B9AAA" },
              ]}
              onPress={() => setVehicleId(v.id)}
            >
              <View style={{ flex: 1 }}>
                <Text style={accountStyles.rowTitle}>
                  {v.plate} · {v.make_model}
                </Text>
                <Text style={accountStyles.rowMeta}>
                  {v.color} · {v.year}
                </Text>
              </View>
              {active ? (
                <Text style={accountStyles.link}>{t("account.spots.edit.vehicleSelected")}</Text>
              ) : null}
            </Pressable>
          );
        })}
      </View>

      <Pressable
        style={accountStyles.primary}
        disabled={save.isPending}
        onPress={() => save.mutate()}
      >
        {save.isPending ? (
          <ActivityIndicator color="#fff" />
        ) : (
          <Text style={accountStyles.primaryText}>{t("account.spots.edit.saveSpot")}</Text>
        )}
      </Pressable>

      <View style={accountStyles.section}>
        <Text style={accountStyles.sectionTitle}>{t("account.spots.edit.pendingOffers")}</Text>
        {(offers.data ?? [])
          .filter((offer) => offer.status === "pending")
          .map((offer) => {
            const preferred = matchesPreferredMinute(
              offer.exchange_at,
              spot.properties.preferred_departure_at,
            );
            return (
              <View key={offer.id} style={accountStyles.row}>
                <View style={{ flex: 1, gap: 4 }}>
                  <Text style={accountStyles.rowTitle}>
                    {offer.driver_name?.trim()
                      ? offer.driver_name
                      : t("account.spots.edit.offerDriverFallback")}
                  </Text>
                  <Text style={accountStyles.meta}>
                    {offer.driver_rating != null &&
                    (offer.driver_rating_count ?? 0) > 0
                      ? t("account.rating.withScore", {
                          score: offer.driver_rating.toFixed(1),
                          count: offer.driver_rating_count ?? 0,
                        })
                      : t("profile.public.noRatings")}
                  </Text>
                  <Pressable
                    onPress={() =>
                      router.push(`/user/${offer.driver_id}` as Href)
                    }
                    accessibilityRole="link"
                  >
                    <Text
                      style={[accountStyles.link, { textDecorationLine: "underline" }]}
                    >
                      {t("profile.public.viewOf", {
                        name: firstGivenName(
                          offer.driver_name?.trim()
                            ? offer.driver_name
                            : t("account.spots.edit.offerDriverFallback"),
                        ),
                      })}
                    </Text>
                  </Pressable>
                  <Text style={accountStyles.rowMeta}>
                    {formatPoints(offer.amount_cents)} pts ·{" "}
                    {formatDateTime(offer.exchange_at)}
                  </Text>
                  <Text style={accountStyles.rowMeta}>
                    {preferred
                      ? t("account.spots.edit.offerAtPreferredTime")
                      : t("account.spots.edit.offerAtOtherTime")}
                  </Text>
                </View>
                <View style={{ gap: 6 }}>
                  <Pressable
                    style={[accountStyles.primary, { paddingHorizontal: 12 }]}
                    disabled={decideOffer.isPending}
                    onPress={() => {
                      void (async () => {
                        if (!(await ensureEmailVerified({ t }))) {
                          return;
                        }
                        decideOffer.mutate({ offerId: offer.id, accept: true });
                      })();
                    }}
                  >
                    <Text style={accountStyles.primaryText}>
                      {t("account.spots.edit.acceptOffer")}
                    </Text>
                  </Pressable>
                  <Pressable
                    style={[accountStyles.danger, { paddingHorizontal: 12 }]}
                    disabled={decideOffer.isPending}
                    onPress={() => decideOffer.mutate({ offerId: offer.id, accept: false })}
                  >
                    <Text style={accountStyles.dangerText}>
                      {t("account.spots.edit.rejectOffer")}
                    </Text>
                  </Pressable>
                </View>
              </View>
            );
          })}
        {offers.isLoading ? <ActivityIndicator color="#F4F7FA" /> : null}
        {!offers.isLoading &&
        !(offers.data ?? []).some((offer) => offer.status === "pending") ? (
          <Text style={accountStyles.empty}>{t("account.spots.edit.noPendingOffers")}</Text>
        ) : null}
      </View>

      <Pressable
        style={[accountStyles.danger, { marginTop: 8 }]}
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
            withdraw.mutate();
          }
        }}
      >
        {withdraw.isPending ? (
          <ActivityIndicator color="#FF8FAB" />
        ) : (
          <Text style={accountStyles.dangerText}>{t("account.spots.withdraw.action")}</Text>
        )}
      </Pressable>
    </AuthScroll>
  );
}
