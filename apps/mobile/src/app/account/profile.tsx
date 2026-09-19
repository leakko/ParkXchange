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
import { useTranslation, type AppLocale } from "@/i18n";

export default function ProfileScreen() {
  const { t, locale, setLocale } = useTranslation();
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
      Alert.alert(t("account.profile.saved.title"), t("account.profile.saved.displayName"));
    },
    onError: (err) => {
      Alert.alert(
        t("account.profile.saveFailed.title"),
        err instanceof Error ? err.message : t("common.error"),
      );
    },
  });

  const savePassword = useMutation({
    mutationFn: async () => {
      if (newPassword !== confirmPassword) {
        throw new Error(t("account.profile.passwordMismatch"));
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
      Alert.alert(
        t("account.profile.passwordChanged.title"),
        t("account.profile.passwordChanged.message"),
      );
    },
    onError: (err) => {
      Alert.alert(
        t("account.profile.passwordChangeFailed.title"),
        err instanceof Error ? err.message : t("common.error"),
      );
    },
  });

  if (!ready || me.isLoading) {
    return (
      <View style={[accountStyles.screen, { justifyContent: "center", alignItems: "center" }]}>
        <ActivityIndicator color="#F4F7FA" />
      </View>
    );
  }

  const languageOptions: { id: AppLocale; labelKey: "account.profile.language.es" | "account.profile.language.en" }[] =
    [
      { id: "es", labelKey: "account.profile.language.es" },
      { id: "en", labelKey: "account.profile.language.en" },
    ];

  return (
    <ScrollView
      style={accountStyles.screen}
      contentContainerStyle={accountStyles.scroll}
      keyboardShouldPersistTaps="handled"
    >
      <Text style={accountStyles.sectionTitle}>{t("account.profile.language.section")}</Text>
      <View style={accountStyles.section}>
        {languageOptions.map((opt) => {
          const selected = locale === opt.id;
          return (
            <Pressable
              key={opt.id}
              style={accountStyles.row}
              onPress={() => setLocale(opt.id)}
            >
              <Text style={accountStyles.rowTitle}>{t(opt.labelKey)}</Text>
              {selected ? <Text style={accountStyles.link}>✓</Text> : null}
            </Pressable>
          );
        })}
      </View>

      <Text style={[accountStyles.sectionTitle, { marginTop: 24 }]}>
        {t("account.profile.displayName.section")}
      </Text>
      <View style={accountStyles.field}>
        <Text style={accountStyles.label}>{t("account.profile.displayName.label")}</Text>
        <TextInput
          style={accountStyles.input}
          value={displayName}
          onChangeText={setDisplayName}
          autoCapitalize="words"
          placeholderTextColor="#7A93A0"
        />
      </View>
      <Text style={accountStyles.meta}>
        {t("account.profile.emailReadOnly", { email: me.data?.email ?? "" })}
      </Text>
      <Pressable
        style={accountStyles.primary}
        disabled={saveName.isPending || !displayName.trim()}
        onPress={() => saveName.mutate()}
      >
        {saveName.isPending ? (
          <ActivityIndicator color="#fff" />
        ) : (
          <Text style={accountStyles.primaryText}>{t("account.profile.saveName")}</Text>
        )}
      </Pressable>

      <View style={[accountStyles.section, { marginTop: 24 }]}>
        <Text style={accountStyles.sectionTitle}>{t("account.profile.password.section")}</Text>
        <View style={accountStyles.field}>
          <Text style={accountStyles.label}>{t("account.profile.password.current")}</Text>
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
          <Text style={accountStyles.label}>{t("account.profile.password.new")}</Text>
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
          <Text style={accountStyles.label}>{t("account.profile.password.confirm")}</Text>
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
            <Text style={accountStyles.primaryText}>{t("account.profile.updatePassword")}</Text>
          )}
        </Pressable>
      </View>
    </ScrollView>
  );
}
