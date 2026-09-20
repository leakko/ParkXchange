import type { ReactNode } from "react";
import {
  KeyboardAvoidingView,
  Platform,
  ScrollView,
  type StyleProp,
  type ViewStyle,
} from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";

import { accountStyles } from "@/account/theme";

type AuthScrollProps = {
  children: ReactNode;
  contentContainerStyle?: StyleProp<ViewStyle>;
};

/** Scrollable form that lifts with the keyboard so bottom fields stay visible. */
export function AuthScroll({ children, contentContainerStyle }: AuthScrollProps) {
  const insets = useSafeAreaInsets();
  // Stack header (~56) sits above the form; without this offset iOS pads too little.
  const headerOffset = 56;

  return (
    <KeyboardAvoidingView
      style={accountStyles.screen}
      behavior="padding"
      keyboardVerticalOffset={headerOffset + (Platform.OS === "ios" ? insets.top : 0)}
    >
      <ScrollView
        style={accountStyles.screen}
        contentContainerStyle={[
          accountStyles.scroll,
          { paddingBottom: 48 + insets.bottom },
          contentContainerStyle,
        ]}
        keyboardShouldPersistTaps="handled"
        keyboardDismissMode="on-drag"
      >
        {children}
      </ScrollView>
    </KeyboardAvoidingView>
  );
}
