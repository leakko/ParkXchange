import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useLocalSearchParams, useRouter } from "expo-router";
import {
  ActivityIndicator,
  Text,
  View,
} from "react-native";

import {
  deleteVehicle,
  listVehicles,
  putVehiclePhoto,
  updateVehicle,
  vehiclePhotoUrl,
} from "@/api/client";
import { apiErrorMessage } from "@/api/errors";
import { VehicleForm, type VehicleFormValues } from "@/account/VehicleForm";
import type { PickedVehiclePhoto } from "@/account/pickVehiclePhoto";
import { accountStyles } from "@/account/theme";
import { AuthScroll } from "@/auth/AuthScroll";
import { useAuthImage } from "@/hooks/useAuthImage";
import { useSession } from "@/hooks/useSession";
import { useTranslation } from "@/i18n";
import { useConfirm } from "@/ui/ConfirmModal";

export default function EditVehicleScreen() {
  const { t } = useTranslation();
  const { confirm, alert } = useConfirm();
  const { id } = useLocalSearchParams<{ id: string }>();
  const router = useRouter();
  const { signedIn } = useSession();
  const queryClient = useQueryClient();

  const vehicles = useQuery({
    queryKey: ["vehicles"],
    queryFn: listVehicles,
    enabled: signedIn,
  });

  const vehicle = vehicles.data?.find((v) => v.id === id);
  const { uri: authPhotoUri } = useAuthImage(
    vehicle?.has_photo ? vehiclePhotoUrl(vehicle.id) : null,
  );

  const save = useMutation({
    mutationFn: async ({
      values,
      photo,
    }: {
      values: VehicleFormValues;
      photo: PickedVehiclePhoto | null;
    }) => {
      if (!id) {
        throw new Error(t("account.vehicles.edit.missingId"));
      }
      const updated = await updateVehicle(id, values);
      if (photo) {
        await putVehiclePhoto(id, photo.bytes, photo.contentType);
      }
      return updated;
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["vehicles"] });
      await alert({
        title: t("account.vehicles.edit.saved.title"),
        message: t("account.vehicles.edit.saved.message"),
        confirmLabel: t("common.ok"),
      });
    },
    onError: async (err) => {
      await alert({
        title: t("account.vehicles.edit.saveFailed.title"),
        message: err instanceof Error ? err.message : t("common.error"),
        confirmLabel: t("common.ok"),
      });
    },
  });

  const remove = useMutation({
    mutationFn: async () => {
      if (!id) {
        throw new Error(t("account.vehicles.edit.missingId"));
      }
      await deleteVehicle(id);
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["vehicles"] });
      router.back();
    },
    onError: async (err) => {
      await alert({
        title: t("account.vehicles.deleteFailed.title"),
        message: apiErrorMessage(err, t),
        confirmLabel: t("common.ok"),
      });
    },
  });

  if (!signedIn || vehicles.isLoading) {
    return (
      <View style={[accountStyles.screen, { justifyContent: "center", alignItems: "center" }]}>
        <ActivityIndicator color="#F4F7FA" />
      </View>
    );
  }

  if (!vehicle) {
    return (
      <View style={[accountStyles.screen, accountStyles.scroll]}>
        <Text style={accountStyles.error}>{t("account.vehicles.notFound")}</Text>
      </View>
    );
  }

  return (
    <AuthScroll>
      <VehicleForm
        initial={{
          plate: vehicle.plate,
          make_model: vehicle.make_model,
          size_class: vehicle.size_class,
          color: vehicle.color,
          year: vehicle.year,
        }}
        photoUri={authPhotoUri}
        submitLabel={t("account.vehicles.edit.submit")}
        busy={save.isPending}
        onSubmit={(values, photo) => save.mutate({ values, photo })}
        deleteBusy={remove.isPending}
        onDelete={async () => {
          const ok = await confirm({
            title: t("account.vehicles.delete.confirmTitle"),
            message: t("account.vehicles.delete.confirmMessage"),
            cancelLabel: t("common.cancel"),
            confirmLabel: t("account.vehicles.delete.confirm"),
            destructive: true,
          });
          if (ok) {
            remove.mutate();
          }
        }}
      />
    </AuthScroll>
  );
}
