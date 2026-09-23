import { useMutation, useQueryClient } from "@tanstack/react-query";
import { type Href, useLocalSearchParams, useRouter } from "expo-router";
import { useEffect, useRef } from "react";
import { Text } from "react-native";

import { createVehicle, putVehiclePhoto } from "@/api/client";
import { apiErrorMessage } from "@/api/errors";
import { VehicleForm, type VehicleFormValues } from "@/account/VehicleForm";
import type { PickedVehiclePhoto } from "@/account/pickVehiclePhoto";
import { accountStyles } from "@/account/theme";
import { AuthScroll } from "@/auth/AuthScroll";
import { useSession } from "@/hooks/useSession";
import { useTranslation } from "@/i18n";
import {
  notifyVehicleCreated,
} from "@/map/vehicleCreateHandoff";
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
  const { alert, confirm } = useConfirm();
  const { ready, signedIn } = useSession();
  const router = useRouter();
  const params = useLocalSearchParams<{ from?: string }>();
  const gateStarted = useRef(false);
  const from = Array.isArray(params.from) ? params.from[0] : params.from;
  const fromAnnounce = from === "announce" || from === "offer";
  const queryClient = useQueryClient();

  useEffect(() => {
    if (!ready || signedIn || gateStarted.current) {
      return;
    }
    gateStarted.current = true;
    const returnTo = from
      ? `/account/vehicles/new?from=${encodeURIComponent(from)}`
      : "/account/vehicles/new";

    void (async () => {
      const proceed = await confirm({
        title: t("auth.required.title"),
        message: t("auth.required.addVehicle"),
        cancelLabel: t("common.cancel"),
        confirmLabel: t("auth.required.signIn"),
      });
      if (proceed) {
        router.replace(
          `/auth/login?returnTo=${encodeURIComponent(returnTo)}` as Href,
        );
      } else {
        router.back();
      }
    })();
  }, [confirm, from, ready, router, signedIn, t]);

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
      if (from === "offer") {
        notifyVehicleCreated("offer");
      } else if (from === "announce") {
        notifyVehicleCreated("announce");
      }
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

  if (!ready || !signedIn) {
    return null;
  }

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
