import BottomSheet, { BottomSheetScrollView } from "@gorhom/bottom-sheet";
import { forwardRef, useEffect, useMemo, useState } from "react";
import {
  ActivityIndicator,
  Image,
  Pressable,
  StyleSheet,
  Text,
  TextInput,
  View,
} from "react-native";

import type {
  ReservationResponse,
  SpotFeature,
  VehicleResponse,
} from "@/api/client";
import { spotVehiclePhotoUrl } from "@/api/client";
import { useAuthImage } from "@/hooks/useAuthImage";
import { openNavigation } from "@/lib/navigation";

type Props = {
  spot: SpotFeature | null;
  active: ReservationResponse | null;
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
  onOwnerReady: () => void;
  onDriverArrived: () => void;
  onDriverReady: () => void;
  onCancel: () => void;
  onEdit: (spot: SpotFeature) => void;
  onWithdraw: (spot: SpotFeature) => void;
};

function localDateTimeInput(value: Date): string {
  const offset = value.getTimezoneOffset() * 60_000;
  return new Date(value.getTime() - offset).toISOString().slice(0, 16);
}

export const SpotSheet = forwardRef<BottomSheet, Props>(function SpotSheet(
  {
    spot,
    active,
    vehicles,
    isOwner,
    isDriver,
    busy,
    onMakeOffer,
    onOwnerReady,
    onDriverArrived,
    onDriverReady,
    onCancel,
    onEdit,
    onWithdraw,
  },
  ref,
) {
  const snapPoints = useMemo(() => ["36%", "82%"], []);
  const [makingOffer, setMakingOffer] = useState(false);
  const [vehicleId, setVehicleId] = useState("");
  const [exchangeAt, setExchangeAt] = useState("");
  const [amount, setAmount] = useState("");
  const price = spot ? (spot.properties.price_cents / 100).toFixed(2) : "";
  const coords = spot?.geometry.coordinates;
  const isActiveForSpot =
    !!active && !!spot && String(active.spot_id) === String(spot.id);
  const vehicle = spot?.properties.vehicle;
  const photoUrl =
    vehicle?.has_photo && spot?.id
      ? spotVehiclePhotoUrl(String(spot.id))
      : null;
  const { uri: photoUri } = useAuthImage(photoUrl);

  useEffect(() => {
    setMakingOffer(false);
    setVehicleId(vehicles[0]?.id ?? "");
    setAmount(price);
    const suggested = spot?.properties.preferred_departure_at
      ? new Date(spot.properties.preferred_departure_at)
      : new Date(Date.now() + 60 * 60 * 1000);
    setExchangeAt(localDateTimeInput(suggested));
  }, [price, spot, vehicles]);

  const submitOffer = async () => {
    if (!spot || !vehicleId) {
      return;
    }
    const parsedDate = new Date(exchangeAt);
    const euros = Number.parseFloat(amount);
    if (!Number.isFinite(parsedDate.getTime()) || !Number.isFinite(euros) || euros < 0) {
      return;
    }
    await onMakeOffer(spot, vehicleId, parsedDate.toISOString(), Math.round(euros * 100));
    setMakingOffer(false);
  };

  return (
    <BottomSheet
      ref={ref}
      index={-1}
      snapPoints={snapPoints}
      enablePanDownToClose
      backgroundStyle={styles.sheet}
      handleIndicatorStyle={styles.handle}
    >
      <BottomSheetScrollView contentContainerStyle={styles.body}>
        {spot ? (
          <>
            <Text style={styles.title}>{spot.properties.owner_name}</Text>
            {spot.properties.is_mine ? (
              <Text style={styles.mineBadge}>Your listing</Text>
            ) : null}
            <Text style={styles.meta}>
              {spot.properties.size_class} · €{price} · {spot.properties.status}
            </Text>
            {spot.properties.address_hint ? (
              <Text style={styles.hint}>{spot.properties.address_hint}</Text>
            ) : null}
            {spot.properties.notes ? (
              <Text style={styles.notes}>{spot.properties.notes}</Text>
            ) : null}

            {vehicle && (vehicle.plate || vehicle.make_model) ? (
              <View style={styles.vehicleBlock}>
                <Text style={styles.vehicleTitle}>
                  {vehicle.plate}
                  {vehicle.make_model ? ` · ${vehicle.make_model}` : ""}
                </Text>
                <Text style={styles.vehicleMeta}>
                  {[vehicle.color, vehicle.year || null, vehicle.size_class]
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

            <Text style={styles.window}>
              {spot.properties.preferred_departure_at
                ? `Preferred ${new Date(spot.properties.preferred_departure_at).toLocaleString()}`
                : "Flexible departure time"}{" "}
              · listed until {new Date(spot.properties.listed_until).toLocaleString()}
            </Text>

            <View style={styles.actions}>
              {spot.properties.is_mine ? (
                <>
                  <Pressable
                    style={styles.primary}
                    disabled={busy}
                    onPress={() => onEdit(spot)}
                  >
                    <Text style={styles.primaryText}>Edit</Text>
                  </Pressable>
                  <Pressable
                    style={styles.danger}
                    disabled={busy}
                    onPress={() => onWithdraw(spot)}
                  >
                    <Text style={styles.dangerText}>Withdraw</Text>
                  </Pressable>
                </>
              ) : null}

              {!spot.properties.is_mine &&
              !isActiveForSpot &&
              spot.properties.status === "available" ? (
                makingOffer ? (
                  <View style={styles.offerForm}>
                    <Text style={styles.formLabel}>Tu vehículo</Text>
                    {vehicles.map((candidate) => (
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
                    ))}
                    <Text style={styles.formLabel}>Fecha y hora del intercambio</Text>
                    <TextInput
                      style={styles.input}
                      value={exchangeAt}
                      onChangeText={setExchangeAt}
                      placeholder="2026-09-19T18:00"
                      placeholderTextColor="#7A93A0"
                      autoCapitalize="none"
                    />
                    <Text style={styles.formLabel}>Oferta (€)</Text>
                    <TextInput
                      style={styles.input}
                      value={amount}
                      onChangeText={setAmount}
                      keyboardType="decimal-pad"
                      placeholderTextColor="#7A93A0"
                    />
                    <Pressable
                      style={styles.primary}
                      disabled={busy || !vehicleId}
                      onPress={() => void submitOffer()}
                    >
                      {busy ? (
                        <ActivityIndicator color="#fff" />
                      ) : (
                        <Text style={styles.primaryText}>Enviar oferta</Text>
                      )}
                    </Pressable>
                    <Pressable onPress={() => setMakingOffer(false)}>
                      <Text style={styles.cancelText}>Cancelar</Text>
                    </Pressable>
                  </View>
                ) : (
                  <Pressable
                    style={styles.primary}
                    disabled={busy}
                    onPress={() => setMakingOffer(true)}
                  >
                    <Text style={styles.primaryText}>Hacer oferta</Text>
                  </Pressable>
                )
              ) : null}

              {isActiveForSpot ? (
                <>
                  <Text style={styles.exchangeTime}>
                    Intercambio: {new Date(active.exchange_at).toLocaleString()}
                  </Text>
                  {isOwner && !active.owner_ready_at ? (
                    <Pressable style={styles.primary} disabled={busy} onPress={onOwnerReady}>
                      <Text style={styles.primaryText}>Listo para salir</Text>
                    </Pressable>
                  ) : null}
                  {isDriver && !active.driver_arrived_at ? (
                    <Pressable
                      style={styles.secondary}
                      disabled={busy}
                      onPress={onDriverArrived}
                    >
                      <Text style={styles.secondaryText}>He llegado</Text>
                    </Pressable>
                  ) : null}
                  {isDriver &&
                  !!active.owner_ready_at &&
                  !!active.driver_arrived_at &&
                  !active.driver_ready_at ? (
                    <Pressable style={styles.primary} disabled={busy} onPress={onDriverReady}>
                      <Text style={styles.primaryText}>Listo</Text>
                    </Pressable>
                  ) : null}
                  <Pressable
                    style={styles.danger}
                    disabled={busy}
                    onPress={onCancel}
                  >
                    <Text style={styles.dangerText}>Cancelar intercambio</Text>
                  </Pressable>
                </>
              ) : null}

              {coords && coords[0] != null && coords[1] != null ? (
                <Pressable
                  style={styles.secondary}
                  onPress={() =>
                    void openNavigation({ lon: coords[0]!, lat: coords[1]! })
                  }
                >
                  <Text style={styles.secondaryText}>Navigate</Text>
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
  sheet: { backgroundColor: "#0B1F33" },
  handle: { backgroundColor: "#5B7A8C" },
  body: { paddingHorizontal: 20, paddingBottom: 28, gap: 6 },
  title: { color: "#F4F7FA", fontSize: 18, fontWeight: "600" },
  mineBadge: { color: "#1B9AAA", fontSize: 13, fontWeight: "600" },
  meta: { color: "#9DB4C0", fontSize: 14 },
  hint: { color: "#D6E2E9", fontSize: 14, marginTop: 4 },
  notes: { color: "#D6E2E9", fontSize: 14 },
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
  window: { color: "#7A93A0", fontSize: 12, marginTop: 8 },
  actions: { marginTop: 14, gap: 8 },
  offerForm: { gap: 8 },
  formLabel: { color: "#9DB4C0", fontSize: 12, marginTop: 4 },
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
