import { Ionicons } from "@expo/vector-icons";
import { useRef, useState } from "react";
import {
  Platform,
  Pressable,
  StyleSheet,
  TextInput,
  View,
  type TextInputProps,
} from "react-native";

import { accountColors, accountStyles } from "@/account/theme";
import { useAuthScroll } from "@/auth/AuthScroll";
import { useTranslation } from "@/i18n";

type PasswordFieldProps = Omit<
  TextInputProps,
  "secureTextEntry" | "value" | "onChangeText"
> & {
  value: string;
  onChangeText: (value: string) => void;
  invalid?: boolean;
};

/**
 * Password input with show/hide toggle. On Android, native secureTextEntry
 * briefly reveals the last typed character; when hidden we mask with bullets
 * instead so nothing flashes.
 */
export function PasswordField({
  value,
  onChangeText,
  style,
  onFocus,
  invalid,
  ...rest
}: PasswordFieldProps) {
  const { t } = useTranslation();
  const authScroll = useAuthScroll();
  const wrapRef = useRef<View>(null);
  const [visible, setVisible] = useState(false);
  const maskNative = Platform.OS === "ios" && !visible;

  const displayValue =
    visible || Platform.OS === "ios" ? value : "•".repeat(value.length);

  const onMaskedChange = (text: string) => {
    if (visible || Platform.OS === "ios") {
      onChangeText(text);
      return;
    }
    // Bullet mask: treat edits as length changes from the end (typical typing).
    if (text.length < value.length) {
      onChangeText(value.slice(0, text.length));
      return;
    }
    const added = text.slice(value.length).split("•").join("");
    onChangeText(value + added);
  };

  return (
    <View
      ref={wrapRef}
      style={styles.wrap}
      collapsable={false}
      onLayout={() => {
        /* keeps measureInWindow accurate after keyboard resize */
      }}
    >
      <TextInput
        {...rest}
        style={[
          accountStyles.input,
          invalid && accountStyles.inputInvalid,
          styles.input,
          style,
        ]}
        value={displayValue}
        onChangeText={onMaskedChange}
        secureTextEntry={maskNative}
        autoCapitalize="none"
        autoCorrect={false}
        textContentType="password"
        autoComplete="password"
        onFocus={(e) => {
          onFocus?.(e);
          authScroll?.ensureVisible((cb) => {
            wrapRef.current?.measureInWindow(cb);
          });
        }}
      />
      <Pressable
        style={styles.toggle}
        onPress={() => setVisible((v) => !v)}
        accessibilityRole="button"
        accessibilityLabel={
          visible ? t("auth.password.hide") : t("auth.password.show")
        }
        hitSlop={8}
      >
        <Ionicons
          name={visible ? "eye-off-outline" : "eye-outline"}
          size={22}
          color={accountColors.muted}
        />
      </Pressable>
    </View>
  );
}

const styles = StyleSheet.create({
  wrap: {
    position: "relative",
    justifyContent: "center",
  },
  input: {
    paddingRight: 48,
  },
  toggle: {
    position: "absolute",
    right: 12,
    height: "100%",
    justifyContent: "center",
  },
});
