import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useLocalSearchParams, useRouter } from "expo-router";
import { Text } from "react-native";

import { createVehicle, putVehiclePhoto } from "@/api/client";
import { apiErrorMessage } from "@/api/errors";
import { VehicleForm, type VehicleFormValues } from "@/account/VehicleForm";
import type { PickedVehiclePhoto } from "@/account/pickVehiclePhoto";
import { accountStyles } from "@/account/theme";
import { AuthScroll } from "@/auth/AuthScroll";
import { useTranslation } from "@/i18n";
import { useConfirm } from "@/ui/ConfirmModal";

const empty: VehicleFormValues = {
  plate: "",
  make_model: "",
  size_class: "medium",
  color: "",
  year: new Date().getFullYear(),
};

export default function NewVehicleScreen() {
  const { t } = useTranslation();
  const { alert } = useConfirm();
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
      // Create first; photo is optional. A failed photo must not leave the user
      // thinking create failed (orphan car + confusing plate_taken on retry).
      const vehicle = await createVehicle(values);
      let photoFailed = false;
      if (photo) {
        try {
          await putVehiclePhoto(vehicle.id, photo.bytes, photo.contentType);
        } catch {
          photoFailed = true;
        }
      }
      return { vehicle, photoFailed };
    },
    onSuccess: async ({ photoFailed }) => {
      await queryClient.invalidateQueries({ queryKey: ["vehicles"] });
      if (photoFailed) {
        await alert({
          title: t("account.vehicles.create.photoFailed.title"),
          message: t("account.vehicles.create.photoFailed.message"),
          confirmLabel: t("common.ok"),
        });
        router.back();
        return;
      }
      router.back();
    },
    onError: async (err) => {
      await alert({
        title: t("account.vehicles.createFailed.title"),
        message: apiErrorMessage(err, t),
        confirmLabel: t("common.ok"),
      });
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
