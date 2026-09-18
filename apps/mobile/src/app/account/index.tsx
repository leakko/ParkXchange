import { useQuery } from "@tanstack/react-query";
import { type Href, useRouter } from "expo-router";
import { ActivityIndicator, Pressable, ScrollView, Text, View } from "react-native";

import { getMe } from "@/api/client";
import { accountStyles } from "@/account/theme";
import { useDevSession } from "@/hooks/useDevSession";

export default function AccountHubScreen() {
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
        <Text style={accountStyles.title}>Signed out</Text>
        <Text style={accountStyles.meta}>
          Dev session cleared. Map and account APIs stay idle until you sign in
          again (or restart the app).
        </Text>
        <Pressable
          style={[accountStyles.primary, { marginTop: 16 }]}
          onPress={() => {
            void retry();
          }}
        >
          <Text style={accountStyles.primaryText}>Dev login</Text>
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
          {me.error instanceof Error ? me.error.message : "Failed to load account"}
        </Text>
        <Pressable
          style={[accountStyles.primary, { marginTop: 16 }]}
          onPress={() => {
            void retry();
          }}
        >
          <Text style={accountStyles.primaryText}>Dev login</Text>
        </Pressable>
      </View>
    );
  }

  const user = me.data;
  const rating =
    user.rating != null
      ? `${user.rating.toFixed(1)} · ${user.rating_count} ratings`
      : `${user.rating_count} ratings`;
  const balance = `€${(user.balance_cents / 100).toFixed(2)}`;

  return (
    <ScrollView style={accountStyles.screen} contentContainerStyle={accountStyles.scroll}>
      <Text style={accountStyles.title}>{user.display_name}</Text>
      <Text style={accountStyles.subtitle}>{user.email}</Text>
      <Text style={accountStyles.meta}>
        {rating} · balance {balance}
      </Text>

      <View style={accountStyles.section}>
        <Pressable
          style={accountStyles.row}
          onPress={() => router.push("/account/profile" as Href)}
        >
          <Text style={accountStyles.rowTitle}>Profile</Text>
          <Text style={accountStyles.link}>Edit</Text>
        </Pressable>
        <Pressable
          style={accountStyles.row}
          onPress={() => router.push("/account/vehicles" as Href)}
        >
          <Text style={accountStyles.rowTitle}>Vehicles</Text>
          <Text style={accountStyles.link}>Manage</Text>
        </Pressable>
        <Pressable
          style={accountStyles.row}
          onPress={() => router.push("/account/spots" as Href)}
        >
          <Text style={accountStyles.rowTitle}>My spots</Text>
          <Text style={accountStyles.link}>Manage</Text>
        </Pressable>
      </View>

      <Pressable
        style={[accountStyles.danger, { marginTop: 16 }]}
        onPress={() => {
          void signOut();
        }}
      >
        <Text style={accountStyles.dangerText}>Sign out</Text>
      </Pressable>
    </ScrollView>
  );
}
