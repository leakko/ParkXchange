import BottomSheet, {
  BottomSheetScrollView,
  BottomSheetTextInput,
} from "@gorhom/bottom-sheet";
import { forwardRef, useEffect, useMemo, useRef, useState, type ComponentRef } from "react";
import {
  ActivityIndicator,
  Alert,
  Image,
  Pressable,
  StyleSheet,
  Text,
  View,
} from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";

import type {
  OfferResponse,
  ReservationResponse,
  SpotFeature,
  VehicleResponse,
} from "@/api/client";
import { listOffers, peerVehiclePhotoUrl, spotVehiclePhotoUrl } from "@/api/client";
import { useAuthImage } from "@/hooks/useAuthImage";
import { useTranslation } from "@/i18n";
import { sizeClassLabel, spotStatusLabel } from "@/i18n/catalogLabels";
import { formatPoints, parsePointsInput } from "@/i18n/formatPoints";
import { openNavigation } from "@/lib/navigation";
import { DateTimeField } from "@/ui/DateTimeField";
import {
  driverNoShowDeadline,
  ownerNoShowDeadline,
  shouldShowNoShowDeadline,
} from "@/map/exchangeLeave";
import {
  driverCancelMessageKey,
  ownerCancelMessageKey,
} from "@/map/exchangeCopy";
import { ExchangeStatusPanel } from "@/map/ExchangeStatusPanel";
import { PeerVehiclePanel } from "@/map/PeerVehiclePanel";

type Props = {
  spot: SpotFeature | null;
  active: ReservationResponse | null;
  pendingOffer: OfferResponse | null;
  vehicles: VehicleResponse[];
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
  onEnRoute: () => void;
  onReady: () => void;
  onUnready: () => void;
  onCancel: () => void;
  onEdit: (spot: SpotFeature) => void;
  onViewOffers: (spot: SpotFeature) => void;
  onWithdraw: (spot: SpotFeature) => void;
  onManageExchange?: () => void;
};

export const SpotSheet = forwardRef<BottomSheet, Props>(function SpotSheet(
  {
    spot,
    active,
    pendingOffer,
    vehicles,
    isOwner,
    isDriver,
    busy,
    onMakeOffer,
    onWithdrawOffer,
    onAddVehicle,
    onEnRoute,
    onReady,
    onUnready,
    onCancel,
    onEdit,
    onViewOffers,
    onWithdraw,
    onManageExchange,
  },
  ref,
) {
  const { t, formatDateTime } = useTranslation();
  const insets = useSafeAreaInsets();
  const snapPoints = useMemo(() => ["36%", "82%"], []);
  const scrollRef = useRef<ComponentRef<typeof BottomSheetScrollView>>(null);
  const [makingOffer, setMakingOffer] = useState(false);
  const [vehicleId, setVehicleId] = useState("");
  const [exchangeAt, setExchangeAt] = useState(() => new Date(Date.now() + 60 * 60 * 1000));
  const [amount, setAmount] = useState("");
  const [pendingOfferCount, setPendingOfferCount] = useState(0);
  const points = spot ? formatPoints(spot.properties.price_cents) : "";
  const coords = spot?.geometry.coordinates;
  const exact = !!spot?.properties.exact_location;
  const isActiveForSpot =
    !!active && !!spot && String(active.spot_id) === String(spot.id);
  const vehicle = exact ? spot?.properties.vehicle : undefined;
  const ownerPhone = exact ? spot?.properties.owner_phone : undefined;
  const photoUrl =
    vehicle?.has_photo && spot?.id
      ? spotVehiclePhotoUrl(String(spot.id))
      : null;
  const { uri: photoUri } = useAuthImage(photoUrl);
  // During live exchange PeerVehiclePanel is the single peer-car source of truth.
  const showInlineSpotVehicle =
    !!vehicle &&
    !!(vehicle.plate || vehicle.make_model) &&
    !(isActiveForSpot && active);

  useEffect(() => {
    setMakingOffer(false);
    setVehicleId(pendingOffer?.vehicle_id ?? vehicles[0]?.id ?? "");
    setAmount(
      pendingOffer
        ? formatPoints(pendingOffer.amount_cents)
        : points,
    );
    const suggested = pendingOffer
      ? new Date(pendingOffer.exchange_at)
      : spot?.properties.preferred_departure_at
        ? new Date(spot.properties.preferred_departure_at)
        : new Date(Date.now() + 60 * 60 * 1000);
    setExchangeAt(suggested);
  }, [points, spot, vehicles, pendingOffer]);

  useEffect(() => {
    if (
      !spot?.id ||
      !spot.properties.is_mine ||
      spot.properties.status !== "available"
    ) {
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

  const beginOffer = () => {
    if (vehicles.length === 0) {
      Alert.alert(t("spotSheet.offer.needVehicle.title"), t("spotSheet.offer.needVehicle.message"), [
        { text: t("common.cancel"), style: "cancel" },
        { text: t("spotSheet.offer.needVehicle.add"), onPress: onAddVehicle },
      ]);
      return;
    }
    setMakingOffer(true);
    // Offer form is taller than the peek snap; expand so the points field stays visible.
    requestAnimationFrame(() => {
      if (typeof ref !== "function" && ref?.current) {
        ref.current.snapToIndex(1);
      }
    });
  };

  const beginEditOffer = () => {
    if (!pendingOffer) {
      return;
    }
    if (vehicles.length === 0) {
      Alert.alert(t("spotSheet.offer.needVehicle.title"), t("spotSheet.offer.needVehicle.message"), [
        { text: t("common.cancel"), style: "cancel" },
        { text: t("spotSheet.offer.needVehicle.add"), onPress: onAddVehicle },
      ]);
      return;
    }
    setVehicleId(pendingOffer.vehicle_id);
    setAmount(formatPoints(pendingOffer.amount_cents));
    setExchangeAt(new Date(pendingOffer.exchange_at));
    setMakingOffer(true);
    requestAnimationFrame(() => {
      if (typeof ref !== "function" && ref?.current) {
        ref.current.snapToIndex(1);
      }
    });
  };

  const submitOffer = async () => {
    if (!spot) {
      return;
    }
    if (!vehicleId) {
      Alert.alert(
        t("spotSheet.offer.needVehicle.title"),
        t("spotSheet.offer.needVehicle.message"),
        [
          { text: t("common.cancel"), style: "cancel" },
          { text: t("spotSheet.offer.needVehicle.add"), onPress: onAddVehicle },
        ],
      );
      return;
    }
    const offerPoints = parsePointsInput(amount);
    if (!Number.isFinite(exchangeAt.getTime())) {
      Alert.alert(
        t("announce.alert.invalidDate.title"),
        t("announce.alert.invalidDate.message"),
      );
      return;
    }
    if (exchangeAt.getTime() <= Date.now()) {
      Alert.alert(
        t("spotSheet.offer.exchangeInPast.title"),
        t("spotSheet.offer.exchangeInPast.message"),
      );
      return;
    }
    if (offerPoints == null) {
      Alert.alert(
        t("announce.alert.invalidPrice.title"),
        t("announce.alert.invalidPrice.message"),
      );
      return;
    }
    if (pendingOffer) {
      await onWithdrawOffer(pendingOffer);
    }
    await onMakeOffer(spot, vehicleId, exchangeAt.toISOString(), offerPoints);
    setMakingOffer(false);
  };

  return (
    <BottomSheet
      ref={ref}
      index={-1}
      snapPoints={snapPoints}
      enablePanDownToClose
      keyboardBehavior="extend"
      keyboardBlurBehavior="restore"
      android_keyboardInputMode="adjustResize"
      // Above map top chrome (search / spot count); otherwise a full-height
      // drag tucks the sheet under those absolute overlays.
      containerStyle={styles.sheetContainer}
      backgroundStyle={styles.sheet}
      handleIndicatorStyle={styles.handle}
    >
      <BottomSheetScrollView
        ref={scrollRef}
        contentContainerStyle={[
          styles.body,
          {
            paddingBottom: (makingOffer ? 120 : 28) + insets.bottom,
          },
        ]}
        keyboardShouldPersistTaps="handled"
      >
        {spot ? (
          <>
            <Text style={styles.title}>{spot.properties.owner_name}</Text>
            {spot.properties.owner_rating != null ? (
              <Text style={styles.meta}>
                {t("spotSheet.rating", {
                  score: spot.properties.owner_rating.toFixed(1),
                })}
              </Text>
            ) : null}
            {spot.properties.is_mine ? (
              <Text style={styles.mineBadge}>{t("spotSheet.yourListing")}</Text>
            ) : null}
            <Text style={styles.meta}>
              {t("spotSheet.meta", {
                sizeClass: sizeClassLabel(t, spot.properties.size_class),
                points,
                status: spotStatusLabel(t, spot.properties.status),
              })}
            </Text>
            {!exact && !spot.properties.is_mine ? (
              <Text style={styles.approxNotice}>{t("spotSheet.approxLocation")}</Text>
            ) : null}
            {spot.properties.address_hint ? (
              <Text style={styles.hint}>{spot.properties.address_hint}</Text>
            ) : null}
            {spot.properties.notes ? (
              <Text style={styles.notes}>{spot.properties.notes}</Text>
            ) : null}

            {showInlineSpotVehicle ? (
              <View style={styles.vehicleBlock}>
                <Text style={styles.vehicleTitle}>
                  {vehicle.plate}
                  {vehicle.make_model ? ` · ${vehicle.make_model}` : ""}
                </Text>
                <Text style={styles.vehicleMeta}>
                  {[
                    vehicle.color,
                    vehicle.year || null,
                    vehicle.size_class
                      ? sizeClassLabel(t, vehicle.size_class)
                      : null,
                  ]
                    .filter(Boolean)
                    .join(" · ")}
                </Text>
                {photoUri ? (
                  <Image
                    source={{ uri: photoUri }}
                    style={styles.photo}
                    resizeMode="cover"
                  />
                ) : null}
              </View>
            ) : null}

            {ownerPhone ? (
              <Text style={styles.hint}>
                {t("spotSheet.ownerPhone", { phone: ownerPhone })}
              </Text>
            ) : null}

            {isActiveForSpot && active ? (
              <Text style={styles.freeAt}>
                {t("spotSheet.exchange.time", {
                  datetime: formatDateTime(active.exchange_at),
                })}
              </Text>
            ) : pendingOffer && !isActiveForSpot ? (
              <Text style={styles.freeAt}>
                {t("spotSheet.offer.pending.meta", {
                  points: formatPoints(pendingOffer.amount_cents),
                  datetime: formatDateTime(pendingOffer.exchange_at),
                })}
              </Text>
            ) : (
              <>
                <Text style={styles.freeAt}>
                  {spot.properties.preferred_departure_at
                    ? t("spotSheet.freeAt", {
                        datetime: formatDateTime(
                          spot.properties.preferred_departure_at,
                        ),
                      })
                    : t("spotSheet.flexibleDeparture")}
                </Text>
                {spot.properties.status === "available" ? (
                  <Text style={styles.listedUntil}>
                    {t("spotSheet.listedUntil", {
                      datetime: formatDateTime(spot.properties.listed_until),
                    })}
                  </Text>
                ) : null}
              </>
            )}

            <View style={styles.actions}>
              {spot.properties.is_mine &&
              spot.properties.status === "available" &&
              !isActiveForSpot ? (
                <>
                  <Pressable
                    style={styles.primary}
                    disabled={busy}
                    onPress={() => onViewOffers(spot)}
                  >
                    <Text style={styles.primaryText}>
                      {pendingOfferCount > 0
                        ? t("spotSheet.viewOffers", { count: pendingOfferCount })
                        : t("spotSheet.viewOffersEmpty")}
                    </Text>
                  </Pressable>
                  <Pressable
                    style={styles.secondary}
                    disabled={busy}
                    onPress={() => onEdit(spot)}
                  >
                    <Text style={styles.secondaryText}>{t("spotSheet.edit")}</Text>
                  </Pressable>
                  <Pressable
                    style={styles.danger}
                    disabled={busy}
                    onPress={() => onWithdraw(spot)}
                  >
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
                  <Text style={styles.formLabel}>{t("spotSheet.offer.pending.title")}</Text>
                  <Pressable
                    style={styles.primary}
                    disabled={busy}
                    onPress={beginEditOffer}
                  >
                    <Text style={styles.primaryText}>
                      {t("spotSheet.offer.pending.edit")}
                    </Text>
                  </Pressable>
                  <Pressable
                    style={styles.danger}
                    disabled={busy}
                    onPress={() => void onWithdrawOffer(pendingOffer)}
                  >
                    <Text style={styles.dangerText}>
                      {t("spotSheet.offer.pending.withdraw")}
                    </Text>
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
                        <Text style={styles.help}>
                          {t("spotSheet.offer.needVehicle.message")}
                        </Text>
                        <Pressable style={styles.secondary} onPress={onAddVehicle}>
                          <Text style={styles.secondaryText}>
                            {t("spotSheet.offer.needVehicle.add")}
                          </Text>
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
                    <Text style={styles.formLabel}>
                      {t("spotSheet.offer.exchangeDatetime")}
                    </Text>
                    <DateTimeField
                      value={exchangeAt}
                      onChange={setExchangeAt}
                      minimumDate={new Date()}
                    />
                    <Text style={styles.formLabel}>{t("spotSheet.offer.amount")}</Text>
                    <BottomSheetTextInput
                      style={styles.input}
                      value={amount}
                      onChangeText={setAmount}
                      keyboardType="number-pad"
                      placeholderTextColor="#7A93A0"
                      onFocus={() => {
                        requestAnimationFrame(() => {
                          scrollRef.current?.scrollToEnd({ animated: true });
                        });
                      }}
                    />
                    <Pressable
                      style={[
                        styles.primary,
                        (busy || !vehicleId) && styles.primaryDisabled,
                      ]}
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
                  <Pressable
                    style={styles.primary}
                    disabled={busy}
                    onPress={beginOffer}
                  >
                    <Text style={styles.primaryText}>{t("spotSheet.offer.makeOffer")}</Text>
                  </Pressable>
                )
              ) : null}

              {spot.properties.is_mine &&
              (spot.properties.status === "reserved" ||
                spot.properties.status === "handover") &&
              !isActiveForSpot ? (
                <>
                  <Text style={styles.help}>
                    {t("account.spots.reservedNoReservation")}
                  </Text>
                  {onManageExchange ? (
                    <Pressable
                      style={styles.primary}
                      disabled={busy}
                      onPress={onManageExchange}
                    >
                      <Text style={styles.primaryText}>
                        {t("account.spots.openExchange")}
                      </Text>
                    </Pressable>
                  ) : null}
                </>
              ) : null}

              {isActiveForSpot && active ? (
                <>
                  <PeerVehiclePanel
                    vehicle={
                      isOwner ? active.driver_vehicle : active.owner_vehicle
                    }
                    counterpart
                    photoUrl={peerVehiclePhotoUrl(String(active.id))}
                  />
                  <ExchangeStatusPanel
                    res={active}
                    iAmOwner={isOwner}
                    deadlineLabel={(() => {
                      const myReady = isOwner
                        ? active.owner_ready_at
                        : active.driver_ready_at;
                      const deadline = isOwner
                        ? driverNoShowDeadline(active)
                        : ownerNoShowDeadline(active);
                      if (
                        !myReady ||
                        !deadline ||
                        !shouldShowNoShowDeadline(active)
                      ) {
                        return null;
                      }
                      return t("exchange.status.deadline", {
                        datetime: formatDateTime(deadline.toISOString()),
                      });
                    })()}
                  />
                  {(() => {
                    const myReady = isOwner
                      ? active.owner_ready_at
                      : active.driver_ready_at;
                    const myEnRoute = isOwner
                      ? active.owner_en_route_at
                      : active.driver_en_route_at;
                    return (
                      <>
                        {!myReady && !myEnRoute ? (
                          <Pressable
                            style={[styles.secondary, busy && styles.primaryDisabled]}
                            disabled={busy}
                            onPress={() => {
                              Alert.alert(
                                t("exchange.confirm.title"),
                                t("exchange.confirm.enRoute"),
                                [
                                  { text: t("common.cancel"), style: "cancel" },
                                  {
                                    text: t("common.confirm"),
                                    onPress: onEnRoute,
                                  },
                                ],
                              );
                            }}
                          >
                            <Text style={styles.secondaryText}>
                              {t("exchange.actions.enRoute")}
                            </Text>
                          </Pressable>
                        ) : null}
                        {!myReady ? (
                          <Pressable
                            style={[styles.primary, busy && styles.primaryDisabled]}
                            disabled={busy}
                            onPress={() => {
                              Alert.alert(
                                t("exchange.confirm.title"),
                                isOwner
                                  ? t("exchange.confirm.ownerReady")
                                  : t("exchange.confirm.driverReady"),
                                [
                                  { text: t("common.cancel"), style: "cancel" },
                                  {
                                    text: t("common.confirm"),
                                    onPress: onReady,
                                  },
                                ],
                              );
                            }}
                          >
                            {busy ? (
                              <ActivityIndicator color="#fff" />
                            ) : (
                              <Text style={styles.primaryText}>
                                {isOwner
                                  ? t("exchange.actions.ownerReady")
                                  : t("exchange.actions.driverReady")}
                              </Text>
                            )}
                          </Pressable>
                        ) : (
                          <Pressable
                            style={[styles.secondary, busy && styles.primaryDisabled]}
                            disabled={busy}
                            onPress={() => {
                              Alert.alert(
                                t("exchange.confirm.title"),
                                t("exchange.confirm.unready"),
                                [
                                  { text: t("common.cancel"), style: "cancel" },
                                  {
                                    text: t("common.confirm"),
                                    onPress: onUnready,
                                  },
                                ],
                              );
                            }}
                          >
                            <Text style={styles.secondaryText}>
                              {t("exchange.actions.unready")}
                            </Text>
                          </Pressable>
                        )}
                      </>
                    );
                  })()}
                  <Pressable
                    style={styles.danger}
                    disabled={busy}
                    onPress={() => {
                      const message = isOwner
                        ? t(ownerCancelMessageKey(active))
                        : t(driverCancelMessageKey(active));
                      Alert.alert(
                        t("spotSheet.exchange.cancelConfirm.title"),
                        message,
                        [
                          { text: t("common.cancel"), style: "cancel" },
                          {
                            text: t("spotSheet.exchange.cancelConfirm.confirm"),
                            style: "destructive",
                            onPress: onCancel,
                          },
                        ],
                      );
                    }}
                  >
                    <Text style={styles.dangerText}>{t("spotSheet.exchange.cancel")}</Text>
                  </Pressable>
                </>
              ) : null}

              {exact &&
              !spot.properties.is_mine &&
              coords &&
              coords[0] != null &&
              coords[1] != null ? (
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
          </>
        ) : (
          <View />
        )}
      </BottomSheetScrollView>
    </BottomSheet>
  );
});

const styles = StyleSheet.create({
  sheetContainer: {
    zIndex: 40,
    elevation: 40,
  },
  sheet: { backgroundColor: "#0B1F33" },
  handle: { backgroundColor: "#5B7A8C" },
  body: { paddingHorizontal: 20, paddingBottom: 28, gap: 6 },
  title: { color: "#F4F7FA", fontSize: 18, fontWeight: "600" },
  mineBadge: { color: "#1B9AAA", fontSize: 13, fontWeight: "600" },
  meta: { color: "#9DB4C0", fontSize: 14 },
  hint: { color: "#D6E2E9", fontSize: 14, marginTop: 4 },
  notes: { color: "#D6E2E9", fontSize: 14 },
  approxNotice: {
    color: "#FFB4C8",
    fontSize: 13,
    lineHeight: 18,
    marginTop: 6,
    paddingVertical: 8,
    paddingHorizontal: 10,
    backgroundColor: "#2A1520",
    borderRadius: 10,
    overflow: "hidden",
  },
  vehicleBlock: { marginTop: 8, gap: 4 },
  vehicleTitle: { color: "#F4F7FA", fontSize: 15, fontWeight: "600" },
  vehicleMeta: { color: "#9DB4C0", fontSize: 13 },
  photo: {
    marginTop: 8,
    width: "100%",
    height: 140,
    borderRadius: 12,
    backgroundColor: "#16324F",
  },
  freeAt: {
    color: "#F4F7FA",
    fontSize: 17,
    fontWeight: "600",
    marginTop: 10,
  },
  listedUntil: { color: "#7A93A0", fontSize: 12, marginTop: 4 },
  actions: { marginTop: 14, gap: 8 },
  offerForm: { gap: 8 },
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
  exchangeTime: { color: "#F4F7FA", fontSize: 14, fontWeight: "600" },
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
