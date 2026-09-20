import * as Localization from "expo-localization";

/** ISO 3166-1 alpha-2 → ITU calling code (digits only, no +). */
const REGION_CALLING_CODES: Record<string, string> = {
  ES: "34",
  AD: "376",
  PT: "351",
  FR: "33",
  DE: "49",
  IT: "39",
  GB: "44",
  IE: "353",
  NL: "31",
  BE: "32",
  CH: "41",
  AT: "43",
  US: "1",
  CA: "1",
  MX: "52",
  AR: "54",
  CL: "56",
  CO: "57",
  PE: "51",
  BR: "55",
  MA: "212",
};

const DEFAULT_CALLING_CODE = "34";

/** Calling code from the device region, falling back to Spain (+34). */
export function resolveDefaultCallingCode(
  regionCode = Localization.getLocales()[0]?.regionCode,
): string {
  const region = regionCode?.toUpperCase();
  if (region && REGION_CALLING_CODES[region]) {
    return REGION_CALLING_CODES[region];
  }
  return DEFAULT_CALLING_CODE;
}

/**
 * Turns a user-typed phone into E.164.
 * Bare national numbers get the device-region prefix (or +34).
 */
export function normalizePhoneInput(
  raw: string,
  callingCode = resolveDefaultCallingCode(),
): string {
  const trimmed = raw.trim();
  if (!trimmed) {
    return "";
  }

  let hasPlus = false;
  let digits = "";
  for (let i = 0; i < trimmed.length; i++) {
    const ch = trimmed.charAt(i);
    if (ch === "+" && i === 0) {
      hasPlus = true;
      continue;
    }
    if (ch >= "0" && ch <= "9") {
      digits += ch;
    }
  }
  if (!digits) {
    return "";
  }

  if (hasPlus) {
    return `+${digits}`;
  }
  if (digits.startsWith(callingCode)) {
    return `+${digits}`;
  }
  // Drop a trunk prefix 0 common in national dialling (e.g. 0600… → 600…).
  if (digits.startsWith("0")) {
    digits = digits.slice(1);
  }
  return `+${callingCode}${digits}`;
}
