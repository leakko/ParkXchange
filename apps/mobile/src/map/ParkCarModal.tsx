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
import { resolveAddressLabel } from "@/map/resolveAddressLabel";
import { useConfirm } from "@/ui/ConfirmModal";

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
  /** True when the seed came from the device GPS (FAB), not a map pick. */
  initialFromCurrentLocation?: boolean;
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
  initialFromCurrentLocation = false,
  onCancel,
  onPickOnMap,
  onAddVehicle,
  onSubmit,
}: Props) {
  const { t, locale } = useTranslation();
  const { alert } = useConfirm();
  const [vehicleId, setVehicleId] = useState<string | null>(null);
  const [vehicleOpen, setVehicleOpen] = useState(false);
  const [coords, setCoords] = useState<[number, number] | null>(null);
  /** Street label for storage / non-GPS display. */
  const [addressLabel, setAddressLabel] = useState<string | null>(null);
  const [resolvingLabel, setResolvingLabel] = useState(false);
  const [fromCurrentLocation, setFromCurrentLocation] = useState(false);
  const [locating, setLocating] = useState(false);

  const applyCoords = async (
    next: [number, number],
    isCurrent: boolean,
    labelHint: string | null,
  ) => {
    setCoords(next);
    setFromCurrentLocation(isCurrent);
    setResolvingLabel(true);
    try {
      const resolved = await resolveAddressLabel(next[0], next[1], labelHint, locale);
      setAddressLabel(resolved);
    } catch {
      setAddressLabel(labelHint);
    } finally {
      setResolvingLabel(false);
    }
  };

  useEffect(() => {
    if (!visible) {
      return;
    }
    setVehicleId((prev) => {
      if (prev && vehicles.some((v) => v.id === prev)) {
        return prev;
      }
      return vehicles[0]?.id ?? null;
    });
    let cancelled = false;
    const seed = async () => {
      if (!initialCoordinates) {
        setCoords(null);
        setAddressLabel(initialAddressLabel);
        setFromCurrentLocation(false);
        return;
      }
      if (cancelled) {
        return;
      }
      await applyCoords(
        initialCoordinates,
        initialFromCurrentLocation,
        initialAddressLabel,
      );
    };
    void seed();
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps -- seed when modal opens / map pick returns
  }, [visible, initialCoordinates, initialAddressLabel, initialFromCurrentLocation, vehicles, locale]);

  const useMyLocation = async () => {
    if (fromCurrentLocation || locating || busy) {
      return;
    }
    setLocating(true);
    try {
      const { currentLatLon } = await import("@/push/locationSeed");
      const here = await currentLatLon();
      if (!here) {
        await alert({
          title: t("map.alert.parkFailed.title"),
          message: t("exchange.locationPermissionRequired"),
          confirmLabel: t("common.ok"),
        });
        return;
      }
      await applyCoords([here.longitude, here.latitude], true, null);
    } finally {
      setLocating(false);
    }
  };

  const selected = vehicles.find((v) => v.id === vehicleId) ?? null;

  const locationDisplay = (() => {
    if (!coords) {
      return t("parkCar.locationMissing");
    }
    if (fromCurrentLocation) {
      return t("parkCar.currentLocation");
    }
    if (resolvingLabel && !addressLabel?.trim()) {
      return t("parkCar.locationResolving");
    }
    return addressLabel?.trim() || t("parkCar.locationMissing");
  })();

  return (
    <Modal visible={visible} animationType="slide" onRequestClose={onCancel}>
      <View style={styles.root}>
        <AuthScroll contentContainerStyle={styles.scroll}>
          <Text style={styles.title}>{t("parkCar.title")}</Text>
          <View style={styles.introBox}>
            <Text style={styles.introText}>{t("parkCar.intro")}</Text>
          </View>

          <Text style={styles.label}>{t("parkCar.location")}</Text>
          <Text style={styles.locationValue}>{locationDisplay}</Text>
          <View style={styles.rowBtns}>
            <Pressable
              style={[styles.chipBtn, { flex: 1 }]}
              onPress={onPickOnMap}
              disabled={busy || locating}
            >
              <Text style={styles.chipBtnText}>{t("parkCar.pickOnMap")}</Text>
            </Pressable>
            <Pressable
              style={[
                styles.chipBtn,
                { flex: 1 },
                (fromCurrentLocation || locating || busy) && styles.disabled,
              ]}
              onPress={() => void useMyLocation()}
              disabled={fromCurrentLocation || locating || busy}
            >
              {locating ? (
                <ActivityIndicator color="#F4F7FA" />
              ) : (
                <Text style={styles.chipBtnText}>{t("parkCar.useMyLocation")}</Text>
              )}
            </Pressable>
          </View>

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
  rowBtns: { flexDirection: "row", gap: 8, marginTop: 4 },
  chipBtn: {
    backgroundColor: "#16324F",
    borderRadius: 12,
    paddingVertical: 12,
    paddingHorizontal: 10,
    alignItems: "center",
    justifyContent: "center",
    minHeight: 44,
  },
  chipBtnText: { color: "#F4F7FA", fontWeight: "600", fontSize: 13, textAlign: "center" },
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
  cancel: { alignItems: "center", paddingVertical: 12 },
  cancelText: { color: "#9BB0C5", fontSize: 15 },
  disabled: { opacity: 0.45 },
});
