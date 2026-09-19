import { useQuery } from "@tanstack/react-query";
import { type Href, useRouter } from "expo-router";
import { ActivityIndicator, Pressable, ScrollView, Text, View } from "react-native";

import { getMe } from "@/api/client";
import { accountStyles } from "@/account/theme";
import { useDevSession } from "@/hooks/useDevSession";
import { useTranslation } from "@/i18n";

export default function AccountHubScreen() {
  const { t } = useTranslation();
  const router = useRouter();
  const { ready, signedOut, signOut, retry } = useDevSession();
  const me = useQuery({
    queryKey: ["me"],
    queryFn: getMe,
    enabled: ready,
  });

  if (signedOut) {
    return (
      <View style={[accountStyles.screen, accountStyles.scroll]}>
        <Text style={accountStyles.title}>{t("account.signedOut.title")}</Text>
        <Text style={accountStyles.meta}>{t("account.signedOut.message")}</Text>
        <Pressable
          style={[accountStyles.primary, { marginTop: 16 }]}
          onPress={() => {
            void retry();
          }}
        >
          <Text style={accountStyles.primaryText}>{t("account.devLogin")}</Text>
        </Pressable>
      </View>
    );
  }

  if (!ready || me.isLoading) {
    return (
      <View style={[accountStyles.screen, { justifyContent: "center", alignItems: "center" }]}>
        <ActivityIndicator color="#F4F7FA" />
      </View>
    );
  }

  if (me.error || !me.data) {
    return (
      <View style={[accountStyles.screen, accountStyles.scroll]}>
        <Text style={accountStyles.error}>
          {me.error instanceof Error ? me.error.message : t("account.loadFailed")}
        </Text>
        <Pressable
          style={[accountStyles.primary, { marginTop: 16 }]}
          onPress={() => {
            void retry();
          }}
        >
          <Text style={accountStyles.primaryText}>{t("account.devLogin")}</Text>
        </Pressable>
      </View>
    );
  }

  const user = me.data;
  const rating =
    user.rating != null
      ? t("account.rating.withScore", {
          score: user.rating.toFixed(1),
          count: user.rating_count,
        })
      : t("account.rating.countOnly", { count: user.rating_count });
  const balance = `€${(user.balance_cents / 100).toFixed(2)}`;

  return (
    <ScrollView style={accountStyles.screen} contentContainerStyle={accountStyles.scroll}>
      <Text style={accountStyles.title}>{user.display_name}</Text>
      <Text style={accountStyles.subtitle}>{user.email}</Text>
      {user.phone ? (
        <Text style={accountStyles.subtitle}>{user.phone}</Text>
      ) : null}
      <Text style={accountStyles.meta}>
        {rating} · {t("account.balance", { amount: balance })}
      </Text>

      <View style={accountStyles.section}>
        <Pressable
          style={accountStyles.row}
          onPress={() => router.push("/account/profile" as Href)}
        >
          <Text style={accountStyles.rowTitle}>{t("account.profile.title")}</Text>
          <Text style={accountStyles.link}>{t("account.profile.edit")}</Text>
        </Pressable>
        <Pressable
          style={accountStyles.row}
          onPress={() => router.push("/account/vehicles" as Href)}
        >
          <Text style={accountStyles.rowTitle}>{t("account.vehicles.title")}</Text>
          <Text style={accountStyles.link}>{t("account.vehicles.manage")}</Text>
        </Pressable>
        <Pressable
          style={accountStyles.row}
          onPress={() => router.push("/account/spots" as Href)}
        >
          <Text style={accountStyles.rowTitle}>{t("account.spots.title")}</Text>
          <Text style={accountStyles.link}>{t("account.spots.manage")}</Text>
        </Pressable>
      </View>

      <Pressable
        style={[accountStyles.danger, { marginTop: 16 }]}
        onPress={() => {
          void signOut();
        }}
      >
        <Text style={accountStyles.dangerText}>{t("account.signOut")}</Text>
      </Pressable>
    </ScrollView>
  );
}
