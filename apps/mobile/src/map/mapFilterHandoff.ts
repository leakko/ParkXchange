import type { MapFilterState } from "@/map/mapFilter";
import { defaultMapFilter } from "@/map/mapFilter";

type Listener = (filter: MapFilterState) => void;

/**
 * Live map filter snapshot for the filter form sheet. The map publishes the
 * current value before opening `/filter`; the sheet publishes on Apply/Reset
 * so the map updates discovery without remounting.
 */
let current: MapFilterState = defaultMapFilter();
const listeners = new Set<Listener>();

export function stageMapFilter(filter: MapFilterState): void {
  current = filter;
}

export function peekMapFilter(): MapFilterState {
  return current;
}

export function publishMapFilter(filter: MapFilterState): void {
  current = filter;
  for (const listener of listeners) {
    listener(filter);
  }
}

export function subscribeMapFilter(listener: Listener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}
