import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type Href, useRouter } from "expo-router";
import {
  ActivityIndicator,
  Alert,
  FlatList,
  Pressable,
  Text,
  View,
} from "react-native";

import { fetchMySpots, withdrawSpot, type SpotFeature } from "@/api/client";
import { accountStyles } from "@/account/theme";
import { useDevSession } from "@/hooks/useDevSession";

function spotTitle(spot: SpotFeature): string {
  const price = (spot.properties.price_cents / 100).toFixed(2);
  return `€${price} · ${spot.properties.status}`;
}

export default function MySpotsScreen() {
  const router = useRouter();
  const { ready } = useDevSession();
  const queryClient = useQueryClient();
  const spots = useQuery({
    queryKey: ["spots", "mine"],
    queryFn: fetchMySpots,
    enabled: ready,
  });

  const withdraw = useMutation({
    mutationFn: (id: string) => withdrawSpot(id),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["spots", "mine"] });
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

  return (
    <View style={accountStyles.screen}>
      <FlatList
        contentContainerStyle={accountStyles.scroll}
        data={spots.data?.features ?? []}
        keyExtractor={(item) => String(item.id)}
        ListEmptyComponent={
          <Text style={accountStyles.empty}>You have no published spots.</Text>
        }
        renderItem={({ item }) => {
          const id = String(item.id);
          const canEdit = item.properties.status === "available";
          return (
            <View
              style={[
                accountStyles.row,
                { marginBottom: 8, flexDirection: "column", alignItems: "stretch" },
              ]}
            >
              <Text style={accountStyles.rowTitle}>{spotTitle(item)}</Text>
              <Text style={accountStyles.rowMeta}>
                {item.properties.vehicle.plate} · {item.properties.vehicle.make_model}
              </Text>
              <Text style={accountStyles.rowMeta}>
                From {new Date(item.properties.available_from).toLocaleString()} · to{" "}
                {new Date(item.properties.expires_at).toLocaleString()}
              </Text>
              <View style={{ flexDirection: "row", gap: 8, marginTop: 10 }}>
                {canEdit ? (
                  <Pressable
                    style={[accountStyles.secondary, { flex: 1 }]}
                    onPress={() => router.push(`/account/spots/${id}` as Href)}
                  >
                    <Text style={accountStyles.secondaryText}>Edit</Text>
                  </Pressable>
                ) : null}
                <Pressable
                  style={[accountStyles.danger, { flex: 1 }]}
                  disabled={withdraw.isPending}
                  onPress={() => {
                    Alert.alert(
                      "Withdraw spot?",
                      "This removes the offer from the map.",
                      [
                        { text: "Cancel", style: "cancel" },
                        {
                          text: "Withdraw",
                          style: "destructive",
                          onPress: () => withdraw.mutate(id),
                        },
                      ],
                    );
                  }}
                >
                  <Text style={accountStyles.dangerText}>Withdraw</Text>
                </Pressable>
              </View>
            </View>
          );
        }}
        refreshing={spots.isFetching}
        onRefresh={() => void spots.refetch()}
      />
      {spots.error ? (
        <Text style={[accountStyles.error, { padding: 20 }]}>
          {spots.error instanceof Error ? spots.error.message : "Failed to load"}
        </Text>
      ) : null}
    </View>
  );
}
