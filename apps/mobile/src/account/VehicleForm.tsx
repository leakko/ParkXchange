import { useState } from "react";
import {
  ActivityIndicator,
  Image,
  Pressable,
  Text,
  TextInput,
  View,
} from "react-native";

import type { CreateVehicleRequest } from "@/api/client";
import { accountStyles } from "@/account/theme";
import {
  pickVehiclePhoto,
  type PickedVehiclePhoto,
} from "@/account/pickVehiclePhoto";
import { useTranslation, type TranslationKey } from "@/i18n";

const SIZE_CLASSES = ["small", "medium", "large"] as const;

const SIZE_LABELS: Record<(typeof SIZE_CLASSES)[number], TranslationKey> = {
  small: "account.vehicles.size.small",
  medium: "account.vehicles.size.medium",
  large: "account.vehicles.size.large",
};

export type VehicleFormValues = CreateVehicleRequest;

type Props = {
  initial: VehicleFormValues;
  /** Local file URI or pre-fetched data URI — never a remote auth URL. */
  photoUri?: string | null;
  submitLabel: string;
  busy?: boolean;
  onSubmit: (values: VehicleFormValues, photo: PickedVehiclePhoto | null) => void;
  onDelete?: () => void;
  deleteBusy?: boolean;
};

export function VehicleForm({
  initial,
  photoUri,
  submitLabel,
  busy,
  onSubmit,
  onDelete,
  deleteBusy,
}: Props) {
  const { t } = useTranslation();
  const [plate, setPlate] = useState(initial.plate);
  const [makeModel, setMakeModel] = useState(initial.make_model);
  const [sizeClass, setSizeClass] = useState(initial.size_class);
  const [color, setColor] = useState(initial.color);
  const [year, setYear] = useState(String(initial.year));
  const [picked, setPicked] = useState<PickedVehiclePhoto | null>(null);
  const [pickError, setPickError] = useState<string | null>(null);

  const previewUri = picked?.uri ?? photoUri ?? null;

  return (
    <View style={{ gap: 4 }}>
      <View style={accountStyles.field}>
        <Text style={accountStyles.label}>{t("account.vehicles.form.plate")}</Text>
        <TextInput
          style={accountStyles.input}
          value={plate}
          onChangeText={setPlate}
          autoCapitalize="characters"
          placeholderTextColor="#7A93A0"
        />
      </View>
      <View style={accountStyles.field}>
        <Text style={accountStyles.label}>{t("account.vehicles.form.makeModel")}</Text>
        <TextInput
          style={accountStyles.input}
          value={makeModel}
          onChangeText={setMakeModel}
          placeholderTextColor="#7A93A0"
        />
      </View>
      <View style={accountStyles.field}>
        <Text style={accountStyles.label}>{t("account.vehicles.form.size")}</Text>
        <View style={accountStyles.sizeRow}>
          {SIZE_CLASSES.map((size) => {
            const active = sizeClass === size;
            return (
              <Pressable
                key={size}
                style={[accountStyles.sizeChip, active && accountStyles.sizeChipActive]}
                onPress={() => setSizeClass(size)}
              >
                <Text
                  style={[
                    accountStyles.sizeChipText,
                    active && accountStyles.sizeChipTextActive,
                  ]}
                >
                  {t(SIZE_LABELS[size])}
                </Text>
              </Pressable>
            );
          })}
        </View>
      </View>
      <View style={accountStyles.field}>
        <Text style={accountStyles.label}>{t("account.vehicles.form.color")}</Text>
        <TextInput
          style={accountStyles.input}
          value={color}
          onChangeText={setColor}
          placeholderTextColor="#7A93A0"
        />
      </View>
      <View style={accountStyles.field}>
        <Text style={accountStyles.label}>{t("account.vehicles.form.year")}</Text>
        <TextInput
          style={accountStyles.input}
          value={year}
          onChangeText={setYear}
          keyboardType="number-pad"
          placeholderTextColor="#7A93A0"
        />
      </View>

      <View style={accountStyles.field}>
        <Text style={accountStyles.label}>{t("account.vehicles.form.photoOptional")}</Text>
        {previewUri ? (
          <Image
            source={{ uri: previewUri }}
            style={{ width: "100%", height: 160, borderRadius: 12, marginBottom: 8 }}
            resizeMode="cover"
          />
        ) : null}
        <Pressable
          style={accountStyles.secondary}
          onPress={() => {
            void (async () => {
              setPickError(null);
              try {
                const photo = await pickVehiclePhoto();
                if (photo) {
                  setPicked(photo);
                }
              } catch (err) {
                setPickError(
                  err instanceof Error
                    ? err.message
                    : t("account.vehicles.form.pickPhotoFailed"),
                );
              }
            })();
          }}
        >
          <Text style={accountStyles.secondaryText}>
            {previewUri
              ? t("account.vehicles.form.changePhoto")
              : t("account.vehicles.form.addPhoto")}
          </Text>
        </Pressable>
        {pickError ? <Text style={accountStyles.error}>{pickError}</Text> : null}
      </View>

      <Pressable
        style={accountStyles.primary}
        disabled={busy}
        onPress={() => {
          const yearNum = Number.parseInt(year, 10);
          onSubmit(
            {
              plate: plate.trim(),
              make_model: makeModel.trim(),
              size_class: sizeClass,
              color: color.trim(),
              year: yearNum,
            },
            picked,
          );
        }}
      >
        {busy ? (
          <ActivityIndicator color="#fff" />
        ) : (
          <Text style={accountStyles.primaryText}>{submitLabel}</Text>
        )}
      </Pressable>

      {onDelete ? (
        <Pressable
          style={[accountStyles.danger, { marginTop: 8 }]}
          disabled={deleteBusy}
          onPress={onDelete}
        >
          {deleteBusy ? (
            <ActivityIndicator color="#FF8FAB" />
          ) : (
            <Text style={accountStyles.dangerText}>
              {t("account.vehicles.delete.action")}
            </Text>
          )}
        </Pressable>
      ) : null}
    </View>
  );
}
