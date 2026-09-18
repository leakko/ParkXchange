import { Stack } from "expo-router";

import { accountColors } from "@/account/theme";

export default function AccountLayout() {
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
      <Stack.Screen name="index" options={{ title: "Account" }} />
      <Stack.Screen name="profile" options={{ title: "Profile" }} />
      <Stack.Screen name="vehicles/index" options={{ title: "Vehicles" }} />
      <Stack.Screen name="vehicles/new" options={{ title: "New vehicle" }} />
      <Stack.Screen name="vehicles/[id]" options={{ title: "Edit vehicle" }} />
      <Stack.Screen name="spots/index" options={{ title: "My spots" }} />
      <Stack.Screen name="spots/[id]" options={{ title: "Edit spot" }} />
    </Stack>
  );
}
