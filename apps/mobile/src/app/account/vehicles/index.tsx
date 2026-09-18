import { useQuery } from "@tanstack/react-query";
import { type Href, useRouter } from "expo-router";
import {
  ActivityIndicator,
  FlatList,
  Pressable,
  Text,
  View,
} from "react-native";

import { listVehicles } from "@/api/client";
import { accountStyles } from "@/account/theme";
import { useDevSession } from "@/hooks/useDevSession";

export default function VehiclesListScreen() {
  const router = useRouter();
  const { ready } = useDevSession();
  const vehicles = useQuery({
    queryKey: ["vehicles"],
    queryFn: listVehicles,
    enabled: ready,
  });

  if (!ready || vehicles.isLoading) {
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
        data={vehicles.data ?? []}
        keyExtractor={(item) => item.id}
        ListHeaderComponent={
          <Pressable
            style={[accountStyles.primary, { marginBottom: 8 }]}
            onPress={() => router.push("/account/vehicles/new" as Href)}
          >
            <Text style={accountStyles.primaryText}>Add vehicle</Text>
          </Pressable>
        }
        ListEmptyComponent={
          <Text style={accountStyles.empty}>No vehicles yet. Add one to announce spots.</Text>
        }
        renderItem={({ item }) => (
          <Pressable
            style={[accountStyles.row, { marginBottom: 8 }]}
            onPress={() => router.push(`/account/vehicles/${item.id}` as Href)}
          >
            <View style={{ flex: 1 }}>
              <Text style={accountStyles.rowTitle}>
                {item.plate} · {item.make_model}
              </Text>
              <Text style={accountStyles.rowMeta}>
                {item.color} · {item.year} · {item.size_class}
                {item.has_photo ? " · photo" : ""}
              </Text>
            </View>
            <Text style={accountStyles.link}>Edit</Text>
          </Pressable>
        )}
        refreshing={vehicles.isFetching}
        onRefresh={() => void vehicles.refetch()}
      />
      {vehicles.error ? (
        <Text style={[accountStyles.error, { padding: 20 }]}>
          {vehicles.error instanceof Error ? vehicles.error.message : "Failed to load"}
        </Text>
      ) : null}
    </View>
  );
}
