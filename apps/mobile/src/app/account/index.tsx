import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type Href, useRouter } from "expo-router";
import { useState } from "react";
import {
  ActivityIndicator,
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
import { useConfirm } from "@/ui/ConfirmModal";

export default function AccountHubScreen() {
  const { t } = useTranslation();
  const { confirm, alert } = useConfirm();
  const router = useRouter();
  const queryClient = useQueryClient();
  const { ready, signedIn, signedOut, signOut } = useSession();
  const [deleting, setDeleting] = useState(false);
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
    onError: async (err) => {
      await alert({
        title: t("account.delete.failed.title"),
        message: err instanceof Error ? err.message : t("common.error"),
        confirmLabel: t("common.ok"),
      });
    },
  });

  const onDeletePress = async () => {
    const ok = await confirm({
      title: t("account.delete.confirmTitle"),
      message: t("account.delete.confirmMessage"),
      cancelLabel: t("common.cancel"),
      confirmLabel: t("account.delete.action"),
      destructive: true,
    });
    if (!ok) {
      return;
    }
    setDeleting(true);
    try {
      await closeAccount.mutateAsync();
    } finally {
      setDeleting(false);
    }
  };

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
        style={[accountStyles.secondary, { marginTop: 24 }]}
        onPress={() => {
          void signOut();
        }}
      >
        <Text style={accountStyles.secondaryText}>{t("account.signOut")}</Text>
      </Pressable>

      <Pressable
        style={[accountStyles.danger, { marginTop: 12 }]}
        disabled={deleting || closeAccount.isPending}
        onPress={() => void onDeletePress()}
      >
        {deleting || closeAccount.isPending ? (
          <ActivityIndicator color="#FF8FAB" />
        ) : (
          <Text style={accountStyles.dangerText}>{t("account.delete.title")}</Text>
        )}
      </Pressable>
    </ScrollView>
  );
}
