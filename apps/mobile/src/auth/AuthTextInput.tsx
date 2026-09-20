import { useRef } from "react";
import { TextInput, type TextInputProps } from "react-native";

import { accountStyles } from "@/account/theme";
import { useAuthScroll } from "@/auth/AuthScroll";

type Props = TextInputProps & {
  /** Highlights the border when validation failed for this field. */
  invalid?: boolean;
};

/** TextInput that scrolls into view above the keyboard inside AuthScroll. */
export function AuthTextInput({ style, onFocus, invalid, ...rest }: Props) {
  const authScroll = useAuthScroll();
  const ref = useRef<TextInput>(null);

  return (
    <TextInput
      {...rest}
      ref={ref}
      style={[accountStyles.input, invalid && accountStyles.inputInvalid, style]}
      onFocus={(e) => {
        onFocus?.(e);
        authScroll?.ensureVisible((cb) => {
          ref.current?.measureInWindow(cb);
        });
      }}
    />
  );
}
