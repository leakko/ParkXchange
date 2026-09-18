import * as ImagePicker from "expo-image-picker";

/** API rejects photos over ~300 KiB. */
export const MAX_VEHICLE_PHOTO_BYTES = 300 * 1024;

export type PickedVehiclePhoto = {
  bytes: ArrayBuffer;
  contentType: "image/jpeg" | "image/png";
  uri: string;
};

function contentTypeForUri(uri: string, mime?: string | null): "image/jpeg" | "image/png" | null {
  const lower = (mime ?? uri).toLowerCase();
  if (lower.includes("png")) {
    return "image/png";
  }
  if (lower.includes("jpeg") || lower.includes("jpg")) {
    return "image/jpeg";
  }
  return null;
}

/** Opens the library with compression; rejects oversize / non-JPEG/PNG. */
export async function pickVehiclePhoto(): Promise<PickedVehiclePhoto | null> {
  const permission = await ImagePicker.requestMediaLibraryPermissionsAsync();
  if (!permission.granted) {
    throw new Error("Photo library permission is required");
  }

  const result = await ImagePicker.launchImageLibraryAsync({
    mediaTypes: ["images"],
    allowsEditing: true,
    quality: 0.45,
    exif: false,
  });
  if (result.canceled || !result.assets[0]) {
    return null;
  }

  const asset = result.assets[0];
  const contentType = contentTypeForUri(asset.uri, asset.mimeType);
  if (!contentType) {
    throw new Error("Only JPEG or PNG photos are supported");
  }

  const res = await fetch(asset.uri);
  const bytes = await res.arrayBuffer();
  if (bytes.byteLength > MAX_VEHICLE_PHOTO_BYTES) {
    throw new Error(
      `Photo is too large (${Math.ceil(bytes.byteLength / 1024)} KiB). Max is 300 KiB.`,
    );
  }

  return { bytes, contentType, uri: asset.uri };
}
