/**
 * Deduplicate Expo's sticky last-notification response so sign-in / locale
 * remounts do not reopen a reservation from a previous account or tap.
 */

export type NotificationResponseIdentity = {
  actionIdentifier: string;
  notification: {
    request: {
      identifier: string;
    };
  };
};

export function notificationResponseKey(
  response: NotificationResponseIdentity,
): string {
  return `${response.notification.request.identifier}:${response.actionIdentifier}`;
}

/** True when this OS "last response" has not been handled yet. */
export function shouldHandleLastNotificationResponse(
  response: NotificationResponseIdentity | null | undefined,
  previouslyHandledKey: string | null | undefined,
): boolean {
  if (!response?.notification?.request?.identifier) {
    return false;
  }
  const key = notificationResponseKey(response);
  if (!key || key === ":") {
    return false;
  }
  return key !== previouslyHandledKey;
}
