import * as Location from "expo-location";
import { useEffect, useState } from "react";
import {
  ActivityIndicator,
  Modal,
  Pressable,
  StyleSheet,
  Switch,
  Text,
  View,
} from "react-native";

import type { VehicleResponse } from "@/api/client";
import { AuthScroll } from "@/auth/AuthScroll";
import { AuthTextInput } from "@/auth/AuthTextInput";
import { useTranslation } from "@/i18n";
import { sizeClassLabel } from "@/i18n/catalogLabels";
import { parsePointsInput } from "@/i18n/formatPoints";
import { reverseGeocode } from "@/map/geocode";
import { useConfirm } from "@/ui/ConfirmModal";
import { DateTimeField } from "@/ui/DateTimeField";

export type AnnounceValues = {
  guidePriceCents: number;
  preferredDepartureAt: string | null;
  autoCancelNoShow: boolean;
  leavingNow: boolean;
  vehicleId: string;
  lon: number;
  lat: number;
  /** Human label for the pin; persisted as spot address_hint when present. */
  addressHint: string | null;
};

type Props = {
  visible: boolean;
  busy: boolean;
  vehicles: VehicleResponse[];
  /** Pre-filled when opening from map pick / long-press / history re-announce. */
  initialCoordinates: [number, number] | null;
  initialAddressLabel: string | null;
  /** Guide price in euros as display string base (cents → form uses points). */
  initialGuidePriceCents?: number | null;
  initialVehicleId?: string | null;
  /**
   * When true (return from “pick on map”), keep price / vehicle / toggles
   * and only refresh the location.
   */
  keepForm?: boolean;
  onCancel: () => void;
  /** Hide the form so the user can tap / search on the map, then reopen. */
  onPickOnMap: () => void;
  onSubmit: (values: AnnounceValues) => Promise<void>;
};

function defaultPreferred(): Date {
  return new Date(Date.now() + 60 * 60 * 1000);
}

function vehicleLabel(v: VehicleResponse): string {
  return `${v.plate} · ${v.make_model}`;
}

export function AnnounceModal({
  visible,
  busy,
  vehicles,
  initialCoordinates,
  initialAddressLabel,
  initialGuidePriceCents = null,
  initialVehicleId = null,
  keepForm = false,
  onCancel,
  onPickOnMap,
  onSubmit,
}: Props) {
  const { t, locale } = useTranslation();
  const { alert } = useConfirm();
  const [price, setPrice] = useState("2");
  const [hasPreferredTime, setHasPreferredTime] = useState(false);
  const [leavingNow, setLeavingNow] = useState(false);
  const [preferredTime, setPreferredTime] = useState(defaultPreferred);
  const [autoCancel, setAutoCancel] = useState(true);
  const [vehicleId, setVehicleId] = useState<string | null>(null);
  const [vehicleOpen, setVehicleOpen] = useState(false);
  const [coords, setCoords] = useState<[number, number] | null>(null);
  const [addressLabel, setAddressLabel] = useState<string | null>(null);
  const [locating, setLocating] = useState(false);
  const [locationEditing, setLocationEditing] = useState(true);
  const [resolvingLabel, setResolvingLabel] = useState(false);

  const applySelection = async (
    lon: number,
    lat: number,
    labelHint: string | null,
  ) => {
    setCoords([lon, lat]);
    setLocationEditing(false);
    if (labelHint && !/^-?\d+\.\d+,\s*-?\d+\.\d+$/.test(labelHint.trim())) {
      setAddressLabel(labelHint);
      return;
    }
    setAddressLabel(null);
    setResolvingLabel(true);
    try {
      const resolved = await reverseGeocode(lon, lat, locale);
      setAddressLabel(resolved);
    } catch {
      setAddressLabel(null);
    } finally {
      setResolvingLabel(false);
    }
  };

  useEffect(() => {
    if (!visible) {
      return;
    }
    setVehicleOpen(false);
    if (!keepForm) {
      setPreferredTime(defaultPreferred());
      if (
        initialGuidePriceCents != null &&
        Number.isFinite(initialGuidePriceCents) &&
        initialGuidePriceCents > 0
      ) {
        setPrice(String(Math.round(initialGuidePriceCents)));
      } else {
        setPrice("2");
      }
      setHasPreferredTime(false);
      setLeavingNow(false);
      setAutoCancel(true);
      const prefVehicle =
        (initialVehicleId &&
          vehicles.some((v) => v.id === initialVehicleId) &&
          initialVehicleId) ||
        vehicles[0]?.id ||
        null;
      setVehicleId(prefVehicle);
    } else if (!vehicleId && vehicles[0]) {
      setVehicleId(vehicles[0].id);
    }
    if (initialCoordinates) {
      void applySelection(
        initialCoordinates[0],
        initialCoordinates[1],
        initialAddressLabel,
      );
    } else if (!keepForm) {
      setCoords(null);
      setAddressLabel(null);
      setLocationEditing(true);
    }
    // Seed when the modal opens or the map pick changes.
    // eslint-disable-next-line react-hooks/exhaustive-deps -- intentional
  }, [
    visible,
    vehicles,
    initialCoordinates,
    initialAddressLabel,
    initialGuidePriceCents,
    initialVehicleId,
    keepForm,
  ]);

  const selectedVehicle = vehicles.find((v) => v.id === vehicleId) ?? null;

  const useMyLocation = async () => {
    setLocating(true);
    try {
      const permission = await Location.requestForegroundPermissionsAsync();
      if (!permission.granted) {
        await alert({
          title: t("map.alert.announceFailed.title"),
          message: t("exchange.locationPermissionRequired"),
          confirmLabel: t("common.ok"),
        });
        return;
      }
      const position = await Location.getCurrentPositionAsync({
        accuracy: Location.Accuracy.Balanced,
      });
      await applySelection(
        position.coords.longitude,
        position.coords.latitude,
        null,
      );
    } catch {
      await alert({
        title: t("map.alert.announceFailed.title"),
        message: t("common.error"),
        confirmLabel: t("common.ok"),
      });
    } finally {
      setLocating(false);
    }
  };

  const submit = async () => {
    if (!vehicleId) {
      await alert({
        title: t("announce.needVehicle.title"),
        message: t("announce.needVehicle.message"),
        confirmLabel: t("common.ok"),
      });
      return;
    }
    if (!coords) {
      await alert({
        title: t("announce.location.required.title"),
        message: t("announce.location.required.message"),
        confirmLabel: t("common.ok"),
      });
      return;
    }
    const points = parsePointsInput(price);
    if (points == null) {
      await alert({
        title: t("announce.alert.invalidPrice.title"),
        message: t("announce.alert.invalidPrice.message"),
        confirmLabel: t("common.ok"),
      });
      return;
    }
    if (!leavingNow && hasPreferredTime && !Number.isFinite(preferredTime.getTime())) {
      await alert({
        title: t("announce.alert.invalidDate.title"),
        message: t("announce.alert.invalidDate.message"),
        confirmLabel: t("common.ok"),
      });
      return;
    }
    if (!leavingNow && hasPreferredTime && preferredTime.getTime() <= Date.now()) {
      await alert({
        title: t("announce.alert.dateInPast.title"),
        message: t("announce.alert.dateInPast.message"),
        confirmLabel: t("common.ok"),
      });
      return;
    }
    await onSubmit({
      guidePriceCents: points,
      preferredDepartureAt:
        !leavingNow && hasPreferredTime ? preferredTime.toISOString() : null,
      autoCancelNoShow: autoCancel,
      leavingNow,
      vehicleId,
      lon: coords[0],
      lat: coords[1],
      addressHint: addressLabel,
    });
  };

  const showLocationForm = locationEditing || !coords;
  const coordsText = coords
    ? t("announce.location.coords", {
        lat: coords[1].toFixed(5),
        lon: coords[0].toFixed(5),
      })
    : null;

  return (
    <Modal visible={visible} transparent animationType="fade" onRequestClose={onCancel}>
      <View style={styles.backdrop}>
        <AuthScroll
          style={styles.scrollRoot}
          contentContainerStyle={styles.scroll}
          keyboardVerticalOffset={0}
        >
          <View style={styles.card}>
            <Text style={styles.title}>{t("announce.title")}</Text>
            <Text style={styles.info}>{t("announce.info")}</Text>

            <Text style={styles.label}>{t("announce.vehicle")}</Text>
            {vehicles.length === 0 ? (
              <Text style={styles.help}>{t("announce.needVehicle.message")}</Text>
            ) : (
              <>
                <Pressable
                  style={styles.dropdown}
                  onPress={() => {
                    if (vehicles.length > 1) {
                      setVehicleOpen((open) => !open);
                    }
                  }}
                >
                  <Text style={styles.dropdownText}>
                    {selectedVehicle
                      ? vehicleLabel(selectedVehicle)
                      : t("announce.vehicle")}
                  </Text>
                  {vehicles.length > 1 ? (
                    <Text style={styles.dropdownChevron}>{vehicleOpen ? "▴" : "▾"}</Text>
                  ) : null}
                </Pressable>
                {vehicleOpen
                  ? vehicles.map((v) => (
                      <Pressable
                        key={v.id}
                        style={[
                          styles.option,
                          v.id === vehicleId ? styles.optionActive : null,
                        ]}
                        onPress={() => {
                          setVehicleId(v.id);
                          setVehicleOpen(false);
                        }}
                      >
                        <Text style={styles.optionTitle}>{vehicleLabel(v)}</Text>
                        <Text style={styles.optionMeta}>
                          {v.color} · {v.year} · {sizeClassLabel(t, v.size_class)}
                        </Text>
                      </Pressable>
                    ))
                  : null}
              </>
            )}

            <Text style={[styles.label, { marginTop: 4 }]}>
              {t("announce.location.label")}
            </Text>

            {!showLocationForm && coords ? (
              <View style={styles.selectedCard}>
                <View style={styles.selectedBody}>
                  {resolvingLabel && !addressLabel ? (
                    <ActivityIndicator color="#1B9AAA" />
                  ) : (
                    <Text style={styles.selectedTitle}>
                      {addressLabel ?? coordsText}
                    </Text>
                  )}
                  {addressLabel && coordsText ? (
                    <Text style={styles.selectedCoords}>{coordsText}</Text>
                  ) : null}
                </View>
                <Pressable
                  style={styles.editBtn}
                  accessibilityLabel={t("announce.location.edit")}
                  onPress={() => setLocationEditing(true)}
                >
                  <Text style={styles.editBtnText}>✎</Text>
                </Pressable>
              </View>
            ) : (
              <>
                <View style={styles.rowBtns}>
                  <Pressable
                    style={[styles.secondaryBtn, { flex: 1 }]}
                    disabled={locating || busy}
                    onPress={() => {
                      void useMyLocation();
                    }}
                  >
                    {locating ? (
                      <ActivityIndicator color="#F4F7FA" />
                    ) : (
                      <Text style={styles.secondaryBtnText}>
                        {t("announce.location.useGps")}
                      </Text>
                    )}
                  </Pressable>
                  <Pressable
                    style={[styles.secondaryBtn, { flex: 1 }]}
                    disabled={busy}
                    onPress={onPickOnMap}
                  >
                    <Text style={styles.secondaryBtnText}>
                      {t("announce.location.pickOnMap")}
                    </Text>
                  </Pressable>
                </View>

                {!coords ? (
                  <Text style={styles.help}>{t("announce.location.noneYet")}</Text>
                ) : (
                  <Pressable onPress={() => setLocationEditing(false)}>
                    <Text style={styles.keepSelection}>
                      {t("announce.location.keepSelection")}
                    </Text>
                  </Pressable>
                )}
              </>
            )}

            <Text style={styles.label}>{t("announce.guidePrice")}</Text>
            <AuthTextInput
              style={styles.input}
              value={price}
              onChangeText={setPrice}
              keyboardType="number-pad"
              placeholderTextColor="#7A93A0"
            />
            <View style={styles.toggleGroup}>
              <View style={styles.toggleRow}>
                <Text style={styles.toggleLabel} numberOfLines={2}>
                  {t("announce.leavingNow")}
                </Text>
                <View style={styles.toggleControl}>
                  <Switch
                    value={leavingNow}
                    onValueChange={(v) => {
                      setLeavingNow(v);
                      if (v) {
                        setHasPreferredTime(false);
                      }
                    }}
                    trackColor={{ false: "#1F3A56", true: "#E85D04" }}
                  />
                </View>
              </View>
              <View style={styles.toggleRow}>
                <Text style={styles.toggleLabel} numberOfLines={2}>
                  {t("announce.preferredDeparture")}
                </Text>
                <View style={styles.toggleControl}>
                  <Switch
                    value={hasPreferredTime}
                    disabled={leavingNow}
                    onValueChange={(v) => {
                      setHasPreferredTime(v);
                      if (v) {
                        setLeavingNow(false);
                      }
                    }}
                  />
                </View>
              </View>
              {hasPreferredTime && !leavingNow ? (
                <DateTimeField
                  value={preferredTime}
                  onChange={setPreferredTime}
                  minimumDate={new Date()}
                />
              ) : null}
              <View style={styles.toggleRow}>
                <Text style={styles.toggleLabel} numberOfLines={2}>
                  {t("announce.autoCancel.label")}
                </Text>
                <View style={styles.toggleControl}>
                  <Switch value={autoCancel} onValueChange={setAutoCancel} />
                </View>
              </View>
            </View>
            <Pressable style={styles.primary} disabled={busy} onPress={() => void submit()}>
              {busy ? (
                <ActivityIndicator color="#fff" />
              ) : (
                <Text style={styles.primaryText}>{t("announce.submit")}</Text>
              )}
            </Pressable>
            <Pressable disabled={busy} onPress={onCancel}>
              <Text style={styles.cancel}>{t("common.cancel")}</Text>
            </Pressable>
          </View>
        </AuthScroll>
      </View>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: {
    flex: 1,
    backgroundColor: "rgba(0,0,0,0.6)",
    justifyContent: "center",
  },
  scrollRoot: {
    flex: 1,
    backgroundColor: "transparent",
  },
  scroll: {
    padding: 24,
    paddingVertical: 48,
    flexGrow: 1,
    justifyContent: "center",
  },
  card: {
    backgroundColor: "#0B1F33",
    borderRadius: 16,
    padding: 20,
    gap: 10,
    overflow: "hidden",
  },
  title: { color: "#F4F7FA", fontSize: 19, fontWeight: "700", marginBottom: 4 },
  info: { color: "#9DB4C0", fontSize: 13, lineHeight: 18, marginBottom: 6 },
  label: { color: "#D6E2E9", fontSize: 13 },
  help: { color: "#7A93A0", fontSize: 12, marginTop: 2 },
  selectedCard: {
    backgroundColor: "#0F2740",
    borderWidth: 1,
    borderColor: "#1B9AAA",
    borderRadius: 12,
    paddingHorizontal: 14,
    paddingVertical: 12,
    flexDirection: "row",
    alignItems: "center",
    gap: 10,
  },
  selectedBody: { flex: 1, gap: 4 },
  selectedTitle: {
    color: "#F4F7FA",
    fontSize: 16,
    fontWeight: "700",
    lineHeight: 22,
  },
  selectedCoords: { color: "#1B9AAA", fontSize: 12, fontWeight: "600" },
  editBtn: {
    width: 40,
    height: 40,
    borderRadius: 10,
    backgroundColor: "#16324F",
    alignItems: "center",
    justifyContent: "center",
  },
  editBtnText: { color: "#F4F7FA", fontSize: 18 },
  keepSelection: {
    color: "#1B9AAA",
    fontSize: 13,
    fontWeight: "600",
    textAlign: "center",
    paddingVertical: 6,
  },
  input: {
    backgroundColor: "#0F2740",
    borderWidth: 1,
    borderColor: "#1F3A56",
    borderRadius: 12,
    color: "#F4F7FA",
    paddingHorizontal: 14,
    paddingVertical: 12,
  },
  dropdown: {
    backgroundColor: "#0F2740",
    borderWidth: 1,
    borderColor: "#1F3A56",
    borderRadius: 12,
    paddingHorizontal: 14,
    paddingVertical: 12,
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
  },
  dropdownText: { color: "#F4F7FA", fontSize: 15, fontWeight: "600", flex: 1 },
  dropdownChevron: { color: "#9DB4C0", fontSize: 14, marginLeft: 8 },
  option: {
    backgroundColor: "#16324F",
    borderRadius: 12,
    paddingVertical: 10,
    paddingHorizontal: 14,
  },
  optionActive: { borderWidth: 1, borderColor: "#1B9AAA" },
  optionTitle: { color: "#F4F7FA", fontWeight: "600", fontSize: 14 },
  optionMeta: { color: "#9DB4C0", fontSize: 12, marginTop: 2 },
  rowBtns: { flexDirection: "row", gap: 8 },
  secondaryBtn: {
    backgroundColor: "#16324F",
    borderRadius: 12,
    paddingVertical: 12,
    alignItems: "center",
  },
  secondaryBtnText: { color: "#F4F7FA", fontWeight: "600", fontSize: 13 },
  toggleGroup: {
    gap: 5,
    width: "100%",
    paddingHorizontal: 4,
    marginVertical: 4,
  },
  toggleRow: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    gap: 8,
    width: "100%",
    paddingVertical: 2,
  },
  toggleLabel: {
    color: "#D6E2E9",
    fontSize: 13,
    lineHeight: 18,
    flex: 1,
    flexShrink: 1,
    minWidth: 0,
    paddingRight: 10,
  },
  toggleControl: {
    flexShrink: 0,
    marginRight: 2,
    transform: [{ scale: 0.9 }],
  },
  primary: {
    backgroundColor: "#1B9AAA",
    borderRadius: 12,
    paddingVertical: 12,
    alignItems: "center",
    marginTop: 4,
  },
  primaryText: { color: "#fff", fontSize: 15, fontWeight: "600" },
  cancel: { color: "#9DB4C0", textAlign: "center", paddingVertical: 8 },
});
