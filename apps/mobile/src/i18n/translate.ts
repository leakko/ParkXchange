type Messages = Record<string, string>;

/** Replace `{name}` placeholders; unknown keys fall back to the key itself. */
export function translate(
  messages: Messages,
  key: string,
  params?: Record<string, string | number>,
): string {
  let text = messages[key] ?? key;
  if (!params) return text;
  for (const [name, value] of Object.entries(params)) {
    text = text.replaceAll(`{${name}}`, String(value));
  }
  return text;
}
