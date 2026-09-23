import { useQuery } from "@tanstack/react-query";
import { Stack, useLocalSearchParams } from "expo-router";
import { useState } from "react";
import {
  ActivityIndicator,
  FlatList,
  Pressable,
  StyleSheet,
  Text,
  View,
} from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";

import { getMe, getUserProfile } from "@/api/client";
import { accountColors, accountStyles } from "@/account/theme";
import { useSession } from "@/hooks/useSession";
import { useTranslation } from "@/i18n";
import { useConfirm } from "@/ui/ConfirmModal";
import { ReportModal, type ReportTarget } from "@/ui/ReportModal";

export default function UserProfileScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const { t } = useTranslation();
  const insets = useSafeAreaInsets();
  const { alert } = useConfirm();
  const { signedIn } = useSession();
  const [reportTarget, setReportTarget] = useState<ReportTarget | null>(null);
  const me = useQuery({
    queryKey: ["me"],
    queryFn: getMe,
    enabled: signedIn,
  });
  const profile = useQuery({
    queryKey: ["user-profile", id],
    queryFn: () => getUserProfile(String(id)),
    enabled: !!id,
  });

  const canReport =
    signedIn && !!id && !!me.data?.id && String(id) !== String(me.data.id);

  const headerTitle =
    profile.data?.display_name?.trim() || t("profile.public.title");

  return (
    <View style={accountStyles.screen}>
      <Stack.Screen options={{ title: headerTitle }} />
      {profile.isLoading ? (
        <ActivityIndicator
          color={accountColors.accent}
          style={{ marginTop: 40 }}
        />
      ) : profile.isError || !profile.data ? (
        <Text style={[accountStyles.meta, styles.pad]}>
          {t("profile.public.missing")}
        </Text>
      ) : (
        <FlatList
          contentContainerStyle={[
            styles.list,
            { paddingBottom: 40 + insets.bottom },
          ]}
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
              {canReport ? (
                <Pressable
                  style={{ marginTop: 12 }}
                  onPress={() =>
                    setReportTarget({ kind: "profile", userId: String(id) })
                  }
                >
                  <Text style={accountStyles.link}>{t("report.profile.cta")}</Text>
                </Pressable>
              ) : null}
            </View>
          }
          data={profile.data.reviews}
          keyExtractor={(item, i) => `${item.created_at}-${i}`}
          ListEmptyComponent={
            <Text style={accountStyles.meta}>
              {t("profile.public.emptyReviews")}
            </Text>
          }
          renderItem={({ item }) => (
            <View style={styles.review}>
              <Text style={styles.reviewTitle}>
                <Text style={styles.starsOn}>{"★".repeat(item.stars)}</Text>
                <Text style={styles.starsOff}>{"☆".repeat(5 - item.stars)}</Text>
                {" · "}
                {item.rater_name}
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

      <ReportModal
        target={reportTarget}
        visible={!!reportTarget}
        onClose={() => setReportTarget(null)}
        onSubmitted={() => {
          void alert({
            title: t("report.sent.title"),
            message: t("report.sent.message"),
            confirmLabel: t("common.ok"),
          });
        }}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  pad: { padding: 16 },
  list: { padding: 16, gap: 12 },
  header: { gap: 6, marginBottom: 12 },
  review: {
    borderTopWidth: StyleSheet.hairlineWidth,
    borderTopColor: accountColors.border,
    paddingVertical: 12,
    gap: 4,
  },
  reviewTitle: { color: accountColors.text, fontWeight: "600" },
  starsOn: { color: "#F5C518" },
  starsOff: { color: accountColors.muted },
  date: { color: accountColors.muted, fontSize: 12 },
});
