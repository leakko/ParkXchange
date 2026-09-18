import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useRouter } from "expo-router";
import { Alert, ScrollView } from "react-native";

import { createVehicle, putVehiclePhoto } from "@/api/client";
import { VehicleForm, type VehicleFormValues } from "@/account/VehicleForm";
import type { PickedVehiclePhoto } from "@/account/pickVehiclePhoto";
import { accountStyles } from "@/account/theme";

const empty: VehicleFormValues = {
  plate: "",
  make_model: "",
  size_class: "medium",
  color: "",
  year: new Date().getFullYear(),
};

export default function NewVehicleScreen() {
  const router = useRouter();
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
      Alert.alert("Create failed", err instanceof Error ? err.message : "error");
    },
  });

  return (
    <ScrollView
      style={accountStyles.screen}
      contentContainerStyle={accountStyles.scroll}
      keyboardShouldPersistTaps="handled"
    >
      <VehicleForm
        initial={empty}
        submitLabel="Create vehicle"
        busy={create.isPending}
        onSubmit={(values, photo) => create.mutate({ values, photo })}
      />
    </ScrollView>
  );
}
