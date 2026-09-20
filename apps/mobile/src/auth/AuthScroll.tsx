import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  Dimensions,
  Keyboard,
  KeyboardAvoidingView,
  Platform,
  ScrollView,
  View,
  type NativeSyntheticEvent,
  type NativeScrollEvent,
  type StyleProp,
  type ViewStyle,
} from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";

import { accountStyles } from "@/account/theme";

type MeasureInWindow = (
  callback: (x: number, y: number, width: number, height: number) => void,
) => void;

type AuthScrollContextValue = {
  /** Scroll so the focused control sits above the keyboard. */
  ensureVisible: (measureInWindow: MeasureInWindow) => void;
};

const AuthScrollContext = createContext<AuthScrollContextValue | null>(null);

export function useAuthScroll(): AuthScrollContextValue | null {
  return useContext(AuthScrollContext);
}

type AuthScrollProps = {
  children: ReactNode;
  contentContainerStyle?: StyleProp<ViewStyle>;
};

/**
 * Form scroll that keeps the focused field above the keyboard.
 * Relies on measureInWindow + explicit scroll (adjustResize alone often fails
 * with React Navigation + edge-to-edge Android).
 */
export function AuthScroll({ children, contentContainerStyle }: AuthScrollProps) {
  const insets = useSafeAreaInsets();
  const scrollRef = useRef<ScrollView>(null);
  const scrollYRef = useRef(0);
  const pendingMeasure = useRef<MeasureInWindow | null>(null);
  const keyboardBottomRef = useRef(0);
  const [keyboardBottom, setKeyboardBottom] = useState(0);
  const headerOffset = 56;

  const scrollFieldAboveKeyboard = useCallback((measureInWindow: MeasureInWindow) => {
    measureInWindow((x, y, width, height) => {
      const winH = Dimensions.get("window").height;
      const kb =
        keyboardBottomRef.current || Keyboard.metrics()?.height || 0;
      if (kb <= 0) {
        return;
      }
      // Leave room under the focused field for the next input / primary button.
      const visibleBottom = winH - kb - 24;
      const fieldBottom = y + height + 140;
      if (fieldBottom <= visibleBottom) {
        return;
      }
      const delta = fieldBottom - visibleBottom;
      scrollRef.current?.scrollTo({
        y: Math.max(0, scrollYRef.current + delta),
        animated: true,
      });
    });
  }, []);

  useEffect(() => {
    const showEvent = Platform.OS === "ios" ? "keyboardWillShow" : "keyboardDidShow";
    const hideEvent = Platform.OS === "ios" ? "keyboardWillHide" : "keyboardDidHide";
    const onShow = Keyboard.addListener(showEvent, (e) => {
      keyboardBottomRef.current = e.endCoordinates.height;
      setKeyboardBottom(e.endCoordinates.height);
      if (pendingMeasure.current) {
        // Focus often fires before keyboard metrics exist; retry now.
        setTimeout(() => {
          if (pendingMeasure.current) {
            scrollFieldAboveKeyboard(pendingMeasure.current);
          }
        }, 50);
      }
    });
    const onHide = Keyboard.addListener(hideEvent, () => {
      keyboardBottomRef.current = 0;
      setKeyboardBottom(0);
      pendingMeasure.current = null;
    });
    return () => {
      onShow.remove();
      onHide.remove();
    };
  }, [scrollFieldAboveKeyboard]);

  const ensureVisible = useCallback(
    (measureInWindow: MeasureInWindow) => {
      pendingMeasure.current = measureInWindow;
      requestAnimationFrame(() => {
        setTimeout(() => scrollFieldAboveKeyboard(measureInWindow), 100);
      });
    },
    [scrollFieldAboveKeyboard],
  );

  const ctx = useMemo(() => ({ ensureVisible }), [ensureVisible]);

  const onScroll = (e: NativeSyntheticEvent<NativeScrollEvent>) => {
    scrollYRef.current = e.nativeEvent.contentOffset.y;
  };

  const bottomPad = 48 + insets.bottom + (keyboardBottom > 0 ? keyboardBottom : 0);

  const scroll = (
    <ScrollView
      ref={scrollRef}
      style={accountStyles.screen}
      contentContainerStyle={[
        accountStyles.scroll,
        { paddingBottom: bottomPad },
        contentContainerStyle,
      ]}
      keyboardShouldPersistTaps="handled"
      keyboardDismissMode="on-drag"
      onScroll={onScroll}
      scrollEventThrottle={16}
    >
      <AuthScrollContext.Provider value={ctx}>{children}</AuthScrollContext.Provider>
    </ScrollView>
  );

  if (Platform.OS === "android") {
    return <View style={accountStyles.screen}>{scroll}</View>;
  }

  return (
    <KeyboardAvoidingView
      style={accountStyles.screen}
      behavior="padding"
      keyboardVerticalOffset={headerOffset + insets.top}
    >
      {scroll}
    </KeyboardAvoidingView>
  );
}
