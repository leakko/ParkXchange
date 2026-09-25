import { StyleSheet } from "react-native";

/** Dark map chrome shared by the account stack. */
export const accountColors = {
  bg: "#0B1F33",
  card: "#16324F",
  accent: "#1B9AAA",
  text: "#F4F7FA",
  muted: "#9DB4C0",
  hint: "#D6E2E9",
  window: "#7A93A0",
  dangerBg: "#3D1F2B",
  dangerText: "#FF8FAB",
  border: "#1F3A56",
  input: "#0F2740",
} as const;

export const accountStyles = StyleSheet.create({
  screen: {
    flex: 1,
    backgroundColor: accountColors.bg,
  },
  scroll: {
    padding: 20,
    gap: 12,
    paddingBottom: 40,
  },
  title: {
    color: accountColors.text,
    fontSize: 22,
    fontWeight: "700",
  },
  subtitle: {
    color: accountColors.muted,
    fontSize: 14,
  },
  meta: {
    color: accountColors.muted,
    fontSize: 13,
  },
  label: {
    color: accountColors.muted,
    fontSize: 13,
    marginBottom: 6,
  },
  input: {
    backgroundColor: accountColors.input,
    borderWidth: 1,
    borderColor: accountColors.border,
    borderRadius: 12,
    color: accountColors.text,
    paddingHorizontal: 14,
    paddingVertical: 12,
    fontSize: 15,
  },
  inputInvalid: {
    borderColor: accountColors.dangerText,
  },
  fieldError: {
    color: accountColors.dangerText,
    fontSize: 12,
    marginTop: 4,
  },
  field: {
    marginBottom: 12,
  },
  row: {
    backgroundColor: accountColors.card,
    borderRadius: 12,
    paddingHorizontal: 16,
    paddingVertical: 14,
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    gap: 12,
  },
  /** List cards in Mis plazas / Mis reservas — denser than generic rows. */
  rowCard: {
    backgroundColor: accountColors.card,
    borderRadius: 12,
    paddingHorizontal: 14,
    paddingVertical: 10,
    flexDirection: "column",
    alignItems: "stretch",
    gap: 2,
    marginBottom: 8,
  },
  /** Open / in-progress items that still need attention (not history). */
  rowAttention: {
    borderWidth: 1.5,
    borderColor: accountColors.text,
  },
  rowTitle: {
    color: accountColors.text,
    fontSize: 16,
    fontWeight: "600",
  },
  rowMeta: {
    color: accountColors.muted,
    fontSize: 13,
    marginTop: 2,
  },
  rowMetaTight: {
    color: accountColors.muted,
    fontSize: 13,
    marginTop: 0,
  },
  rowActions: {
    flexDirection: "row",
    flexWrap: "wrap",
    gap: 8,
    marginTop: 6,
  },
  primary: {
    backgroundColor: accountColors.accent,
    borderRadius: 12,
    paddingVertical: 12,
    alignItems: "center",
  },
  primaryText: {
    color: "#fff",
    fontWeight: "600",
    fontSize: 15,
  },
  secondary: {
    backgroundColor: accountColors.card,
    borderRadius: 12,
    paddingVertical: 12,
    alignItems: "center",
  },
  secondaryText: {
    color: accountColors.text,
    fontWeight: "600",
    fontSize: 15,
  },
  danger: {
    backgroundColor: accountColors.dangerBg,
    borderRadius: 12,
    paddingVertical: 12,
    alignItems: "center",
  },
  dangerText: {
    color: accountColors.dangerText,
    fontWeight: "600",
    fontSize: 15,
  },
  link: {
    color: accountColors.accent,
    fontWeight: "600",
    fontSize: 15,
  },
  empty: {
    color: accountColors.muted,
    fontSize: 14,
    textAlign: "center",
    marginTop: 24,
  },
  error: {
    color: accountColors.dangerText,
    fontSize: 13,
  },
  section: {
    marginTop: 8,
    gap: 8,
  },
  sectionTitle: {
    color: accountColors.text,
    fontSize: 16,
    fontWeight: "600",
    marginBottom: 4,
  },
  sizeRow: {
    flexDirection: "row",
    gap: 8,
  },
  sizeChip: {
    flex: 1,
    borderRadius: 12,
    paddingVertical: 10,
    alignItems: "center",
    backgroundColor: accountColors.card,
    borderWidth: 1,
    borderColor: accountColors.border,
  },
  sizeChipActive: {
    borderColor: accountColors.accent,
    backgroundColor: "#12485A",
  },
  sizeChipText: {
    color: accountColors.muted,
    fontWeight: "600",
    fontSize: 13,
    textTransform: "capitalize",
  },
  sizeChipTextActive: {
    color: accountColors.text,
  },
});
