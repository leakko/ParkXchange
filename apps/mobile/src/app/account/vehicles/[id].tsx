import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useLocalSearchParams, useRouter } from "expo-router";
import { useEffect, useState } from "react";
import {
  ActivityIndicator,
  Alert,
  ScrollView,
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
import { getAccessToken } from "@/api/session";
import { VehicleForm, type VehicleFormValues } from "@/account/VehicleForm";
import type { PickedVehiclePhoto } from "@/account/pickVehiclePhoto";
import { accountStyles } from "@/account/theme";
import { useDevSession } from "@/hooks/useDevSession";

export default function EditVehicleScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const router = useRouter();
  const { ready } = useDevSession();
  const queryClient = useQueryClient();
  const [authHeader, setAuthHeader] = useState<Record<string, string> | undefined>();

  const vehicles = useQuery({
    queryKey: ["vehicles"],
    queryFn: listVehicles,
    enabled: ready,
  });

  const vehicle = vehicles.data?.find((v) => v.id === id);

  useEffect(() => {
    void getAccessToken().then((token) => {
      if (token) {
        setAuthHeader({ Authorization: `Bearer ${token}` });
      }
    });
  }, []);

  const save = useMutation({
    mutationFn: async ({
      values,
      photo,
    }: {
      values: VehicleFormValues;
      photo: PickedVehiclePhoto | null;
    }) => {
      if (!id) {
        throw new Error("Missing vehicle id");
      }
      const updated = await updateVehicle(id, values);
      if (photo) {
        await putVehiclePhoto(id, photo.bytes, photo.contentType);
      }
      return updated;
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["vehicles"] });
      Alert.alert("Saved", "Vehicle updated.");
    },
    onError: (err) => {
      Alert.alert("Save failed", err instanceof Error ? err.message : "error");
    },
  });

  const remove = useMutation({
    mutationFn: async () => {
      if (!id) {
        throw new Error("Missing vehicle id");
      }
      await deleteVehicle(id);
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["vehicles"] });
      router.back();
    },
    onError: (err) => {
      Alert.alert("Delete failed", err instanceof Error ? err.message : "error");
    },
  });

  if (!ready || vehicles.isLoading) {
    return (
      <View style={[accountStyles.screen, { justifyContent: "center", alignItems: "center" }]}>
        <ActivityIndicator color="#F4F7FA" />
      </View>
    );
  }

  if (!vehicle) {
    return (
      <View style={[accountStyles.screen, accountStyles.scroll]}>
        <Text style={accountStyles.error}>Vehicle not found</Text>
      </View>
    );
  }

  return (
    <ScrollView
      style={accountStyles.screen}
      contentContainerStyle={accountStyles.scroll}
      keyboardShouldPersistTaps="handled"
    >
      <VehicleForm
        initial={{
          plate: vehicle.plate,
          make_model: vehicle.make_model,
          size_class: vehicle.size_class,
          color: vehicle.color,
          year: vehicle.year,
        }}
        photoUri={vehicle.has_photo ? vehiclePhotoUrl(vehicle.id) : null}
        photoHeaders={authHeader}
        submitLabel="Save vehicle"
        busy={save.isPending}
        onSubmit={(values, photo) => save.mutate({ values, photo })}
        deleteBusy={remove.isPending}
        onDelete={() => {
          Alert.alert(
            "Delete vehicle?",
            "This fails if an active spot still references it.",
            [
              { text: "Cancel", style: "cancel" },
              {
                text: "Delete",
                style: "destructive",
                onPress: () => remove.mutate(),
              },
            ],
          );
        }}
      />
    </ScrollView>
  );
}
