import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import {
  ActivityIndicator,
  Alert,
  Pressable,
  ScrollView,
  Text,
  TextInput,
  View,
} from "react-native";

import { changePassword, getMe, updateMe } from "@/api/client";
import { accountStyles } from "@/account/theme";
import { useDevSession } from "@/hooks/useDevSession";

export default function ProfileScreen() {
  const { ready } = useDevSession();
  const queryClient = useQueryClient();
  const me = useQuery({
    queryKey: ["me"],
    queryFn: getMe,
    enabled: ready,
  });

  const [displayName, setDisplayName] = useState("");
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");

  useEffect(() => {
    if (me.data) {
      setDisplayName(me.data.display_name);
    }
  }, [me.data]);

  const saveName = useMutation({
    mutationFn: () => updateMe({ display_name: displayName.trim() }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["me"] });
      Alert.alert("Saved", "Display name updated.");
    },
    onError: (err) => {
      Alert.alert("Save failed", err instanceof Error ? err.message : "error");
    },
  });

  const savePassword = useMutation({
    mutationFn: async () => {
      if (newPassword !== confirmPassword) {
        throw new Error("New password and confirmation do not match");
      }
      await changePassword({
        current_password: currentPassword,
        new_password: newPassword,
      });
    },
    onSuccess: () => {
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
      Alert.alert("Password changed", "Use the new password next time you sign in.");
    },
    onError: (err) => {
      Alert.alert("Password change failed", err instanceof Error ? err.message : "error");
    },
  });

  if (!ready || me.isLoading) {
    return (
      <View style={[accountStyles.screen, { justifyContent: "center", alignItems: "center" }]}>
        <ActivityIndicator color="#F4F7FA" />
      </View>
    );
  }

  return (
    <ScrollView
      style={accountStyles.screen}
      contentContainerStyle={accountStyles.scroll}
      keyboardShouldPersistTaps="handled"
    >
      <Text style={accountStyles.sectionTitle}>Display name</Text>
      <View style={accountStyles.field}>
        <Text style={accountStyles.label}>Name</Text>
        <TextInput
          style={accountStyles.input}
          value={displayName}
          onChangeText={setDisplayName}
          autoCapitalize="words"
          placeholderTextColor="#7A93A0"
        />
      </View>
      <Text style={accountStyles.meta}>Email is {me.data?.email} (read-only)</Text>
      <Pressable
        style={accountStyles.primary}
        disabled={saveName.isPending || !displayName.trim()}
        onPress={() => saveName.mutate()}
      >
        {saveName.isPending ? (
          <ActivityIndicator color="#fff" />
        ) : (
          <Text style={accountStyles.primaryText}>Save name</Text>
        )}
      </Pressable>

      <View style={[accountStyles.section, { marginTop: 24 }]}>
        <Text style={accountStyles.sectionTitle}>Change password</Text>
        <View style={accountStyles.field}>
          <Text style={accountStyles.label}>Current password</Text>
          <TextInput
            style={accountStyles.input}
            value={currentPassword}
            onChangeText={setCurrentPassword}
            secureTextEntry
            autoCapitalize="none"
            placeholderTextColor="#7A93A0"
          />
        </View>
        <View style={accountStyles.field}>
          <Text style={accountStyles.label}>New password</Text>
          <TextInput
            style={accountStyles.input}
            value={newPassword}
            onChangeText={setNewPassword}
            secureTextEntry
            autoCapitalize="none"
            placeholderTextColor="#7A93A0"
          />
        </View>
        <View style={accountStyles.field}>
          <Text style={accountStyles.label}>Confirm new password</Text>
          <TextInput
            style={accountStyles.input}
            value={confirmPassword}
            onChangeText={setConfirmPassword}
            secureTextEntry
            autoCapitalize="none"
            placeholderTextColor="#7A93A0"
          />
        </View>
        <Pressable
          style={accountStyles.primary}
          disabled={
            savePassword.isPending ||
            !currentPassword ||
            !newPassword ||
            !confirmPassword
          }
          onPress={() => savePassword.mutate()}
        >
          {savePassword.isPending ? (
            <ActivityIndicator color="#fff" />
          ) : (
            <Text style={accountStyles.primaryText}>Update password</Text>
          )}
        </Pressable>
      </View>
    </ScrollView>
  );
}
