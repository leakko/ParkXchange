import { useEffect, useState } from "react";
import {
  Image,
  StyleSheet,
  View,
  type LayoutChangeEvent,
  type StyleProp,
  type ViewStyle,
} from "react-native";

type Props = {
  uri: string;
  width: number;
  height: number;
  borderRadius?: number;
  style?: StyleProp<ViewStyle>;
};

type FillProps = {
  uri: string;
  height: number;
  borderRadius?: number;
  style?: StyleProp<ViewStyle>;
};

/**
 * Auth photos arrive as large data URIs. On Android the native Image can briefly
 * paint at intrinsic pixel size and blow Yoga layout. Keep a rigid box, mount
 * the bitmap only after layout, paint absolute + invisible until onLoad.
 */
export function FixedBoxImage({
  uri,
  width,
  height,
  borderRadius = 0,
  style,
}: Props) {
  const [mounted, setMounted] = useState(false);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    setMounted(false);
    setReady(false);
    const t = requestAnimationFrame(() => setMounted(true));
    return () => cancelAnimationFrame(t);
  }, [uri, width, height]);

  return (
    <View
      collapsable={false}
      pointerEvents="none"
      style={[
        styles.box,
        {
          width,
          height,
          borderRadius,
          maxWidth: width,
          maxHeight: height,
          minWidth: width,
          minHeight: height,
        },
        style,
      ]}
    >
      {mounted ? (
        <Image
          key={uri}
          source={{ uri, width, height }}
          style={[
            styles.img,
            {
              width,
              height,
              borderRadius,
              opacity: ready ? 1 : 0,
            },
          ]}
          resizeMode="cover"
          resizeMethod="resize"
          onLoad={() => setReady(true)}
          onLoadEnd={() => setReady(true)}
          accessibilityIgnoresInvertColors
        />
      ) : null}
    </View>
  );
}

/** Full-width variant: measures parent, then locks a numeric box. */
export function FixedHeightFillImage({
  uri,
  height,
  borderRadius = 0,
  style,
}: FillProps) {
  const [width, setWidth] = useState(0);

  const onLayout = (e: LayoutChangeEvent) => {
    const next = Math.round(e.nativeEvent.layout.width);
    if (next > 0 && next !== width) {
      setWidth(next);
    }
  };

  return (
    <View
      collapsable={false}
      onLayout={onLayout}
      style={[styles.fillHost, { height, borderRadius }, style]}
    >
      {width > 0 ? (
        <FixedBoxImage
          uri={uri}
          width={width}
          height={height}
          borderRadius={borderRadius}
        />
      ) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  box: {
    overflow: "hidden",
    backgroundColor: "#16324F",
    flexGrow: 0,
    flexShrink: 0,
  },
  fillHost: {
    width: "100%",
    overflow: "hidden",
    backgroundColor: "#16324F",
    flexGrow: 0,
    flexShrink: 0,
  },
  img: {
    position: "absolute",
    top: 0,
    left: 0,
    right: 0,
    bottom: 0,
  },
});
