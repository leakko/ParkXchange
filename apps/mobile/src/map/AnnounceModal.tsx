import * as Location from "expo-location";
import { useEffect, useState } from "react";
import {
  ActivityIndicator,
  Alert,
  Modal,
  Pressable,
  ScrollView,
  StyleSheet,
  Switch,
  Text,
  TextInput,
  View,
} from "react-native";

import type { VehicleResponse } from "@/api/client";
import { useTranslation } from "@/i18n";
import { searchAddresses, type AddressSuggestion } from "@/map/geocode";
import { DateTimeField } from "@/ui/DateTimeField";

export type AnnounceValues = {
  guidePriceCents: number;
  preferredDepartureAt: string | null;
  autoCancelNoShow: boolean;
  vehicleId: string;
  lon: number;
  lat: number;
};

type Props = {
  visible: boolean;
  busy: boolean;
  vehicles: VehicleResponse[];
  /** Pre-filled when opening from map pick / long-press. */
  initialCoordinates: [number, number] | null;
  initialAddressLabel: string | null;
  onCancel: () => void;
  /** Hide the form so the user can tap the map, then reopen with coords. */
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
  onCancel,
  onPickOnMap,
  onSubmit,
}: Props) {
  const { t } = useTranslation();
  const [price, setPrice] = useState("1.50");
  const [hasPreferredTime, setHasPreferredTime] = useState(false);
  const [preferredTime, setPreferredTime] = useState(defaultPreferred);
  const [autoCancel, setAutoCancel] = useState(true);
  const [vehicleId, setVehicleId] = useState<string | null>(null);
  const [vehicleOpen, setVehicleOpen] = useState(false);
  const [coords, setCoords] = useState<[number, number] | null>(null);
  const [addressLabel, setAddressLabel] = useState<string | null>(null);
  const [addressQuery, setAddressQuery] = useState("");
  const [suggestions, setSuggestions] = useState<AddressSuggestion[]>([]);
  const [searching, setSearching] = useState(false);
  const [locating, setLocating] = useState(false);

  useEffect(() => {
    if (!visible) {
      return;
    }
    setPreferredTime(defaultPreferred());
    setVehicleOpen(false);
    setSuggestions([]);
    setAddressQuery("");
    setCoords(initialCoordinates);
    setAddressLabel(initialAddressLabel);
    setVehicleId(vehicles[0]?.id ?? null);
  }, [visible, vehicles, initialCoordinates, initialAddressLabel]);

  const selectedVehicle = vehicles.find((v) => v.id === vehicleId) ?? null;

  const runSearch = async () => {
    setSearching(true);
    try {
      const hits = await searchAddresses(addressQuery);
      setSuggestions(hits);
      if (hits.length === 0) {
        Alert.alert(
          t("announce.location.searchEmpty.title"),
          t("announce.location.searchEmpty.message"),
        );
      }
    } catch {
      Alert.alert(t("announce.location.searchFailed.title"), t("common.error"));
    } finally {
      setSearching(false);
    }
  };

  const useMyLocation = async () => {
    setLocating(true);
    try {
      const permission = await Location.requestForegroundPermissionsAsync();
      if (!permission.granted) {
        Alert.alert(
          t("map.alert.announceFailed.title"),
          t("exchange.locationPermissionRequired"),
        );
        return;
      }
      const position = await Location.getCurrentPositionAsync({
        accuracy: Location.Accuracy.Balanced,
      });
      const lon = position.coords.longitude;
      const lat = position.coords.latitude;
      setCoords([lon, lat]);
      setAddressLabel(t("announce.location.currentGps"));
      setSuggestions([]);
    } catch {
      Alert.alert(t("map.alert.announceFailed.title"), t("common.error"));
    } finally {
      setLocating(false);
    }
  };

  const submit = async () => {
    if (!vehicleId) {
      Alert.alert(
        t("announce.needVehicle.title"),
        t("announce.needVehicle.message"),
      );
      return;
    }
    if (!coords) {
      Alert.alert(
        t("announce.location.required.title"),
        t("announce.location.required.message"),
      );
      return;
    }
    const euros = Number.parseFloat(price);
    if (!Number.isFinite(euros) || euros < 0) {
      Alert.alert(
        t("announce.alert.invalidPrice.title"),
        t("announce.alert.invalidPrice.message"),
      );
      return;
    }
    if (hasPreferredTime && !Number.isFinite(preferredTime.getTime())) {
      Alert.alert(
        t("announce.alert.invalidDate.title"),
        t("announce.alert.invalidDate.message"),
      );
      return;
    }
    await onSubmit({
      guidePriceCents: Math.round(euros * 100),
      preferredDepartureAt: hasPreferredTime
        ? preferredTime.toISOString()
        : null,
      autoCancelNoShow: autoCancel,
      vehicleId,
      lon: coords[0],
      lat: coords[1],
    });
  };

  return (
    <Modal visible={visible} transparent animationType="fade" onRequestClose={onCancel}>
      <View style={styles.backdrop}>
        <ScrollView
          contentContainerStyle={styles.scroll}
          keyboardShouldPersistTaps="handled"
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
                          {v.color} · {v.year} · {v.size_class}
                        </Text>
                      </Pressable>
                    ))
                  : null}
              </>
            )}

            <Text style={[styles.label, { marginTop: 4 }]}>
              {t("announce.location.label")}
            </Text>
            <TextInput
              style={styles.input}
              value={addressQuery}
              onChangeText={setAddressQuery}
              placeholder={t("announce.location.placeholder")}
              placeholderTextColor="#7A93A0"
              autoCapitalize="none"
              returnKeyType="search"
              onSubmitEditing={() => {
                void runSearch();
              }}
            />
            <View style={styles.rowBtns}>
              <Pressable
                style={[styles.secondaryBtn, { flex: 1 }]}
                disabled={searching || busy}
                onPress={() => {
                  void runSearch();
                }}
              >
                {searching ? (
                  <ActivityIndicator color="#F4F7FA" />
                ) : (
                  <Text style={styles.secondaryBtnText}>
                    {t("announce.location.search")}
                  </Text>
                )}
              </Pressable>
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
            </View>
            <Pressable
              style={styles.secondaryBtn}
              disabled={busy}
              onPress={onPickOnMap}
            >
              <Text style={styles.secondaryBtnText}>
                {t("announce.location.pickOnMap")}
              </Text>
            </Pressable>

            {suggestions.map((s) => (
              <Pressable
                key={`${s.lon},${s.lat},${s.label}`}
                style={styles.option}
                onPress={() => {
                  setCoords([s.lon, s.lat]);
                  setAddressLabel(s.label);
                  setSuggestions([]);
                  setAddressQuery(s.label);
                }}
              >
                <Text style={styles.optionTitle}>{s.label}</Text>
              </Pressable>
            ))}

            {coords ? (
              <Text style={styles.selectedLoc}>
                {addressLabel ??
                  t("announce.location.coords", {
                    lat: coords[1].toFixed(5),
                    lon: coords[0].toFixed(5),
                  })}
              </Text>
            ) : (
              <Text style={styles.help}>{t("announce.location.noneYet")}</Text>
            )}

            <Text style={styles.label}>{t("announce.guidePrice")}</Text>
            <TextInput
              style={styles.input}
              value={price}
              onChangeText={setPrice}
              keyboardType="decimal-pad"
              placeholderTextColor="#7A93A0"
            />
            <View style={styles.toggleRow}>
              <Text style={styles.label}>{t("announce.preferredDeparture")}</Text>
              <Switch value={hasPreferredTime} onValueChange={setHasPreferredTime} />
            </View>
            {hasPreferredTime ? (
              <DateTimeField value={preferredTime} onChange={setPreferredTime} />
            ) : null}
            <View style={styles.toggleRow}>
              <View style={{ flex: 1 }}>
                <Text style={styles.label}>{t("announce.autoCancel.label")}</Text>
                <Text style={styles.help}>{t("announce.autoCancel.help")}</Text>
              </View>
              <Switch value={autoCancel} onValueChange={setAutoCancel} />
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
        </ScrollView>
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
  scroll: {
    padding: 24,
    paddingVertical: 48,
  },
  card: {
    backgroundColor: "#0B1F33",
    borderRadius: 16,
    padding: 20,
    gap: 10,
  },
  title: { color: "#F4F7FA", fontSize: 19, fontWeight: "700", marginBottom: 4 },
  info: { color: "#9DB4C0", fontSize: 13, lineHeight: 18, marginBottom: 6 },
  label: { color: "#D6E2E9", fontSize: 13 },
  help: { color: "#7A93A0", fontSize: 12, marginTop: 2 },
  selectedLoc: { color: "#1B9AAA", fontSize: 13, fontWeight: "600" },
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
  toggleRow: {
    minHeight: 44,
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    gap: 12,
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
