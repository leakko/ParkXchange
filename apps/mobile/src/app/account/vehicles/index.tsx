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
import { useSession } from "@/hooks/useSession";
import { useTranslation } from "@/i18n";

export default function VehiclesListScreen() {
  const { t } = useTranslation();
  const router = useRouter();
  const { signedIn } = useSession();
  const vehicles = useQuery({
    queryKey: ["vehicles"],
    queryFn: listVehicles,
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
                {item.has_photo ? t("account.vehicles.hasPhoto") : ""}
              </Text>
            </View>
            <Text style={accountStyles.link}>{t("account.vehicles.edit")}</Text>
          </Pressable>
        )}
        refreshing={vehicles.isFetching}
        onRefresh={() => void vehicles.refetch()}
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
