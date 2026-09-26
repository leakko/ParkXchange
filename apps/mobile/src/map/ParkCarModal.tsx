import { useEffect, useState } from "react";
import {
  ActivityIndicator,
  Modal,
  Pressable,
  StyleSheet,
  Text,
  View,
} from "react-native";

import type { VehicleResponse } from "@/api/client";
import { AuthScroll } from "@/auth/AuthScroll";
import { useTranslation } from "@/i18n";

export type ParkCarValues = {
  vehicleId: string;
  lon: number;
  lat: number;
  addressHint: string | null;
};

type Props = {
  visible: boolean;
  busy: boolean;
  vehicles: VehicleResponse[];
  /** Pre-filled coords from GPS / long-press / pick-on-map. */
  initialCoordinates: [number, number] | null;
  initialAddressLabel: string | null;
  onCancel: () => void;
  onPickOnMap: () => void;
  onAddVehicle: () => void;
  onSubmit: (values: ParkCarValues) => Promise<void>;
};

/**
 * Short flow to save an unpublished parked-car reminder.
 * Publishing for others is a later step from the spot sheet.
 */
export function ParkCarModal({
  visible,
  busy,
  vehicles,
  initialCoordinates,
  initialAddressLabel,
  onCancel,
  onPickOnMap,
  onAddVehicle,
  onSubmit,
}: Props) {
  const { t } = useTranslation();
  const [vehicleId, setVehicleId] = useState<string | null>(null);
  const [vehicleOpen, setVehicleOpen] = useState(false);
  const [coords, setCoords] = useState<[number, number] | null>(null);
  const [addressLabel, setAddressLabel] = useState<string | null>(null);

  useEffect(() => {
    if (!visible) {
      return;
    }
    setCoords(initialCoordinates);
    setAddressLabel(initialAddressLabel);
    setVehicleId((prev) => {
      if (prev && vehicles.some((v) => v.id === prev)) {
        return prev;
      }
      return vehicles[0]?.id ?? null;
    });
  }, [visible, initialCoordinates, initialAddressLabel, vehicles]);

  const selected = vehicles.find((v) => v.id === vehicleId) ?? null;

  return (
    <Modal visible={visible} animationType="slide" onRequestClose={onCancel}>
      <View style={styles.root}>
        <AuthScroll contentContainerStyle={styles.scroll}>
          <Text style={styles.title}>{t("parkCar.title")}</Text>
          <View style={styles.introBox}>
            <Text style={styles.introText}>{t("parkCar.intro")}</Text>
          </View>

          <Text style={styles.label}>{t("parkCar.location")}</Text>
          <Text style={styles.locationValue}>
            {addressLabel?.trim() ||
              (coords
                ? `${coords[1].toFixed(5)}, ${coords[0].toFixed(5)}`
                : t("parkCar.locationMissing"))}
          </Text>
          <Pressable style={styles.secondaryBtn} onPress={onPickOnMap} disabled={busy}>
            <Text style={styles.secondaryBtnText}>{t("parkCar.pickOnMap")}</Text>
          </Pressable>

          <Text style={styles.label}>{t("announce.vehicle")}</Text>
          {vehicles.length === 0 ? (
            <Pressable style={styles.primaryBtn} onPress={onAddVehicle} disabled={busy}>
              <Text style={styles.primaryBtnText}>{t("parkCar.addVehicle")}</Text>
            </Pressable>
          ) : (
            <>
              <Pressable
                style={styles.select}
                onPress={() => setVehicleOpen((o) => !o)}
                disabled={busy}
              >
                <Text style={styles.selectText}>
                    {selected
                    ? `${selected.plate} · ${selected.make_model}`
                    : t("announce.vehicle")}
                </Text>
              </Pressable>
              {vehicleOpen
                ? vehicles.map((v) => (
                    <Pressable
                      key={v.id}
                      style={styles.option}
                      onPress={() => {
                        setVehicleId(v.id);
                        setVehicleOpen(false);
                      }}
                    >
                      <Text style={styles.optionText}>
                        {v.plate} · {v.make_model}
                      </Text>
                    </Pressable>
                  ))
                : null}
            </>
          )}

          <Pressable
            style={[styles.primaryBtn, (!coords || !vehicleId || busy) && styles.disabled]}
            disabled={!coords || !vehicleId || busy}
            onPress={() => {
              if (!coords || !vehicleId) {
                return;
              }
              void onSubmit({
                vehicleId,
                lon: coords[0],
                lat: coords[1],
                addressHint: addressLabel,
              });
            }}
          >
            {busy ? (
              <ActivityIndicator color="#fff" />
            ) : (
              <Text style={styles.primaryBtnText}>{t("parkCar.save")}</Text>
            )}
          </Pressable>

          <Pressable style={styles.cancel} onPress={onCancel} disabled={busy}>
            <Text style={styles.cancelText}>{t("common.cancel")}</Text>
          </Pressable>
        </AuthScroll>
      </View>
    </Modal>
  );
}

const styles = StyleSheet.create({
  root: { flex: 1, backgroundColor: "#0B1F33" },
  scroll: { padding: 20, paddingTop: 48, gap: 12 },
  title: { color: "#fff", fontSize: 22, fontWeight: "700", marginBottom: 4 },
  introBox: {
    backgroundColor: "#1A73E822",
    borderColor: "#1A73E8",
    borderWidth: 1,
    borderRadius: 12,
    padding: 14,
    marginBottom: 8,
  },
  introText: { color: "#D6E4FF", fontSize: 14, lineHeight: 20 },
  label: { color: "#9BB0C5", fontSize: 13, marginTop: 8 },
  locationValue: { color: "#fff", fontSize: 15 },
  select: {
    backgroundColor: "#132A42",
    borderRadius: 10,
    padding: 14,
  },
  selectText: { color: "#fff", fontSize: 15 },
  option: {
    backgroundColor: "#0F2438",
    padding: 12,
    borderRadius: 8,
  },
  optionText: { color: "#CFE0F0", fontSize: 14 },
  primaryBtn: {
    marginTop: 16,
    backgroundColor: "#1A73E8",
    borderRadius: 12,
    paddingVertical: 14,
    alignItems: "center",
  },
  primaryBtnText: { color: "#fff", fontWeight: "700", fontSize: 16 },
  secondaryBtn: {
    alignSelf: "flex-start",
    paddingVertical: 8,
    paddingHorizontal: 4,
  },
  secondaryBtnText: { color: "#7EB6FF", fontSize: 14, fontWeight: "600" },
  cancel: { alignItems: "center", paddingVertical: 12 },
  cancelText: { color: "#9BB0C5", fontSize: 15 },
  disabled: { opacity: 0.45 },
});
