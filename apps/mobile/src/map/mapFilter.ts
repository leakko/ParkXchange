const DEFAULT_WINDOW_MS = 2 * 60 * 60 * 1000;

export type MapFilterState = {
  from: string;
  to: string;
  includeFlexible: boolean;
  includeLeavingNow: boolean;
  leavingNowOnly: boolean;
  /** True when the user changed the day, range, or listing options. */
  isCustom: boolean;
};

export function defaultMapFilter(now = new Date()): MapFilterState {
  return {
    from: now.toISOString(),
    to: new Date(now.getTime() + DEFAULT_WINDOW_MS).toISOString(),
    includeFlexible: true,
    includeLeavingNow: true,
    leavingNowOnly: false,
    isCustom: false,
  };
}

/** Quick chip: only «Me voy ya» listings; time/flexible windows are ignored. */
export function leavingNowOnlyMapFilter(now = new Date()): MapFilterState {
  const base = defaultMapFilter(now);
  return {
    ...base,
    includeFlexible: false,
    includeLeavingNow: true,
    leavingNowOnly: true,
    isCustom: true,
  };
}

export function mapFilterFromDayRange(
  dayLocal: Date,
  startHour: number,
  startMinute: number,
  endHour: number,
  endMinute: number,
  includeFlexible: boolean,
  includeLeavingNow = true,
  leavingNowOnly = false,
): MapFilterState {
  const year = dayLocal.getFullYear();
  const month = dayLocal.getMonth();
  const day = dayLocal.getDate();

  return {
    from: new Date(year, month, day, startHour, startMinute).toISOString(),
    to: new Date(year, month, day, endHour, endMinute).toISOString(),
    includeFlexible,
    includeLeavingNow: leavingNowOnly ? true : includeLeavingNow,
    leavingNowOnly,
    isCustom: true,
  };
}

export function isValidMapFilterDayRange(
  dayLocal: Date,
  startHour: number,
  startMinute: number,
  endHour: number,
  endMinute: number,
): boolean {
  const filter = mapFilterFromDayRange(dayLocal, startHour, startMinute, endHour, endMinute, true);
  return Date.parse(filter.to) > Date.parse(filter.from);
}

export function isDefaultMapFilter(filter: MapFilterState, now = new Date()): boolean {
  const expected = defaultMapFilter(now);
  return (
    !filter.isCustom &&
    filter.includeFlexible === expected.includeFlexible &&
    filter.includeLeavingNow === expected.includeLeavingNow &&
    filter.leavingNowOnly === expected.leavingNowOnly &&
    filter.from === expected.from &&
    filter.to === expected.to
  );
}
