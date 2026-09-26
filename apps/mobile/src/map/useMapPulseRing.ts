import { useEffect, useState } from "react";
import { AppState } from "react-native";

const PERIOD_MS = 1800;
const TICK_MS = 160;
const MIN_RADIUS = 16;
const MAX_RADIUS = 26;
const MAX_OPACITY = 0.42;
const MIN_OPACITY = 0.05;

const REST = {
  radius: MIN_RADIUS,
  opacity: MAX_OPACITY * 0.55,
};

/**
 * Soft expanding ring for the active exchange pin.
 * Only ticks while enabled and the app is foregrounded — constant MapLibre
 * paint updates every frame freeze emulators.
 */
export function useMapPulseRing(enabled: boolean): { radius: number; opacity: number } {
  const [frame, setFrame] = useState(0);

  useEffect(() => {
    if (!enabled) {
      return;
    }
    let id: ReturnType<typeof setInterval> | null = null;
    const start = () => {
      if (id != null) {
        return;
      }
      id = setInterval(() => {
        setFrame((n) => n + 1);
      }, TICK_MS);
    };
    const stop = () => {
      if (id != null) {
        clearInterval(id);
        id = null;
      }
    };
    if (AppState.currentState === "active") {
      start();
    }
    const sub = AppState.addEventListener("change", (state) => {
      if (state === "active") {
        start();
      } else {
        stop();
      }
    });
    return () => {
      stop();
      sub.remove();
    };
  }, [enabled]);

  if (!enabled) {
    return REST;
  }

  const t = ((frame * TICK_MS) % PERIOD_MS) / PERIOD_MS;
  const ease = 1 - (1 - t) * (1 - t);
  return {
    radius: MIN_RADIUS + (MAX_RADIUS - MIN_RADIUS) * ease,
    opacity: MAX_OPACITY - (MAX_OPACITY - MIN_OPACITY) * ease,
  };
}
