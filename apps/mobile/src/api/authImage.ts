import { getAccessToken } from "@/api/session";

/**
 * RN Image `source.headers` is unreliable across platforms. Fetch the bytes
 * with an Authorization header and return a data URI the Image can load locally.
 */
export async function fetchAuthImageUri(
  url: string | null | undefined,
): Promise<string | null> {
  if (!url) {
    return null;
  }

  const token = await getAccessToken();
  if (!token) {
    return null;
  }

  try {
    const res = await fetch(url, {
      headers: { Authorization: `Bearer ${token}` },
    });
    if (!res.ok) {
      return null;
    }

    const rawType = res.headers.get("Content-Type") ?? "image/jpeg";
    const mime = rawType.split(";")[0]?.trim() || "image/jpeg";
    const buffer = await res.arrayBuffer();
    if (buffer.byteLength === 0) {
      return null;
    }

    return `data:${mime};base64,${bytesToBase64(new Uint8Array(buffer))}`;
  } catch {
    return null;
  }
}

function bytesToBase64(bytes: Uint8Array): string {
  let binary = "";
  const chunkSize = 0x8000;
  for (let i = 0; i < bytes.length; i += chunkSize) {
    const chunk = bytes.subarray(i, i + chunkSize);
    binary += String.fromCharCode(...chunk);
  }
  return btoa(binary);
}
