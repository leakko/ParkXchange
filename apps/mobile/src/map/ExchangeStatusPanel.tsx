import { StyleSheet, Text, View } from "react-native";

import { useTranslation, type TranslationKey } from "@/i18n";
import {
  myHandshakePhase,
  peerPhase,
  type HandshakeFields,
  type PeerPhase,
} from "@/map/exchangeLeave";

type Props = {
  res: HandshakeFields;
  iAmOwner: boolean;
  deadlineLabel?: string | null;
};

function youPhaseKey(iAmOwner: boolean, phase: PeerPhase): TranslationKey {
  const role = iAmOwner ? "youOwner" : "youDriver";
  if (phase === "ready") {
    return `exchange.statusPanel.${role}.ready` as TranslationKey;
  }
  if (phase === "en_route") {
    return `exchange.statusPanel.${role}.en_route` as TranslationKey;
  }
  return `exchange.statusPanel.${role}.idle` as TranslationKey;
}

function themPhaseKey(iAmOwner: boolean, phase: PeerPhase): TranslationKey {
  // Looking at the other party: owner sees driver, driver sees owner.
  const role = iAmOwner ? "themDriver" : "themOwner";
  if (phase === "ready") {
    return `exchange.statusPanel.${role}.ready` as TranslationKey;
  }
  if (phase === "en_route") {
    return `exchange.statusPanel.${role}.en_route` as TranslationKey;
  }
  return `exchange.statusPanel.${role}.idle` as TranslationKey;
}

export function ExchangeStatusPanel({ res, iAmOwner, deadlineLabel }: Props) {
  const { t } = useTranslation();
  const mine = myHandshakePhase(res, iAmOwner);
  const theirs = peerPhase(res, iAmOwner);
  const themLabel = iAmOwner
    ? t("exchange.statusPanel.themDriver")
    : t("exchange.statusPanel.themOwner");

  return (
    <View style={styles.box}>
      <Text style={styles.title}>{t("exchange.statusPanel.title")}</Text>
      <View style={styles.row}>
        <Text style={styles.who}>{t("exchange.statusPanel.you")}</Text>
        <Text style={styles.phase}>{t(youPhaseKey(iAmOwner, mine))}</Text>
      </View>
      <View style={styles.row}>
        <Text style={styles.who}>{themLabel}</Text>
        <Text style={styles.phase}>{t(themPhaseKey(iAmOwner, theirs))}</Text>
      </View>
      {deadlineLabel ? <Text style={styles.deadline}>{deadlineLabel}</Text> : null}
    </View>
  );
}

const styles = StyleSheet.create({
  box: {
    backgroundColor: "#0F2740",
    borderWidth: 1,
    borderColor: "#1F3A56",
    borderRadius: 12,
    paddingHorizontal: 12,
    paddingVertical: 10,
    gap: 8,
  },
  title: {
    color: "#9DB4C0",
    fontSize: 12,
    fontWeight: "700",
    textTransform: "uppercase",
    letterSpacing: 0.4,
  },
  row: {
    flexDirection: "row",
    alignItems: "flex-start",
    gap: 10,
  },
  // Fixed width so “Tú” and “Quien reservó” / “Quien deja el hueco” align.
  who: {
    color: "#1B9AAA",
    fontSize: 13,
    fontWeight: "700",
    width: 128,
    flexShrink: 0,
  },
  phase: {
    color: "#F4F7FA",
    fontSize: 14,
    fontWeight: "600",
    flex: 1,
    lineHeight: 20,
  },
  deadline: {
    color: "#7A93A0",
    fontSize: 12,
    marginTop: 2,
  },
});
