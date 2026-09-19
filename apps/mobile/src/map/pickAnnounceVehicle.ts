import { type Href } from "expo-router";
import { ActionSheetIOS, Platform } from "react-native";

import { listVehicles, type VehicleResponse } from "@/api/client";

function label(v: VehicleResponse): string {
  return `${v.plate} · ${v.make_model}`;
}

type Navigate = { push: (href: Href) => void };

/**
 * Platform picker. Android uses a modal list via `showAndroidList` because
 * Alert only supports three buttons.
 */
function chooseVehicle(
  vehicles: VehicleResponse[],
  showAndroidList: (vehicles: VehicleResponse[]) => Promise<string | null>,
): Promise<string | null> {
  return new Promise((resolve) => {
    if (Platform.OS === "ios") {
      const options = [...vehicles.map(label), "Cancel"];
      ActionSheetIOS.showActionSheetWithOptions(
        {
          title: "Which vehicle?",
          message: "Drivers will see this car at the exchange.",
          options,
          cancelButtonIndex: options.length - 1,
        },
        (index) => {
          if (index == null || index === options.length - 1) {
            resolve(null);
            return;
          }
          resolve(vehicles[index]?.id ?? null);
        },
      );
      return;
    }

    void showAndroidList(vehicles).then(resolve);
  });
}

/**
 * Ensures the user has a vehicle and picks one for announce.
 * Navigates to create-vehicle when the list is empty; returns null if cancelled
 * or when redirected to create.
 *
 * `showAndroidList` presents a modal list (Android Alert cannot list many cars).
 */
export async function pickAnnounceVehicle(
  router: Navigate,
  showAndroidList: (vehicles: VehicleResponse[]) => Promise<string | null>,
): Promise<string | null> {
  const vehicles = await listVehicles();
  if (vehicles.length === 0) {
    router.push("/account/vehicles/new" as Href);
    return null;
  }
  if (vehicles.length === 1) {
    return vehicles[0]!.id;
  }
  return chooseVehicle(vehicles, showAndroidList);
}

export type { VehicleResponse };
