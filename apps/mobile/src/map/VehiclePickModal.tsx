import { Modal, Pressable, StyleSheet, Text, View } from "react-native";

import type { VehicleResponse } from "@/api/client";

type Props = {
  visible: boolean;
  vehicles: VehicleResponse[];
  onPick: (id: string) => void;
  onCancel: () => void;
};

/** Full-screen list used on Android when announcing with multiple vehicles. */
export function VehiclePickModal({ visible, vehicles, onPick, onCancel }: Props) {
  return (
    <Modal visible={visible} transparent animationType="fade" onRequestClose={onCancel}>
      <Pressable style={styles.backdrop} onPress={onCancel}>
        <Pressable style={styles.card} onPress={(e) => e.stopPropagation()}>
          <Text style={styles.title}>Which vehicle?</Text>
          <Text style={styles.sub}>Claimers will see this car at the spot.</Text>
          {vehicles.map((v) => (
            <Pressable
              key={v.id}
              style={styles.row}
              onPress={() => onPick(v.id)}
            >
              <Text style={styles.rowTitle}>
                {v.plate} · {v.make_model}
              </Text>
              <Text style={styles.rowMeta}>
                {v.color} · {v.year} · {v.size_class}
              </Text>
            </Pressable>
          ))}
          <Pressable style={styles.cancel} onPress={onCancel}>
            <Text style={styles.cancelText}>Cancel</Text>
          </Pressable>
        </Pressable>
      </Pressable>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: {
    flex: 1,
    backgroundColor: "rgba(0,0,0,0.55)",
    justifyContent: "flex-end",
  },
  card: {
    backgroundColor: "#0B1F33",
    borderTopLeftRadius: 16,
    borderTopRightRadius: 16,
    paddingHorizontal: 20,
    paddingTop: 18,
    paddingBottom: 28,
    gap: 8,
  },
  title: { color: "#F4F7FA", fontSize: 17, fontWeight: "600" },
  sub: { color: "#9DB4C0", fontSize: 13, marginBottom: 6 },
  row: {
    backgroundColor: "#16324F",
    borderRadius: 12,
    paddingVertical: 12,
    paddingHorizontal: 14,
  },
  rowTitle: { color: "#F4F7FA", fontWeight: "600", fontSize: 15 },
  rowMeta: { color: "#9DB4C0", fontSize: 13, marginTop: 2 },
  cancel: {
    marginTop: 4,
    paddingVertical: 12,
    alignItems: "center",
  },
  cancelText: { color: "#9DB4C0", fontWeight: "600", fontSize: 15 },
});
