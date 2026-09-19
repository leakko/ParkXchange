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
import { useSession } from "@/hooks/useSession";
import { useTranslation } from "@/i18n";

function spotTitle(
  spot: SpotFeature,
  t: (key: "account.spots.spotTitle", params: Record<string, string | number>) => string,
): string {
  const price = (spot.properties.price_cents / 100).toFixed(2);
  return t("account.spots.spotTitle", { price, status: spot.properties.status });
}

export default function MySpotsScreen() {
  const { t, formatDateTime } = useTranslation();
  const router = useRouter();
  const { signedIn } = useSession();
  const queryClient = useQueryClient();
  const spots = useQuery({
    queryKey: ["spots", "mine"],
    queryFn: fetchMySpots,
    enabled: signedIn,
  });

  const withdraw = useMutation({
    mutationFn: (id: string) => withdrawSpot(id),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["spots", "mine"] });
    },
    onError: (err) => {
      Alert.alert(
        t("account.spots.withdrawFailed.title"),
        err instanceof Error ? err.message : t("common.error"),
      );
    },
  });

  if (!signedIn || spots.isLoading) {
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
          <Text style={accountStyles.empty}>{t("account.spots.empty")}</Text>
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
              <Text style={accountStyles.rowTitle}>{spotTitle(item, t)}</Text>
              <Text style={accountStyles.rowMeta}>
                {item.properties.vehicle
                  ? `${item.properties.vehicle.plate} · ${item.properties.vehicle.make_model}`
                  : item.properties.size_class}
              </Text>
              <Text style={accountStyles.rowMeta}>
                {t("account.spots.listedUntil", {
                  datetime: formatDateTime(item.properties.listed_until),
                })}
              </Text>
              <View style={{ flexDirection: "row", gap: 8, marginTop: 10 }}>
                {canEdit ? (
                  <Pressable
                    style={[accountStyles.secondary, { flex: 1 }]}
                    onPress={() => router.push(`/account/spots/${id}` as Href)}
                  >
                    <Text style={accountStyles.secondaryText}>{t("account.spots.edit")}</Text>
                  </Pressable>
                ) : null}
                <Pressable
                  style={[accountStyles.danger, { flex: 1 }]}
                  disabled={withdraw.isPending}
                  onPress={() => {
                    Alert.alert(
                      t("account.spots.withdraw.confirmTitle"),
                      t("account.spots.withdraw.confirmMessage"),
                      [
                        { text: t("common.cancel"), style: "cancel" },
                        {
                          text: t("account.spots.withdraw.action"),
                          style: "destructive",
                          onPress: () => withdraw.mutate(id),
                        },
                      ],
                    );
                  }}
                >
                  <Text style={accountStyles.dangerText}>
                    {t("account.spots.withdraw.action")}
                  </Text>
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
          {spots.error instanceof Error ? spots.error.message : t("account.spots.loadFailed")}
        </Text>
      ) : null}
    </View>
  );
}
