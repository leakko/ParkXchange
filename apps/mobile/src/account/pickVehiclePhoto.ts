import * as ImagePicker from "expo-image-picker";

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

/** Opens the library with light client compression; the API normalizes size. */
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

  return { bytes, contentType, uri: asset.uri };
}
