import { Stack } from "expo-router";

import { accountColors } from "@/account/theme";
import { useTranslation } from "@/i18n";

export default function AccountLayout() {
  const { t } = useTranslation();

  return (
    <Stack
      screenOptions={{
        headerShown: true,
        headerStyle: { backgroundColor: accountColors.bg },
        headerTintColor: accountColors.text,
        headerTitleStyle: { color: accountColors.text, fontWeight: "600" },
        headerShadowVisible: false,
        contentStyle: { backgroundColor: accountColors.bg },
      }}
    >
      <Stack.Screen name="index" options={{ title: t("account.nav.account") }} />
      <Stack.Screen name="profile" options={{ title: t("account.nav.profile") }} />
      <Stack.Screen name="vehicles/index" options={{ title: t("account.nav.vehicles") }} />
      <Stack.Screen name="vehicles/new" options={{ title: t("account.nav.newVehicle") }} />
      <Stack.Screen name="vehicles/[id]" options={{ title: t("account.nav.editVehicle") }} />
      <Stack.Screen name="spots/index" options={{ title: t("account.nav.mySpots") }} />
      <Stack.Screen name="spots/[id]" options={{ title: t("account.nav.editSpot") }} />
    </Stack>
  );
}
