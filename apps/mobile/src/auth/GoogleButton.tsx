import { GoogleSigninButton } from "@react-native-google-signin/google-signin";
import { StyleSheet, View } from "react-native";

type Props = {
  disabled?: boolean;
  onPress: () => void;
};

/** Official Google "G" icon button (not the wide text CTA). */
export function GoogleButton({ disabled = false, onPress }: Props) {
  return (
    <View style={styles.wrap}>
      <GoogleSigninButton
        size={GoogleSigninButton.Size.Icon}
        color={GoogleSigninButton.Color.Light}
        disabled={disabled}
        onPress={onPress}
        style={styles.button}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  wrap: {
    marginTop: 8,
    alignItems: "center",
  },
  button: {
    width: 48,
    height: 48,
  },
});
