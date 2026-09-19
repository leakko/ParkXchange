import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useLocalSearchParams, useRouter } from "expo-router";
import { useEffect, useState } from "react";
import {
  ActivityIndicator,
  Alert,
  Pressable,
  ScrollView,
  Text,
  TextInput,
  View,
} from "react-native";

import {
  fetchMySpots,
  listVehicles,
  type SpotFeature,
  updateSpot,
  withdrawSpot,
} from "@/api/client";
import { accountStyles } from "@/account/theme";
import { useDevSession } from "@/hooks/useDevSession";

/** Remaining offer window relative to now, for form defaults only. */
function remainingWindow(spot: SpotFeature, now = Date.now()) {
  const expires = new Date(spot.properties.listed_until).getTime();
  const availableIn = 0;
  const start = now;
  const duration = Math.max(1, Math.ceil((expires - start) / 60_000));
  return {
    availableInMinutes: String(availableIn),
    durationMinutes: String(duration),
  };
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

  const spot = spots.data?.features.find((f) => String(f.id) === id);

  const [price, setPrice] = useState("");
  const [notes, setNotes] = useState("");
  const [vehicleId, setVehicleId] = useState("");
  const [durationMinutes, setDurationMinutes] = useState("");
  const [availableInMinutes, setAvailableInMinutes] = useState("");
  const [windowBaseline, setWindowBaseline] = useState<{
    durationMinutes: string;
    availableInMinutes: string;
  } | null>(null);

  useEffect(() => {
    if (!spot) {
      return;
    }
    setPrice((spot.properties.price_cents / 100).toFixed(2));
    setNotes(spot.properties.notes ?? "");
    setVehicleId(spot.properties.vehicle.id);
    const window = remainingWindow(spot);
    setDurationMinutes(window.durationMinutes);
    setAvailableInMinutes(window.availableInMinutes);
    setWindowBaseline(window);
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
      };
      const windowChanged =
        windowBaseline != null &&
        (durationMinutes !== windowBaseline.durationMinutes ||
          availableInMinutes !== windowBaseline.availableInMinutes);
      if (windowChanged) {
        const duration = Number.parseInt(durationMinutes, 10);
        const delay = Number.parseInt(availableInMinutes, 10);
        if (!Number.isFinite(duration) || duration <= 0) {
          throw new Error("Duration must be a positive number of minutes");
        }
        if (!Number.isFinite(delay) || delay < 0) {
          throw new Error("Available-in must be zero or more minutes");
        }
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
        <Text style={accountStyles.label}>Price (€)</Text>
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
      <View style={accountStyles.field}>
        <Text style={accountStyles.label}>Available in (minutes)</Text>
        <TextInput
          style={accountStyles.input}
          value={availableInMinutes}
          onChangeText={setAvailableInMinutes}
          keyboardType="number-pad"
          placeholderTextColor="#7A93A0"
        />
      </View>
      <View style={accountStyles.field}>
        <Text style={accountStyles.label}>Duration (minutes)</Text>
        <TextInput
          style={accountStyles.input}
          value={durationMinutes}
          onChangeText={setDurationMinutes}
          keyboardType="number-pad"
          placeholderTextColor="#7A93A0"
        />
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
