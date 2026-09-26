/** Matches labels like "37.37000, -5.97000" that are not useful to people. */
const COORDINATE_LABEL_RE = /^-?\d+(\.\d+)?,\s*-?\d+(\.\d+)?$/;

export function looksLikeCoordinateLabel(label: string | null | undefined): boolean {
  const trimmed = label?.trim() ?? "";
  return trimmed.length > 0 && COORDINATE_LABEL_RE.test(trimmed);
}
