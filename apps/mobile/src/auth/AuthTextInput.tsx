import { useRef } from "react";
import { TextInput, type TextInputProps } from "react-native";

import { accountStyles } from "@/account/theme";
import { useAuthScroll } from "@/auth/AuthScroll";

/** TextInput that scrolls into view above the keyboard inside AuthScroll. */
export function AuthTextInput({ style, onFocus, ...rest }: TextInputProps) {
  const authScroll = useAuthScroll();
  const ref = useRef<TextInput>(null);

  return (
    <TextInput
      {...rest}
      ref={ref}
      style={[accountStyles.input, style]}
      onFocus={(e) => {
        onFocus?.(e);
        authScroll?.ensureVisible((cb) => {
          ref.current?.measureInWindow(cb);
        });
      }}
    />
  );
}
