import { Stack } from "expo-router";

import { accountColors } from "@/account/theme";

export default function AuthLayout() {
  return (
    <Stack
      screenOptions={{
        headerStyle: { backgroundColor: accountColors.bg },
        headerTintColor: accountColors.text,
        headerTitleStyle: { color: accountColors.text },
        contentStyle: { backgroundColor: accountColors.bg },
      }}
    >
      <Stack.Screen name="login" options={{ title: "Sign in" }} />
      <Stack.Screen name="register" options={{ title: "Create account" }} />
      <Stack.Screen name="forgot" options={{ title: "Forgot password" }} />
      <Stack.Screen name="reset" options={{ title: "Reset password" }} />
      <Stack.Screen name="verify-email" options={{ title: "Confirm email" }} />
    </Stack>
  );
}
