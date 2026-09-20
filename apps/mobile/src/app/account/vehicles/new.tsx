import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useLocalSearchParams, useRouter } from "expo-router";
import { Alert, Text } from "react-native";

import { createVehicle, putVehiclePhoto } from "@/api/client";
import { VehicleForm, type VehicleFormValues } from "@/account/VehicleForm";
import type { PickedVehiclePhoto } from "@/account/pickVehiclePhoto";
import { accountStyles } from "@/account/theme";
import { AuthScroll } from "@/auth/AuthScroll";
import { useTranslation } from "@/i18n";

const empty: VehicleFormValues = {
  plate: "",
  make_model: "",
  size_class: "medium",
  color: "",
  year: new Date().getFullYear(),
};

export default function NewVehicleScreen() {
  const { t } = useTranslation();
  const router = useRouter();
  const params = useLocalSearchParams<{ from?: string }>();
  const fromAnnounce = params.from === "announce" || params.from === "offer";
  const queryClient = useQueryClient();

  const create = useMutation({
    mutationFn: async ({
      values,
      photo,
    }: {
      values: VehicleFormValues;
      photo: PickedVehiclePhoto | null;
    }) => {
      const vehicle = await createVehicle(values);
      if (photo) {
        await putVehiclePhoto(vehicle.id, photo.bytes, photo.contentType);
      }
      return vehicle;
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["vehicles"] });
      router.back();
    },
    onError: (err) => {
      Alert.alert(
        t("account.vehicles.createFailed.title"),
        err instanceof Error ? err.message : t("common.error"),
      );
    },
  });

  return (
    <AuthScroll>
      {fromAnnounce ? (
        <Text style={[accountStyles.meta, { marginBottom: 8 }]}>
          {t("account.vehicles.create.announceHint")}
        </Text>
      ) : null}
      <VehicleForm
        initial={empty}
        submitLabel={t("account.vehicles.create.submit")}
        busy={create.isPending}
        onSubmit={(values, photo) => create.mutate({ values, photo })}
      />
    </AuthScroll>
  );
}
