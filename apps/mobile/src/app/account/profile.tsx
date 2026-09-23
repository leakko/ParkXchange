import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import {
  ActivityIndicator,
  Pressable,
  Text,
  View,
} from "react-native";

import { changePassword, getMe, updateMe } from "@/api/client";
import { accountStyles } from "@/account/theme";
import { AuthScroll } from "@/auth/AuthScroll";
import { AuthTextInput } from "@/auth/AuthTextInput";
import { PasswordField } from "@/auth/PasswordField";
import { normalizePhoneInput } from "@/auth/phone";
import { useSession } from "@/hooks/useSession";
import { useTranslation, type AppLocale } from "@/i18n";
import {
  getLocationAssistanceEnabled,
  setLocationAssistanceEnabled,
} from "@/push/settings";
import { disarmArrivalGeofence } from "@/push/geofence";
import { useConfirm } from "@/ui/ConfirmModal";
import { ReportModal, type ReportTarget } from "@/ui/ReportModal";

export default function ProfileScreen() {
  const { t, locale, setLocale } = useTranslation();
  const { alert } = useConfirm();
  const { signedIn } = useSession();
  const queryClient = useQueryClient();
  const me = useQuery({
    queryKey: ["me"],
    queryFn: getMe,
    enabled: signedIn,
  });

  const [displayName, setDisplayName] = useState("");
  const [phone, setPhone] = useState("");
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [locationAssistance, setLocationAssistance] = useState(true);
  const [reportTarget, setReportTarget] = useState<ReportTarget | null>(null);

  useEffect(() => {
    if (me.data) {
      setDisplayName(me.data.display_name);
      setPhone(me.data.phone ?? "");
    }
  }, [me.data]);

  useEffect(() => {
    void getLocationAssistanceEnabled().then(setLocationAssistance);
  }, []);

  const saveName = useMutation({
    mutationFn: () => updateMe({ display_name: displayName.trim() }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["me"] });
      await alert({
        title: t("account.profile.saved.title"),
        message: t("account.profile.saved.displayName"),
        confirmLabel: t("common.ok"),
      });
    },
    onError: async (err) => {
      await alert({
        title: t("account.profile.saveFailed.title"),
        message: err instanceof Error ? err.message : t("common.error"),
        confirmLabel: t("common.ok"),
      });
    },
  });

  const savePhone = useMutation({
    mutationFn: () => updateMe({ phone: normalizePhoneInput(phone) }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["me"] });
      await alert({
        title: t("account.profile.saved.title"),
        message: t("account.profile.saved.phone"),
        confirmLabel: t("common.ok"),
      });
    },
    onError: async (err) => {
      await alert({
        title: t("account.profile.saveFailed.title"),
        message: err instanceof Error ? err.message : t("common.error"),
        confirmLabel: t("common.ok"),
      });
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
    onSuccess: async () => {
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
      await alert({
        title: t("account.profile.passwordChanged.title"),
        message: t("account.profile.passwordChanged.message"),
        confirmLabel: t("common.ok"),
      });
    },
    onError: async (err) => {
      await alert({
        title: t("account.profile.passwordChangeFailed.title"),
        message: err instanceof Error ? err.message : t("common.error"),
        confirmLabel: t("common.ok"),
      });
    },
  });

  if (!signedIn || me.isLoading) {
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
    <AuthScroll>
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
        {t("account.profile.locationAssistance.section")}
      </Text>
      <Pressable
        style={accountStyles.row}
        onPress={() => {
          const next = !locationAssistance;
          setLocationAssistance(next);
          void setLocationAssistanceEnabled(next);
          if (!next) {
            void disarmArrivalGeofence();
          }
        }}
      >
        <View style={{ flex: 1, paddingRight: 12 }}>
          <Text style={accountStyles.rowTitle}>
            {t("account.profile.locationAssistance.title")}
          </Text>
          <Text style={accountStyles.rowMeta}>
            {t("account.profile.locationAssistance.meta")}
          </Text>
        </View>
        <Text style={accountStyles.link}>
          {locationAssistance
            ? t("account.profile.locationAssistance.on")
            : t("account.profile.locationAssistance.off")}
        </Text>
      </Pressable>

      <Text style={[accountStyles.sectionTitle, { marginTop: 24 }]}>
        {t("account.profile.displayName.section")}
      </Text>
      <View style={accountStyles.field}>
        <Text style={accountStyles.label}>{t("account.profile.displayName.label")}</Text>
        <AuthTextInput
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

      <Text style={[accountStyles.sectionTitle, { marginTop: 24 }]}>
        {t("account.profile.phone.section")}
      </Text>
      <View style={accountStyles.field}>
        <Text style={accountStyles.label}>{t("account.profile.phone.label")}</Text>
        <AuthTextInput
          value={phone}
          onChangeText={setPhone}
          keyboardType="phone-pad"
          placeholder={t("auth.phone.placeholder")}
          placeholderTextColor="#7A93A0"
        />
      </View>
      <Text style={accountStyles.meta}>{t("auth.phone.hint")}</Text>
      <Pressable
        style={accountStyles.primary}
        disabled={savePhone.isPending}
        onPress={() => savePhone.mutate()}
      >
        {savePhone.isPending ? (
          <ActivityIndicator color="#fff" />
        ) : (
          <Text style={accountStyles.primaryText}>{t("account.profile.savePhone")}</Text>
        )}
      </Pressable>

      <View style={[accountStyles.section, { marginTop: 24 }]}>
        <Text style={accountStyles.sectionTitle}>{t("account.profile.password.section")}</Text>
        <View style={accountStyles.field}>
          <Text style={accountStyles.label}>{t("account.profile.password.current")}</Text>
          <PasswordField value={currentPassword} onChangeText={setCurrentPassword} />
        </View>
        <View style={accountStyles.field}>
          <Text style={accountStyles.label}>{t("account.profile.password.new")}</Text>
          <PasswordField value={newPassword} onChangeText={setNewPassword} />
        </View>
        <View style={accountStyles.field}>
          <Text style={accountStyles.label}>{t("account.profile.password.confirm")}</Text>
          <PasswordField value={confirmPassword} onChangeText={setConfirmPassword} />
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

      <Pressable
        style={[accountStyles.secondary, { marginTop: 24 }]}
        onPress={() => setReportTarget({ kind: "problem" })}
      >
        <Text style={accountStyles.secondaryText}>{t("report.problem.cta")}</Text>
      </Pressable>

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
    </AuthScroll>
  );
}
