import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useLocalSearchParams, useRouter } from "expo-router";
import { useEffect, useState } from "react";
import {
  ActivityIndicator,
  Alert,
  Pressable,
  ScrollView,
  Switch,
  Text,
  TextInput,
  View,
} from "react-native";

import {
  acceptOffer,
  fetchMySpots,
  listOffers,
  listVehicles,
  rejectOffer,
  type SpotFeature,
  updateSpot,
  withdrawSpot,
} from "@/api/client";
import { accountStyles } from "@/account/theme";
import { useDevSession } from "@/hooks/useDevSession";
import { matchesPreferredMinute } from "@/map/exchange";

function localDateTimeInput(value: string): string {
  const date = new Date(value);
  const offset = date.getTimezoneOffset() * 60_000;
  return new Date(date.getTime() - offset).toISOString().slice(0, 16);
}

export default function EditSpotScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const router = useRouter();
  const { ready } = useDevSession();
  const queryClient = useQueryClient();

  const spots = useQuery({
    queryKey: ["spots", "mine"],
    queryFn: fetchMySpots,
    enabled: ready,
  });
  const vehicles = useQuery({
    queryKey: ["vehicles"],
    queryFn: listVehicles,
    enabled: ready,
  });
  const offers = useQuery({
    queryKey: ["offers", id],
    queryFn: () => listOffers(id),
    enabled: ready && !!id,
  });

  const spot = spots.data?.features.find((f) => String(f.id) === id);

  const [price, setPrice] = useState("");
  const [notes, setNotes] = useState("");
  const [vehicleId, setVehicleId] = useState("");
  const [hasPreferredTime, setHasPreferredTime] = useState(false);
  const [preferredTime, setPreferredTime] = useState("");
  const [autoCancel, setAutoCancel] = useState(true);

  useEffect(() => {
    if (!spot) {
      return;
    }
    setPrice((spot.properties.price_cents / 100).toFixed(2));
    setNotes(spot.properties.notes ?? "");
    setVehicleId(spot.properties.vehicle.id);
    setHasPreferredTime(!!spot.properties.preferred_departure_at);
    setPreferredTime(
      spot.properties.preferred_departure_at
        ? localDateTimeInput(spot.properties.preferred_departure_at)
        : localDateTimeInput(new Date(Date.now() + 60 * 60 * 1000).toISOString()),
    );
    setAutoCancel(spot.properties.auto_cancel_no_show);
  }, [spot]);

  const save = useMutation({
    mutationFn: async () => {
      if (!id) {
        throw new Error("Missing spot id");
      }
      const euros = Number.parseFloat(price);
      if (!Number.isFinite(euros) || euros < 0) {
        throw new Error("Enter a valid price");
      }
      const body: Parameters<typeof updateSpot>[1] = {
        price_cents: Math.round(euros * 100),
        auto_cancel_no_show: autoCancel,
      };
      if (hasPreferredTime) {
        const preferred = new Date(preferredTime);
        if (!Number.isFinite(preferred.getTime())) {
          throw new Error("Enter a valid preferred departure date and time");
        }
        body.preferred_departure_at = preferred.toISOString();
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
      Alert.alert("Saved", "Spot updated.", [
        { text: "OK", onPress: () => router.back() },
      ]);
    },
    onError: (err) => {
      Alert.alert("Save failed", err instanceof Error ? err.message : "error");
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
        Alert.alert("Oferta aceptada", "El intercambio ya está reservado.");
      }
    },
    onError: (err) => {
      Alert.alert("No se pudo actualizar", err instanceof Error ? err.message : "error");
    },
  });

  const withdraw = useMutation({
    mutationFn: async () => {
      if (!id) {
        throw new Error("Missing spot id");
      }
      await withdrawSpot(id);
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["spots", "mine"] });
      router.back();
    },
    onError: (err) => {
      Alert.alert("Withdraw failed", err instanceof Error ? err.message : "error");
    },
  });

  if (!ready || spots.isLoading) {
    return (
      <View style={[accountStyles.screen, { justifyContent: "center", alignItems: "center" }]}>
        <ActivityIndicator color="#F4F7FA" />
      </View>
    );
  }

  if (!spot) {
    return (
      <View style={[accountStyles.screen, accountStyles.scroll]}>
        <Text style={accountStyles.error}>Spot not found</Text>
      </View>
    );
  }

  if (spot.properties.status !== "available") {
    return (
      <View style={[accountStyles.screen, accountStyles.scroll]}>
        <Text style={accountStyles.meta}>
          Only available spots can be edited (status: {spot.properties.status}).
        </Text>
        <Pressable
          style={[accountStyles.danger, { marginTop: 16 }]}
          onPress={() => {
            Alert.alert("Withdraw spot?", "This removes the offer from the map.", [
              { text: "Cancel", style: "cancel" },
              {
                text: "Withdraw",
                style: "destructive",
                onPress: () => withdraw.mutate(),
              },
            ]);
          }}
        >
          <Text style={accountStyles.dangerText}>Withdraw</Text>
        </Pressable>
      </View>
    );
  }

  return (
    <ScrollView
      style={accountStyles.screen}
      contentContainerStyle={accountStyles.scroll}
      keyboardShouldPersistTaps="handled"
    >
      <Text style={accountStyles.meta}>
        Location and space size are fixed. Listed until{" "}
        {new Date(spot.properties.listed_until).toLocaleString()}
      </Text>

      <View style={accountStyles.field}>
        <Text style={accountStyles.label}>Precio orientativo (€)</Text>
        <TextInput
          style={accountStyles.input}
          value={price}
          onChangeText={setPrice}
          keyboardType="decimal-pad"
          placeholderTextColor="#7A93A0"
        />
      </View>
      <View style={accountStyles.field}>
        <Text style={accountStyles.label}>Notes</Text>
        <TextInput
          style={[accountStyles.input, { minHeight: 72, textAlignVertical: "top" }]}
          value={notes}
          onChangeText={setNotes}
          multiline
          placeholderTextColor="#7A93A0"
        />
      </View>
      <View style={[accountStyles.row, { marginBottom: 12 }]}>
        <Text style={accountStyles.rowTitle}>Hora de salida preferida</Text>
        <Switch value={hasPreferredTime} onValueChange={setHasPreferredTime} />
      </View>
      {hasPreferredTime ? (
        <View style={accountStyles.field}>
          <TextInput
            style={accountStyles.input}
            value={preferredTime}
            onChangeText={setPreferredTime}
            placeholder="2026-09-19T18:00"
            placeholderTextColor="#7A93A0"
            autoCapitalize="none"
          />
        </View>
      ) : null}
      <View style={[accountStyles.row, { marginBottom: 12 }]}>
        <View style={{ flex: 1 }}>
          <Text style={accountStyles.rowTitle}>Auto-cancelar ausencia</Text>
          <Text style={accountStyles.rowMeta}>Después del margen de cortesía.</Text>
        </View>
        <Switch value={autoCancel} onValueChange={setAutoCancel} />
      </View>

      <View style={accountStyles.field}>
        <Text style={accountStyles.label}>Vehicle</Text>
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
              {active ? <Text style={accountStyles.link}>Selected</Text> : null}
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
          <Text style={accountStyles.primaryText}>Save spot</Text>
        )}
      </Pressable>

      <View style={accountStyles.section}>
        <Text style={accountStyles.sectionTitle}>Ofertas pendientes</Text>
        {(offers.data ?? [])
          .filter((offer) => offer.status === "pending")
          .map((offer) => {
            const preferred = matchesPreferredMinute(
              offer.exchange_at,
              spot.properties.preferred_departure_at,
            );
            return (
              <View key={offer.id} style={accountStyles.row}>
                <View style={{ flex: 1 }}>
                  <Text style={accountStyles.rowTitle}>
                    €{(offer.amount_cents / 100).toFixed(2)} ·{" "}
                    {new Date(offer.exchange_at).toLocaleString()}
                  </Text>
                  <Text style={accountStyles.rowMeta}>
                    {preferred ? "a tu hora" : "otra hora"}
                  </Text>
                </View>
                <View style={{ gap: 6 }}>
                  <Pressable
                    style={[accountStyles.primary, { paddingHorizontal: 12 }]}
                    disabled={decideOffer.isPending}
                    onPress={() => decideOffer.mutate({ offerId: offer.id, accept: true })}
                  >
                    <Text style={accountStyles.primaryText}>Aceptar</Text>
                  </Pressable>
                  <Pressable
                    style={[accountStyles.danger, { paddingHorizontal: 12 }]}
                    disabled={decideOffer.isPending}
                    onPress={() => decideOffer.mutate({ offerId: offer.id, accept: false })}
                  >
                    <Text style={accountStyles.dangerText}>Rechazar</Text>
                  </Pressable>
                </View>
              </View>
            );
          })}
        {offers.isLoading ? <ActivityIndicator color="#F4F7FA" /> : null}
        {!offers.isLoading &&
        !(offers.data ?? []).some((offer) => offer.status === "pending") ? (
          <Text style={accountStyles.empty}>No hay ofertas pendientes.</Text>
        ) : null}
      </View>

      <Pressable
        style={[accountStyles.danger, { marginTop: 8 }]}
        disabled={withdraw.isPending}
        onPress={() => {
          Alert.alert("Withdraw spot?", "This removes the offer from the map.", [
            { text: "Cancel", style: "cancel" },
            {
              text: "Withdraw",
              style: "destructive",
              onPress: () => withdraw.mutate(),
            },
          ]);
        }}
      >
        {withdraw.isPending ? (
          <ActivityIndicator color="#FF8FAB" />
        ) : (
          <Text style={accountStyles.dangerText}>Withdraw</Text>
        )}
      </Pressable>
    </ScrollView>
  );
}
