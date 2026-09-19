import type { components } from "@parkxchange/api-contract";

import { apiUrl } from "@/config";
import {
  clearSessionAndNotify,
  getAccessToken,
  getRefreshToken,
  setSession,
} from "@/api/session";

export type SpotFeatureCollection = components["schemas"]["SpotFeatureCollection"];
export type SpotFeature = components["schemas"]["SpotFeature"];
export type SessionResponse = components["schemas"]["SessionResponse"];
export type TicketResponse = components["schemas"]["TicketResponse"];
export type ReservationResponse = components["schemas"]["ReservationResponse"];
export type OfferResponse = components["schemas"]["OfferResponse"];
export type CreateOfferRequest = components["schemas"]["CreateOfferRequest"];
export type CreateSpotRequest = components["schemas"]["CreateSpotRequest"];
export type UpdateSpotRequest = components["schemas"]["UpdateSpotRequest"];
export type UserResponse = components["schemas"]["UserResponse"];
export type VehicleResponse = components["schemas"]["VehicleResponse"];
export type CreateVehicleRequest = components["schemas"]["CreateVehicleRequest"];
export type UpdateVehicleRequest = components["schemas"]["UpdateVehicleRequest"];

export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

type BBox = readonly [number, number, number, number];

/** Auth endpoints where 401 means bad credentials, not a dead session. */
const authCredentialPaths = new Set([
  "/v1/auth/login",
  "/v1/auth/register",
  "/v1/auth/google",
  "/v1/auth/refresh",
  "/v1/auth/password/forgot",
  "/v1/auth/password/reset",
  "/v1/me/password",
]);

let refreshInFlight: Promise<boolean> | null = null;

async function tryRefreshTokens(): Promise<boolean> {
  if (refreshInFlight) {
    return refreshInFlight;
  }
  refreshInFlight = (async () => {
    const refresh = await getRefreshToken();
    if (!refresh) {
      return false;
    }
    try {
      const session = await refreshSession(refresh);
      await setSession(session.access_token, session.refresh_token);
      return true;
    } catch {
      return false;
    }
  })().finally(() => {
    refreshInFlight = null;
  });
  return refreshInFlight;
}

async function parseError(res: Response): Promise<ApiError> {
  try {
    const body = (await res.json()) as { error?: { code?: string; message?: string } };
    return new ApiError(
      res.status,
      body.error?.code ?? "unknown",
      body.error?.message ?? res.statusText,
    );
  } catch {
    return new ApiError(res.status, "unknown", res.statusText);
  }
}

async function rawFetch(path: string, init: RequestInit = {}): Promise<Response> {
  const headers = new Headers(init.headers);
  if (!headers.has("Content-Type") && init.body) {
    headers.set("Content-Type", "application/json");
  }
  const token = await getAccessToken();
  if (token && !headers.has("Authorization")) {
    headers.set("Authorization", `Bearer ${token}`);
  }
  return fetch(`${apiUrl}${path}`, { ...init, headers });
}

export async function apiFetch(path: string, init: RequestInit = {}): Promise<Response> {
  const res = await rawFetch(path, init);
  if (res.status !== 401 || authCredentialPaths.has(path)) {
    return res;
  }

  // Access token missing/expired: try refresh once, then wipe UI session state.
  if (await tryRefreshTokens()) {
    return rawFetch(path, init);
  }
  await clearSessionAndNotify();
  return res;
}

export async function login(email: string, password: string): Promise<SessionResponse> {
  const res = await apiFetch("/v1/auth/login", {
    method: "POST",
    body: JSON.stringify({ email, password }),
  });
  if (!res.ok) {
    throw await parseError(res);
  }
  return (await res.json()) as SessionResponse;
}

export async function register(body: {
  email: string;
  password: string;
  display_name: string;
  phone?: string;
}): Promise<SessionResponse> {
  const res = await apiFetch("/v1/auth/register", {
    method: "POST",
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    throw await parseError(res);
  }
  return (await res.json()) as SessionResponse;
}

export async function loginWithGoogle(idToken: string): Promise<SessionResponse> {
  const res = await apiFetch("/v1/auth/google", {
    method: "POST",
    body: JSON.stringify({ id_token: idToken }),
  });
  if (!res.ok) {
    throw await parseError(res);
  }
  return (await res.json()) as SessionResponse;
}

export async function forgotPassword(email: string): Promise<void> {
  const res = await apiFetch("/v1/auth/password/forgot", {
    method: "POST",
    body: JSON.stringify({ email }),
  });
  if (!res.ok) {
    throw await parseError(res);
  }
}

export async function resetPassword(token: string, password: string): Promise<void> {
  const res = await apiFetch("/v1/auth/password/reset", {
    method: "POST",
    body: JSON.stringify({ token, password }),
  });
  if (!res.ok) {
    throw await parseError(res);
  }
}

export async function refreshSession(refreshToken: string): Promise<SessionResponse> {
  const res = await fetch(`${apiUrl}/v1/auth/refresh`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ refresh_token: refreshToken }),
  });
  if (!res.ok) {
    throw await parseError(res);
  }
  return (await res.json()) as SessionResponse;
}

export async function logout(refreshToken: string): Promise<void> {
  await apiFetch("/v1/auth/logout", {
    method: "POST",
    body: JSON.stringify({ refresh_token: refreshToken }),
  });
}

export async function issueWsTicket(): Promise<TicketResponse> {
  const res = await apiFetch("/v1/ws/tickets", { method: "POST" });
  if (!res.ok) {
    throw await parseError(res);
  }
  return (await res.json()) as TicketResponse;
}

export function bboxQuery(bbox: BBox): string {
  return bbox.map((n) => n.toFixed(6)).join(",");
}

export async function fetchSpots(opts: {
  bbox: BBox;
  zoom: number;
  from: string;
  to: string;
}): Promise<SpotFeatureCollection> {
  const qs = new URLSearchParams({
    bbox: bboxQuery(opts.bbox),
    zoom: String(Math.round(opts.zoom)),
    from: opts.from,
    to: opts.to,
  });
  const res = await apiFetch(`/v1/spots?${qs}`);
  if (!res.ok) {
    throw await parseError(res);
  }
  return (await res.json()) as SpotFeatureCollection;
}

export async function createSpot(body: CreateSpotRequest): Promise<SpotFeature> {
  const res = await apiFetch("/v1/spots", {
    method: "POST",
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    throw await parseError(res);
  }
  return (await res.json()) as SpotFeature;
}

export async function getSpot(id: string): Promise<SpotFeature> {
  const res = await apiFetch(`/v1/spots/${id}`);
  if (!res.ok) {
    throw await parseError(res);
  }
  return (await res.json()) as SpotFeature;
}

export async function getMe(): Promise<UserResponse> {
  const res = await apiFetch("/v1/me");
  if (!res.ok) {
    throw await parseError(res);
  }
  return (await res.json()) as UserResponse;
}

export async function updateMe(body: {
  display_name?: string;
  phone?: string;
}): Promise<UserResponse> {
  const res = await apiFetch("/v1/me", {
    method: "PATCH",
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    throw await parseError(res);
  }
  return (await res.json()) as UserResponse;
}

export async function changePassword(body: {
  current_password: string;
  new_password: string;
}): Promise<void> {
  const res = await apiFetch("/v1/me/password", {
    method: "POST",
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    throw await parseError(res);
  }
}

export async function listVehicles(): Promise<VehicleResponse[]> {
  const res = await apiFetch("/v1/vehicles");
  if (!res.ok) {
    throw await parseError(res);
  }
  return (await res.json()) as VehicleResponse[];
}

export async function createVehicle(body: CreateVehicleRequest): Promise<VehicleResponse> {
  const res = await apiFetch("/v1/vehicles", {
    method: "POST",
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    throw await parseError(res);
  }
  return (await res.json()) as VehicleResponse;
}

export async function updateVehicle(
  id: string,
  body: UpdateVehicleRequest,
): Promise<VehicleResponse> {
  const res = await apiFetch(`/v1/vehicles/${id}`, {
    method: "PATCH",
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    throw await parseError(res);
  }
  return (await res.json()) as VehicleResponse;
}

export async function deleteVehicle(id: string): Promise<void> {
  const res = await apiFetch(`/v1/vehicles/${id}`, { method: "DELETE" });
  if (!res.ok) {
    throw await parseError(res);
  }
}

/** Raw JPEG/PNG body — Content-Type must be the image media type, not JSON. */
export async function putVehiclePhoto(
  id: string,
  bytes: ArrayBuffer,
  contentType: string,
): Promise<void> {
  const res = await apiFetch(`/v1/vehicles/${id}/photo`, {
    method: "PUT",
    headers: { "Content-Type": contentType },
    body: bytes,
  });
  if (!res.ok) {
    throw await parseError(res);
  }
}

export function vehiclePhotoUrl(id: string): string {
  return `${apiUrl}/v1/vehicles/${id}/photo`;
}

export async function fetchMySpots(): Promise<SpotFeatureCollection> {
  const res = await apiFetch("/v1/spots/mine");
  if (!res.ok) {
    throw await parseError(res);
  }
  return (await res.json()) as SpotFeatureCollection;
}

export async function updateSpot(id: string, body: UpdateSpotRequest): Promise<SpotFeature> {
  const res = await apiFetch(`/v1/spots/${id}`, {
    method: "PATCH",
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    throw await parseError(res);
  }
  return (await res.json()) as SpotFeature;
}

export async function withdrawSpot(id: string): Promise<void> {
  const res = await apiFetch(`/v1/spots/${id}`, { method: "DELETE" });
  if (!res.ok) {
    throw await parseError(res);
  }
}

export function spotVehiclePhotoUrl(spotId: string): string {
  return `${apiUrl}/v1/spots/${spotId}/vehicle/photo`;
}

export async function createOffer(
  spotId: string,
  body: CreateOfferRequest,
): Promise<OfferResponse> {
  const res = await apiFetch(`/v1/spots/${spotId}/offers`, {
    method: "POST",
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    throw await parseError(res);
  }
  return (await res.json()) as OfferResponse;
}

export async function listOffers(spotId: string): Promise<OfferResponse[]> {
  const res = await apiFetch(`/v1/spots/${spotId}/offers`);
  if (!res.ok) {
    throw await parseError(res);
  }
  return (await res.json()) as OfferResponse[];
}

export async function listMyOffers(): Promise<OfferResponse[]> {
  const res = await apiFetch("/v1/offers/mine");
  if (!res.ok) {
    throw await parseError(res);
  }
  return (await res.json()) as OfferResponse[];
}

async function postOfferAction(id: string, action: string): Promise<Response> {
  const res = await apiFetch(`/v1/offers/${id}/${action}`, { method: "POST" });
  if (!res.ok) {
    throw await parseError(res);
  }
  return res;
}

export async function acceptOffer(id: string): Promise<ReservationResponse> {
  const res = await postOfferAction(id, "accept");
  return (await res.json()) as ReservationResponse;
}

export const rejectOffer = async (id: string): Promise<void> => {
  await postOfferAction(id, "reject");
};

export const withdrawOffer = async (id: string): Promise<void> => {
  await postOfferAction(id, "withdraw");
};

export async function fetchActiveReservations(): Promise<ReservationResponse[]> {
  const res = await apiFetch("/v1/reservations/active");
  if (!res.ok) {
    throw await parseError(res);
  }
  return (await res.json()) as ReservationResponse[];
}

export async function fetchReservations(): Promise<ReservationResponse[]> {
  const res = await apiFetch("/v1/reservations");
  if (!res.ok) {
    throw await parseError(res);
  }
  return (await res.json()) as ReservationResponse[];
}

export async function getReservation(id: string): Promise<ReservationResponse> {
  const res = await apiFetch(`/v1/reservations/${id}`);
  if (!res.ok) {
    throw await parseError(res);
  }
  return (await res.json()) as ReservationResponse;
}

async function postReservationAction(id: string, action: string): Promise<void> {
  const res = await apiFetch(`/v1/reservations/${id}/${action}`, { method: "POST" });
  if (!res.ok) {
    throw await parseError(res);
  }
}

export const cancelReservation = (id: string) => postReservationAction(id, "cancel");
export const ownerReady = (id: string) => postReservationAction(id, "owner-ready");
export const driverArrived = (id: string) =>
  postReservationAction(id, "driver-arrived");
export const clearDriverArrived = (id: string) =>
  postReservationAction(id, "clear-driver-arrived");
export const driverReady = (id: string) =>
  postReservationAction(id, "driver-ready");
export const driverConfirmEntered = (id: string) =>
  postReservationAction(id, "driver-confirm-entered");
export const driverReportOwnerNoShow = (id: string) =>
  postReservationAction(id, "driver-report-owner-no-show");
