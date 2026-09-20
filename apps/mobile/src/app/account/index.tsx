import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type Href, useRouter } from "expo-router";
import { useCallback } from "react";
import {
  ActivityIndicator,
  Alert,
  Pressable,
  ScrollView,
  Text,
  View,
} from "react-native";

import { deleteAccount, getMe } from "@/api/client";
import { accountStyles } from "@/account/theme";
import { useSession } from "@/hooks/useSession";
import { useTranslation } from "@/i18n";
import { formatPoints } from "@/i18n/formatPoints";

export default function AccountHubScreen() {
  const { t } = useTranslation();
  const router = useRouter();
  const queryClient = useQueryClient();
  const { ready, signedIn, signedOut, signOut } = useSession();
  const me = useQuery({
    queryKey: ["me"],
    queryFn: getMe,
    enabled: signedIn,
  });

  const closeAccount = useMutation({
    mutationFn: deleteAccount,
    onSuccess: async () => {
      queryClient.clear();
      await signOut();
    },
    onError: (err) => {
      Alert.alert(
        t("account.delete.failed.title"),
        err instanceof Error ? err.message : t("common.error"),
      );
    },
  });

  const confirmDelete = useCallback(() => {
    Alert.alert(t("account.delete.confirmTitle"), t("account.delete.confirmMessage"), [
      { text: t("common.cancel"), style: "cancel" },
      {
        text: t("account.delete.action"),
        style: "destructive",
        onPress: () => closeAccount.mutate(),
      },
    ]);
  }, [closeAccount, t]);

  if (!ready) {
    return (
      <View style={[accountStyles.screen, { justifyContent: "center", alignItems: "center" }]}>
        <ActivityIndicator color="#F4F7FA" />
      </View>
    );
  }

  if (signedOut) {
    return (
      <View style={[accountStyles.screen, accountStyles.scroll]}>
        <Text style={accountStyles.title}>{t("account.signedOut.title")}</Text>
        <Text style={accountStyles.meta}>{t("account.signedOut.message")}</Text>
        <Pressable
          style={[accountStyles.primary, { marginTop: 16 }]}
          onPress={() =>
            router.push(
              `/auth/login?returnTo=${encodeURIComponent("/account")}` as Href,
            )
          }
        >
          <Text style={accountStyles.primaryText}>{t("account.signIn")}</Text>
        </Pressable>
      </View>
    );
  }

  if (me.isLoading) {
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
          onPress={() =>
            router.push(
              `/auth/login?returnTo=${encodeURIComponent("/account")}` as Href,
            )
          }
        >
          <Text style={accountStyles.primaryText}>{t("account.signIn")}</Text>
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
  const balance = `${formatPoints(user.balance_cents)} pts`;

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

      <Pressable
        style={accountStyles.row}
        onPress={() => router.push("/account/profile" as Href)}
      >
        <View>
          <Text style={accountStyles.rowTitle}>{t("account.profile.title")}</Text>
          <Text style={accountStyles.rowMeta}>{t("account.profile.edit")}</Text>
        </View>
      </Pressable>
      <Pressable
        style={accountStyles.row}
        onPress={() => router.push("/account/vehicles" as Href)}
      >
        <View>
          <Text style={accountStyles.rowTitle}>{t("account.vehicles.title")}</Text>
          <Text style={accountStyles.rowMeta}>{t("account.vehicles.manage")}</Text>
        </View>
      </Pressable>
      <Pressable
        style={accountStyles.row}
        onPress={() => router.push("/account/spots" as Href)}
      >
        <View>
          <Text style={accountStyles.rowTitle}>{t("account.spots.title")}</Text>
          <Text style={accountStyles.rowMeta}>{t("account.spots.manage")}</Text>
        </View>
      </Pressable>
      <Pressable
        style={accountStyles.row}
        onPress={() => router.push("/account/reservations" as Href)}
      >
        <View>
          <Text style={accountStyles.rowTitle}>{t("account.reservations.title")}</Text>
          <Text style={accountStyles.rowMeta}>{t("account.reservations.manage")}</Text>
        </View>
      </Pressable>

      <Pressable
        style={[accountStyles.danger, { marginTop: 24 }]}
        onPress={() => {
          void signOut();
        }}
      >
        <Text style={accountStyles.dangerText}>{t("account.signOut")}</Text>
      </Pressable>

      <Pressable
        style={[accountStyles.danger, { marginTop: 12 }]}
        disabled={closeAccount.isPending}
        onPress={confirmDelete}
      >
        <Text style={accountStyles.dangerText}>{t("account.delete.title")}</Text>
      </Pressable>
    </ScrollView>
  );
}
