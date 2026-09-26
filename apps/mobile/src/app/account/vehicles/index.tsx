import { useQuery } from "@tanstack/react-query";
import { Ionicons } from "@expo/vector-icons";
import { type Href, useRouter } from "expo-router";
import {
  ActivityIndicator,
  FlatList,
  Pressable,
  Text,
  View,
} from "react-native";

import { fetchMySpots, listVehicles, type SpotFeature } from "@/api/client";
import { accountStyles } from "@/account/theme";
import { useSession } from "@/hooks/useSession";
import { useTranslation } from "@/i18n";
import { sizeClassLabel } from "@/i18n/catalogLabels";
import { isLiveMapSpotStatus } from "@/map/liveMapSpot";

function parkedSpotForVehicle(
  vehicleId: string,
  spots: SpotFeature[] | undefined,
): SpotFeature | null {
  for (const spot of spots ?? []) {
    if (!isLiveMapSpotStatus(spot.properties.status)) {
      continue;
    }
    if (spot.properties.vehicle?.id === vehicleId) {
      return spot;
    }
  }
  return null;
}

export default function VehiclesListScreen() {
  const { t } = useTranslation();
  const router = useRouter();
  const { signedIn } = useSession();
  const vehicles = useQuery({
    queryKey: ["vehicles"],
    queryFn: listVehicles,
    enabled: signedIn,
  });
  const mySpots = useQuery({
    queryKey: ["spots", "mine"],
    queryFn: fetchMySpots,
    enabled: signedIn,
  });

  if (!signedIn || vehicles.isLoading) {
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
            <Text style={accountStyles.primaryText}>{t("account.vehicles.add")}</Text>
          </Pressable>
        }
        ListEmptyComponent={
          <Text style={accountStyles.empty}>{t("account.vehicles.empty")}</Text>
        }
        renderItem={({ item }) => {
          const parked = parkedSpotForVehicle(item.id, mySpots.data?.features);
          const lon = parked?.geometry.coordinates[0];
          const lat = parked?.geometry.coordinates[1];
          return (
            <Pressable
              style={[accountStyles.row, { marginBottom: 8, flexDirection: "row", gap: 8 }]}
              onPress={() => router.push(`/account/vehicles/${item.id}` as Href)}
            >
              <View style={{ flex: 1 }}>
                <Text style={accountStyles.rowTitle}>
                  {item.plate} · {item.make_model}
                </Text>
                <Text style={accountStyles.rowMeta}>
                  {item.color} · {item.year} · {sizeClassLabel(t, item.size_class)}
                  {item.has_photo ? t("account.vehicles.hasPhoto") : ""}
                </Text>
              </View>
              {parked && lon != null && lat != null ? (
                <Pressable
                  accessibilityRole="button"
                  accessibilityLabel={t("account.vehicles.showOnMap")}
                  hitSlop={8}
                  onPress={(e) => {
                    e.stopPropagation?.();
                    const q = new URLSearchParams({
                      focusLon: String(lon),
                      focusLat: String(lat),
                      focusSpot: String(parked.id),
                    });
                    router.replace(`/?${q.toString()}` as Href);
                  }}
                  style={{
                    width: 36,
                    height: 36,
                    borderRadius: 18,
                    backgroundColor: "#16324F",
                    alignItems: "center",
                    justifyContent: "center",
                  }}
                >
                  <Ionicons name="locate" size={20} color="#F4F7FA" />
                </Pressable>
              ) : null}
              <Text style={accountStyles.link}>{t("account.vehicles.edit")}</Text>
            </Pressable>
          );
        }}
        refreshing={vehicles.isFetching || mySpots.isFetching}
        onRefresh={() => {
          void vehicles.refetch();
          void mySpots.refetch();
        }}
      />
      {vehicles.error ? (
        <Text style={[accountStyles.error, { padding: 20 }]}>
          {vehicles.error instanceof Error
            ? vehicles.error.message
            : t("account.vehicles.loadFailed")}
        </Text>
      ) : null}
    </View>
  );
}
