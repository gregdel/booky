import { describe, expect, test } from "bun:test";

import {
  calendarExclusiveEndToStoredEnd,
  exclusiveEndToInclusiveLastDay,
  inclusiveLastDayToExclusiveEnd,
} from "./dates";

describe("booking date conversions", () => {
  test("displays stored exclusive end as inclusive last day", () => {
    expect(exclusiveEndToInclusiveLastDay("2026-07-12")).toBe("2026-07-11");
  });

  test("sends inclusive form last day as exclusive end", () => {
    expect(inclusiveLastDayToExclusiveEnd("2026-07-11")).toBe("2026-07-12");
  });

  test("saving an unchanged dialog round-trips the stored exclusive end", () => {
    const storedEnd = "2026-07-12";
    const displayedLastDay = exclusiveEndToInclusiveLastDay(storedEnd);

    expect(inclusiveLastDayToExclusiveEnd(displayedLastDay)).toBe(storedEnd);
  });

  test("drag and resize should keep FullCalendar exclusive end unchanged", () => {
    expect(calendarExclusiveEndToStoredEnd("2026-07-12")).toBe("2026-07-12");
  });
});
