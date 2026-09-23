import { useQuery } from "@tanstack/react-query";
import { Stack, useLocalSearchParams } from "expo-router";
import { ActivityIndicator, FlatList, StyleSheet, Text, View } from "react-native";

import { getUserProfile } from "@/api/client";
import { accountColors, accountStyles } from "@/account/theme";
import { useTranslation } from "@/i18n";

export default function UserProfileScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const { t } = useTranslation();
  const profile = useQuery({
    queryKey: ["user-profile", id],
    queryFn: () => getUserProfile(String(id)),
    enabled: !!id,
  });

  return (
    <View style={accountStyles.screen}>
      <Stack.Screen options={{ title: t("profile.public.title") }} />
      {profile.isLoading ? (
        <ActivityIndicator color={accountColors.accent} style={{ marginTop: 40 }} />
      ) : profile.isError || !profile.data ? (
        <Text style={accountStyles.meta}>{t("profile.public.missing")}</Text>
      ) : (
        <FlatList
          contentContainerStyle={styles.list}
          ListHeaderComponent={
            <View style={styles.header}>
              <Text style={accountStyles.title}>{profile.data.display_name}</Text>
              <Text style={accountStyles.meta}>
                {profile.data.rating != null
                  ? t("account.rating.withScore", {
                      score: profile.data.rating.toFixed(1),
                      count: profile.data.rating_count,
                    })
                  : t("profile.public.noRatings")}
              </Text>
            </View>
          }
          data={profile.data.reviews}
          keyExtractor={(item, i) => `${item.created_at}-${i}`}
          ListEmptyComponent={
            <Text style={accountStyles.meta}>{t("profile.public.emptyReviews")}</Text>
          }
          renderItem={({ item }) => (
            <View style={styles.review}>
              <Text style={styles.reviewTitle}>
                {"★".repeat(item.stars)}
                {"☆".repeat(5 - item.stars)} · {item.rater_name}
              </Text>
              {item.comment ? (
                <Text style={accountStyles.meta}>{item.comment}</Text>
              ) : null}
              <Text style={styles.date}>
                {new Date(item.created_at).toLocaleDateString()}
              </Text>
            </View>
          )}
        />
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  list: { padding: 16, gap: 12, paddingBottom: 40 },
  header: { gap: 6, marginBottom: 12 },
  review: {
    borderTopWidth: StyleSheet.hairlineWidth,
    borderTopColor: accountColors.border,
    paddingVertical: 12,
    gap: 4,
  },
  reviewTitle: { color: accountColors.text, fontWeight: "600" },
  date: { color: accountColors.muted, fontSize: 12 },
});
