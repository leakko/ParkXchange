import { useEffect, useState, type ComponentType } from "react";
import {
  ActivityIndicator,
  Pressable,
  StyleSheet,
  Text,
  TextInput as RNTextInput,
  View,
  type TextInputProps,
} from "react-native";

import type {
  OfferResponse,
  ReservationResponse,
  SpotFeature,
  VehicleResponse,
} from "@/api/client";
import { listOffers, peerVehiclePhotoUrl, spotVehiclePhotoUrl } from "@/api/client";
import { useTranslation } from "@/i18n";
import { carSizeLabel, spotStatusLabel } from "@/i18n/catalogLabels";
import { formatPoints, parsePointsInput } from "@/i18n/formatPoints";
import { openNavigation } from "@/lib/navigation";
import { driverCancelMessageKey, ownerCancelMessageKey } from "@/map/exchangeCopy";
import { passAuthGate } from "@/map/authGate";
import {
  driverNoShowDeadline,
  ownerNoShowDeadline,
  shouldShowNoShowDeadline,
} from "@/map/exchangeLeave";
import { ExchangeStatusPanel } from "@/map/ExchangeStatusPanel";
import { OccupyingVehiclePanel } from "@/map/OccupyingVehiclePanel";
import { OtherDetailsPanel } from "@/map/OtherDetailsPanel";
import { PeerVehiclePanel } from "@/map/PeerVehiclePanel";
import { useStreetAddress } from "@/map/useStreetAddress";
import { DateTimeField } from "@/ui/DateTimeField";
import { useConfirm } from "@/ui/ConfirmModal";
import { leavingNowExchangeFromChip } from "@/map/leavingNowOffer";

export type SpotSheetBodyProps = {
  spot: SpotFeature | null;
  active: ReservationResponse | null;
  pendingOffer: OfferResponse | null;
  vehicles: VehicleResponse[];
  signedIn: boolean;
  isOwner: boolean;
  isDriver: boolean;
  busy?: boolean;
  onMakeOffer: (
    spot: SpotFeature,
    vehicleId: string,
    exchangeAt: string,
    amountCents: number,
  ) => Promise<void>;
  onWithdrawOffer: (offer: OfferResponse) => Promise<void>;
  onAddVehicle: () => void;
  onRequireSignIn: () => void;
  onEnRoute: () => void;
  onReady: () => void;
  onUnready: () => void;
  onCancel: () => void;
  onEdit: (spot: SpotFeature) => void;
  onViewOffers: (spot: SpotFeature) => void;
  onWithdraw: (spot: SpotFeature) => void;
  /** Convert unpublished parked reminder into a public listing. */
  onPublish?: (spot: SpotFeature) => void;
  onReportListing?: (spot: SpotFeature) => void;
  makingOffer: boolean;
  setMakingOffer: (v: boolean) => void;
  onExpandSheet: () => void;
  /** Optional sheet-aware input; defaults to RN TextInput. */
  TextInput?: ComponentType<TextInputProps>;
};

/**
 * Spot detail body — presentation only; no sheet/gesture wiring.
 */
export function SpotSheetBody({
  spot,
  active,
  pendingOffer,
  vehicles,
  signedIn,
  isOwner,
  busy,
  onMakeOffer,
  onWithdrawOffer,
  onAddVehicle,
  onRequireSignIn,
  onEnRoute,
  onReady,
  onUnready,
  onCancel,
  onEdit,
  onViewOffers,
  onWithdraw,
  onPublish,
  onReportListing,
  makingOffer,
  setMakingOffer,
  onExpandSheet,
  TextInput = RNTextInput,
}: SpotSheetBodyProps) {
  const { t, formatDateTime } = useTranslation();
  const { confirm, alert } = useConfirm();
  const [vehicleId, setVehicleId] = useState("");
  const [exchangeAt, setExchangeAt] = useState(() => new Date(Date.now() + 60 * 60 * 1000));
  const [amount, setAmount] = useState("");
  const [pendingOfferCount, setPendingOfferCount] = useState(0);

  const points = spot ? formatPoints(spot.properties.price_cents) : "";
  const coords = spot?.geometry.coordinates;
  const exact = !!spot?.properties.exact_location;
  const isActiveForSpot = !!active && !!spot && String(active.spot_id) === String(spot.id);
  const isUnpublishedMine =
    !!spot?.properties.is_mine && spot.properties.status === "unpublished" && !isActiveForSpot;
  const vehicle = exact ? spot?.properties.vehicle : undefined;
  const ownerPhone = exact ? spot?.properties.owner_phone : undefined;
  const vehiclePhotoUrl =
    vehicle?.has_photo && spot?.id ? spotVehiclePhotoUrl(String(spot.id)) : null;
  const showInlineSpotVehicle =
    !!vehicle && !!(vehicle.plate || vehicle.make_model) && !(isActiveForSpot && active);
  const streetAddress = useStreetAddress(
    coords?.[0] != null ? Number(coords[0]) : null,
    coords?.[1] != null ? Number(coords[1]) : null,
    spot?.properties.address_hint,
  );

  const deadlineLabel =
    isActiveForSpot && active
      ? (() => {
          const myReady = isOwner ? active.owner_ready_at : active.driver_ready_at;
          const deadline = isOwner ? driverNoShowDeadline(active) : ownerNoShowDeadline(active);
          if (!myReady || !deadline || !shouldShowNoShowDeadline(active)) {
            return null;
          }
          return t("exchange.status.deadline", {
            datetime: formatDateTime(deadline.toISOString()),
          });
        })()
      : null;

  const myReady =
    isActiveForSpot && active ? (isOwner ? active.owner_ready_at : active.driver_ready_at) : null;
  const myEnRoute =
    isActiveForSpot && active
      ? isOwner
        ? active.owner_en_route_at
        : active.driver_en_route_at
      : null;

  // Reset offer form when the spot or pending offer changes — not when the
  // vehicle list refreshes after adding a car mid-flow.
  useEffect(() => {
    setMakingOffer(false);
    setVehicleId(pendingOffer?.vehicle_id ?? "");
    setAmount(pendingOffer ? formatPoints(pendingOffer.amount_cents) : points);
    const suggested = pendingOffer
      ? new Date(pendingOffer.exchange_at)
      : spot?.properties.leaving_now
        ? new Date(Date.now() + 5 * 60_000)
        : spot?.properties.preferred_departure_at
          ? new Date(spot.properties.preferred_departure_at)
          : new Date(Date.now() + 60 * 60 * 1000);
    setExchangeAt(suggested);
  }, [
    points,
    spot?.id,
    pendingOffer,
    setMakingOffer,
    spot?.properties.leaving_now,
    spot?.properties.preferred_departure_at,
  ]);

  // After “add car”, makingOffer flips on with a possibly stale exchangeAt
  // (soft-gate returned before beginOffer set the +5 min chip).
  useEffect(() => {
    if (!makingOffer || !spot?.properties.leaving_now || pendingOffer) {
      return;
    }
    const now = Date.now();
    const nearChip = ([5, 15, 30] as const).some(
      (mins) => Math.abs(exchangeAt.getTime() - (now + mins * 60_000)) < 45_000,
    );
    if (!nearChip) {
      setAmount(formatPoints(spot.properties.price_cents));
      setExchangeAt(new Date(now + 5 * 60_000));
    }
  }, [makingOffer]); // eslint-disable-line react-hooks/exhaustive-deps -- snap once when form opens

  useEffect(() => {
    setVehicleId((current) => {
      if (pendingOffer?.vehicle_id && vehicles.some((v) => v.id === pendingOffer.vehicle_id)) {
        return pendingOffer.vehicle_id;
      }
      if (current && vehicles.some((v) => v.id === current)) {
        return current;
      }
      return vehicles[0]?.id ?? "";
    });
  }, [vehicles, pendingOffer?.vehicle_id]);

  useEffect(() => {
    if (!spot?.id || !spot.properties.is_mine || spot.properties.status !== "available") {
      setPendingOfferCount(0);
      return;
    }
    let cancelled = false;
    void (async () => {
      try {
        const offers = await listOffers(String(spot.id));
        if (!cancelled) {
          setPendingOfferCount(offers.filter((o) => o.status === "pending").length);
        }
      } catch {
        if (!cancelled) {
          setPendingOfferCount(0);
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [spot?.id, spot?.properties.is_mine, spot?.properties.status]);

  const requireAuth = async (
    messageKey: "auth.required.offer" | "auth.required.addVehicle" | "auth.required.report",
  ): Promise<boolean> =>
    passAuthGate({
      signedIn,
      confirmSignIn: () =>
        confirm({
          title: t("auth.required.title"),
          message: t(messageKey),
          cancelLabel: t("common.cancel"),
          confirmLabel: t("auth.required.signIn"),
        }),
      onRequireSignIn,
    });

  const ensureVehicle = async (): Promise<boolean> => {
    if (!(await requireAuth("auth.required.offer"))) {
      return false;
    }
    if (vehicles.length > 0) {
      return true;
    }
    const add = await confirm({
      title: t("spotSheet.offer.needVehicle.title"),
      message: t("spotSheet.offer.needVehicle.message"),
      cancelLabel: t("common.cancel"),
      confirmLabel: t("spotSheet.offer.needVehicle.add"),
    });
    if (add) {
      onAddVehicle();
    }
    return false;
  };

  const beginAddVehicle = async () => {
    if (await requireAuth("auth.required.addVehicle")) {
      onAddVehicle();
    }
  };

  const beginOffer = async () => {
    if (!(await ensureVehicle())) {
      return;
    }
    if (spot?.properties.leaving_now) {
      setAmount(formatPoints(spot.properties.price_cents));
      setExchangeAt(new Date(Date.now() + 5 * 60_000));
    }
    setMakingOffer(true);
    requestAnimationFrame(onExpandSheet);
  };

  const beginEditOffer = async () => {
    if (!pendingOffer) {
      return;
    }
    if (!(await ensureVehicle())) {
      return;
    }
    setVehicleId(pendingOffer.vehicle_id);
    setAmount(formatPoints(pendingOffer.amount_cents));
    setExchangeAt(new Date(pendingOffer.exchange_at));
    setMakingOffer(true);
    requestAnimationFrame(onExpandSheet);
  };

  const submitOffer = async () => {
    if (!spot) {
      return;
    }
    if (!vehicleId) {
      await ensureVehicle();
      return;
    }
    const offerPoints = spot.properties.leaving_now
      ? spot.properties.price_cents
      : parsePointsInput(amount);
    // Leaving-now chips are relative to "now": re-resolve at submit so a pause
    // (e.g. add vehicle) cannot send a stale absolute timestamp outside 5/15/30.
    const submitExchangeAt = spot.properties.leaving_now
      ? leavingNowExchangeFromChip(exchangeAt)
      : exchangeAt;
    if (!Number.isFinite(submitExchangeAt.getTime())) {
      await alert({
        title: t("announce.alert.invalidDate.title"),
        message: t("announce.alert.invalidDate.message"),
        confirmLabel: t("common.ok"),
      });
      return;
    }
    if (submitExchangeAt.getTime() <= Date.now()) {
      await alert({
        title: t("spotSheet.offer.exchangeInPast.title"),
        message: t("spotSheet.offer.exchangeInPast.message"),
        confirmLabel: t("common.ok"),
      });
      return;
    }
    if (offerPoints == null) {
      await alert({
        title: t("announce.alert.invalidPrice.title"),
        message: t("announce.alert.invalidPrice.message"),
        confirmLabel: t("common.ok"),
      });
      return;
    }
    if (pendingOffer) {
      await onWithdrawOffer(pendingOffer);
    }
    await onMakeOffer(spot, vehicleId, submitExchangeAt.toISOString(), offerPoints);
    setMakingOffer(false);
  };

  if (!spot) {
    return <View />;
  }

  // Peek-first: time + points + primary actions.
  // Header already shows “Tu anuncio” / owner name — don't repeat it here.
  const headlineTime = isUnpublishedMine ? (
    <Text style={styles.freeAt}>{t("spotSheet.unpublished.headline")}</Text>
  ) : isActiveForSpot && active ? (
    <Text style={styles.freeAt}>
      {t("spotSheet.exchange.time", {
        datetime: formatDateTime(active.exchange_at),
      })}
    </Text>
  ) : pendingOffer && !isActiveForSpot ? (
    <Text style={styles.freeAt}>
      {t("spotSheet.offer.pending.meta", {
        points: formatPoints(pendingOffer.amount_cents),
        datetime: spot.properties.leaving_now
          ? t("spotSheet.leavingNowDeparture")
          : formatDateTime(pendingOffer.exchange_at),
      })}
    </Text>
  ) : (
    <Text style={styles.freeAt}>
      {spot.properties.leaving_now
        ? t("spotSheet.leavingNowDeparture")
        : spot.properties.preferred_departure_at
          ? t("spotSheet.freeAt", {
              datetime: formatDateTime(spot.properties.preferred_departure_at),
            })
          : t("spotSheet.flexibleDeparture")}
    </Text>
  );

  const pointsLine = isUnpublishedMine ? (
    <Text style={styles.pointsHero}>
      <Text style={styles.meta}>
        {carSizeLabel(t, spot.properties.size_class)}
        {" · "}
        {spotStatusLabel(t, spot.properties.status)}
      </Text>
    </Text>
  ) : isActiveForSpot && active ? (
    <Text style={styles.pointsHero}>
      {formatPoints(active.price_cents)} pts
      <Text style={styles.meta}>
        {" · "}
        {carSizeLabel(t, spot.properties.size_class)}
      </Text>
    </Text>
  ) : pendingOffer && !isActiveForSpot ? null : (
    <Text style={styles.pointsHero}>
      {points} pts
      <Text style={styles.meta}>
        {" · "}
        {carSizeLabel(t, spot.properties.size_class)}
        {" · "}
        {spotStatusLabel(t, spot.properties.status)}
      </Text>
    </Text>
  );

  return (
    <>
      {headlineTime}
      {pointsLine}

      <View style={styles.actions}>
        {isUnpublishedMine ? (
          <>
            <View style={styles.publishPromo}>
              <Text style={styles.publishPromoText}>{t("spotSheet.unpublished.promo")}</Text>
            </View>
            <Pressable
              style={styles.primary}
              disabled={busy}
              onPress={() => onPublish?.(spot)}
            >
              <Text style={styles.primaryText}>{t("spotSheet.unpublished.publish")}</Text>
            </Pressable>
            <Pressable style={styles.danger} disabled={busy} onPress={() => onWithdraw(spot)}>
              <Text style={styles.dangerText}>{t("spotSheet.withdraw")}</Text>
            </Pressable>
          </>
        ) : null}

        {spot.properties.is_mine && spot.properties.status === "available" && !isActiveForSpot ? (
          <>
            <Pressable style={styles.primary} disabled={busy} onPress={() => onViewOffers(spot)}>
              <Text style={styles.primaryText}>
                {pendingOfferCount > 0
                  ? t("spotSheet.viewOffers", { count: pendingOfferCount })
                  : t("spotSheet.viewOffersEmpty")}
              </Text>
            </Pressable>
            <Pressable style={styles.secondary} disabled={busy} onPress={() => onEdit(spot)}>
              <Text style={styles.secondaryText}>{t("spotSheet.edit")}</Text>
            </Pressable>
            <Pressable style={styles.danger} disabled={busy} onPress={() => onWithdraw(spot)}>
              <Text style={styles.dangerText}>{t("spotSheet.withdraw")}</Text>
            </Pressable>
          </>
        ) : null}

        {!spot.properties.is_mine &&
        !isActiveForSpot &&
        spot.properties.status === "available" &&
        pendingOffer &&
        !makingOffer ? (
          <View style={styles.offerForm}>
            <Pressable style={styles.primary} disabled={busy} onPress={() => void beginEditOffer()}>
              <Text style={styles.primaryText}>{t("spotSheet.offer.pending.edit")}</Text>
            </Pressable>
            <Pressable
              style={styles.danger}
              disabled={busy}
              onPress={() => void onWithdrawOffer(pendingOffer)}
            >
              <Text style={styles.dangerText}>{t("spotSheet.offer.pending.withdraw")}</Text>
            </Pressable>
          </View>
        ) : null}

        {!spot.properties.is_mine &&
        !isActiveForSpot &&
        spot.properties.status === "available" &&
        (makingOffer || !pendingOffer) ? (
          makingOffer ? (
            <View style={styles.offerForm}>
              <Text style={styles.formLabel}>{t("spotSheet.offer.yourVehicle")}</Text>
              {vehicles.length === 0 ? (
                <>
                  <Text style={styles.help}>{t("spotSheet.offer.needVehicle.message")}</Text>
                  <Pressable style={styles.secondary} onPress={() => void beginAddVehicle()}>
                    <Text style={styles.secondaryText}>{t("spotSheet.offer.needVehicle.add")}</Text>
                  </Pressable>
                </>
              ) : (
                vehicles.map((candidate) => (
                  <Pressable
                    key={candidate.id}
                    style={[
                      styles.vehicleChoice,
                      candidate.id === vehicleId && styles.vehicleChoiceActive,
                    ]}
                    onPress={() => setVehicleId(candidate.id)}
                  >
                    <Text style={styles.secondaryText}>
                      {candidate.plate} · {candidate.make_model}
                    </Text>
                  </Pressable>
                ))
              )}
              <Text style={styles.formLabel}>{t("spotSheet.offer.exchangeDatetime")}</Text>
              {spot.properties.leaving_now ? (
                <View style={styles.etaRow}>
                  {([5, 15, 30] as const).map((mins) => (
                    <Pressable
                      key={mins}
                      style={[
                        styles.etaChip,
                        Math.abs(exchangeAt.getTime() - (Date.now() + mins * 60_000)) < 45_000 &&
                          styles.etaChipActive,
                      ]}
                      onPress={() => setExchangeAt(new Date(Date.now() + mins * 60_000))}
                    >
                      <Text style={styles.etaChipText}>
                        {t("spotSheet.offer.etaMinutes", { minutes: mins })}
                      </Text>
                    </Pressable>
                  ))}
                </View>
              ) : (
                <DateTimeField
                  value={exchangeAt}
                  onChange={setExchangeAt}
                  minimumDate={new Date()}
                />
              )}
              <Text style={styles.formLabel}>{t("spotSheet.offer.amount")}</Text>
              {spot.properties.leaving_now ? (
                <Text style={styles.help}>
                  {t("spotSheet.offer.fixedPrice", {
                    points: formatPoints(spot.properties.price_cents),
                  })}
                </Text>
              ) : (
                <TextInput
                  style={styles.input}
                  value={amount}
                  onChangeText={setAmount}
                  keyboardType="number-pad"
                  placeholderTextColor="#7A93A0"
                />
              )}
              <Pressable
                style={[styles.primary, (busy || !vehicleId) && styles.primaryDisabled]}
                disabled={busy || !vehicleId}
                onPress={() => void submitOffer()}
              >
                {busy ? (
                  <ActivityIndicator color="#fff" />
                ) : (
                  <Text style={styles.primaryText}>{t("spotSheet.offer.submit")}</Text>
                )}
              </Pressable>
              <Pressable onPress={() => setMakingOffer(false)}>
                <Text style={styles.cancelText}>{t("common.cancel")}</Text>
              </Pressable>
            </View>
          ) : (
            <Pressable style={styles.primary} disabled={busy} onPress={() => void beginOffer()}>
              <Text style={styles.primaryText}>{t("spotSheet.offer.makeOffer")}</Text>
            </Pressable>
          )
        ) : null}

        {spot.properties.is_mine &&
        (spot.properties.status === "reserved" || spot.properties.status === "handover") &&
        !isActiveForSpot ? (
          <Text style={styles.help}>{t("account.spots.reservedNoReservation")}</Text>
        ) : null}

        {isActiveForSpot && active ? (
          <>
            {!myReady && !myEnRoute ? (
              <Pressable
                style={[styles.secondary, busy && styles.primaryDisabled]}
                disabled={busy}
                onPress={onEnRoute}
              >
                <Text style={styles.secondaryText}>{t("exchange.actions.enRoute")}</Text>
              </Pressable>
            ) : null}
            {!myReady ? (
              <Pressable
                style={[styles.primary, busy && styles.primaryDisabled]}
                disabled={busy}
                onPress={onReady}
              >
                {busy ? (
                  <ActivityIndicator color="#fff" />
                ) : (
                  <Text style={styles.primaryText}>
                    {isOwner ? t("exchange.actions.ownerReady") : t("exchange.actions.driverReady")}
                  </Text>
                )}
              </Pressable>
            ) : (
              <Pressable
                style={[styles.secondary, busy && styles.primaryDisabled]}
                disabled={busy}
                onPress={() => {
                  void (async () => {
                    const ok = await confirm({
                      title: t("exchange.confirm.title"),
                      message: t("exchange.confirm.unready"),
                      cancelLabel: t("common.cancel"),
                      confirmLabel: t("common.confirm"),
                    });
                    if (ok) onUnready();
                  })();
                }}
              >
                <Text style={styles.secondaryText}>{t("exchange.actions.unready")}</Text>
              </Pressable>
            )}
            <Pressable
              style={styles.danger}
              disabled={busy}
              onPress={() => {
                void (async () => {
                  const message = isOwner
                    ? t(ownerCancelMessageKey(active))
                    : t(driverCancelMessageKey(active));
                  const ok = await confirm({
                    title: t("spotSheet.exchange.cancelConfirm.title"),
                    message,
                    cancelLabel: t("common.cancel"),
                    confirmLabel: t("spotSheet.exchange.cancelConfirm.confirm"),
                    destructive: true,
                  });
                  if (ok) onCancel();
                })();
              }}
            >
              <Text style={styles.dangerText}>{t("spotSheet.exchange.cancel")}</Text>
            </Pressable>
          </>
        ) : null}

        {coords && coords[0] != null && coords[1] != null ? (
          <Pressable
            style={styles.secondary}
            onPress={() =>
              void openNavigation(
                { lon: coords[0]!, lat: coords[1]! },
                {
                  failedTitle: t("navigation.failed.title"),
                  failedMessage: t("navigation.failed.message"),
                },
              )
            }
          >
            <Text style={styles.secondaryText}>{t("spotSheet.navigate")}</Text>
          </Pressable>
        ) : null}
      </View>

      {!exact && !spot.properties.is_mine ? (
        <Text style={styles.approxNotice}>{t("spotSheet.approxLocation")}</Text>
      ) : null}

      {/* Secondary detail — below the peek fold */}
      {isActiveForSpot && active ? (
        <>
          <ExchangeStatusPanel res={active} iAmOwner={isOwner} deadlineLabel={deadlineLabel} />
          <PeerVehiclePanel
            vehicle={isOwner ? active.driver_vehicle : active.owner_vehicle}
            counterpart
            photoUrl={peerVehiclePhotoUrl(String(active.id))}
          />
        </>
      ) : null}

      {showInlineSpotVehicle ? (
        <OccupyingVehiclePanel vehicle={vehicle} photoUrl={vehiclePhotoUrl} />
      ) : null}

      <OtherDetailsPanel
        address={streetAddress}
        ownerContact={ownerPhone}
        comments={spot.properties.notes}
      />

      {!spot.properties.is_mine && onReportListing ? (
        <Pressable
          style={styles.reportHit}
          disabled={busy}
          onPress={() => {
            void (async () => {
              if (!(await requireAuth("auth.required.report"))) {
                return;
              }
              onReportListing(spot);
            })();
          }}
        >
          <Text style={styles.reportText}>{t("report.spot.cta")}</Text>
        </Pressable>
      ) : null}

      {!isActiveForSpot && !pendingOffer && spot.properties.status === "available" ? (
        <Text style={styles.listedUntil}>
          {t("spotSheet.listedUntil", {
            datetime: formatDateTime(spot.properties.listed_until),
          })}
        </Text>
      ) : null}
    </>
  );
}

const styles = StyleSheet.create({
  publishPromo: {
    backgroundColor: "#1A73E822",
    borderColor: "#1A73E8",
    borderWidth: 1,
    borderRadius: 12,
    padding: 12,
  },
  publishPromoText: { color: "#E8F1FF", fontSize: 14, lineHeight: 20, fontWeight: "600" },
  meta: { color: "#9DB4C0", fontSize: 14, fontWeight: "400" },
  pointsHero: {
    color: "#F4F7FA",
    fontSize: 16,
    fontWeight: "700",
    marginTop: 2,
  },
  approxNotice: {
    color: "#FFB4C8",
    fontSize: 13,
    lineHeight: 18,
    marginTop: 10,
    paddingVertical: 8,
    paddingHorizontal: 10,
    backgroundColor: "#2A1520",
    borderRadius: 10,
    overflow: "hidden",
  },
  freeAt: {
    color: "#F4F7FA",
    fontSize: 18,
    fontWeight: "700",
    marginTop: 4,
  },
  listedUntil: { color: "#7A93A0", fontSize: 12, marginTop: 10 },
  reportHit: { marginTop: 14, paddingVertical: 8, alignItems: "center" },
  reportText: { color: "#9DB4C0", fontSize: 13, fontWeight: "600" },
  actions: { marginTop: 12, gap: 8 },
  offerForm: { gap: 8 },
  etaRow: { flexDirection: "row", gap: 8, flexWrap: "wrap" },
  etaChip: {
    borderWidth: 1,
    borderColor: "#1F3A56",
    borderRadius: 10,
    paddingHorizontal: 12,
    paddingVertical: 8,
    backgroundColor: "#16324F",
  },
  etaChipActive: { borderColor: "#E85D04", backgroundColor: "#3D2410" },
  etaChipText: { color: "#F4F7FA", fontWeight: "600", fontSize: 13 },
  formLabel: { color: "#9DB4C0", fontSize: 12, marginTop: 4 },
  help: { color: "#7A93A0", fontSize: 13, lineHeight: 18 },
  input: {
    backgroundColor: "#0F2740",
    borderWidth: 1,
    borderColor: "#1F3A56",
    borderRadius: 10,
    color: "#F4F7FA",
    paddingHorizontal: 12,
    paddingVertical: 10,
  },
  vehicleChoice: {
    backgroundColor: "#16324F",
    borderWidth: 1,
    borderColor: "#1F3A56",
    borderRadius: 10,
    padding: 10,
  },
  vehicleChoiceActive: { borderColor: "#1B9AAA" },
  cancelText: { color: "#9DB4C0", textAlign: "center", paddingVertical: 8 },
  primary: {
    backgroundColor: "#1B9AAA",
    borderRadius: 12,
    paddingVertical: 12,
    alignItems: "center",
  },
  primaryDisabled: { opacity: 0.45 },
  primaryText: { color: "#fff", fontWeight: "600", fontSize: 15 },
  secondary: {
    backgroundColor: "#16324F",
    borderRadius: 12,
    paddingVertical: 12,
    alignItems: "center",
  },
  secondaryText: { color: "#F4F7FA", fontWeight: "600", fontSize: 15 },
  danger: {
    backgroundColor: "#3D1F2B",
    borderRadius: 12,
    paddingVertical: 12,
    alignItems: "center",
  },
  dangerText: { color: "#FF8FAB", fontWeight: "600", fontSize: 15 },
});
