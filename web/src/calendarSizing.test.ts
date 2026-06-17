import { describe, expect, test } from "bun:test";

import { calendarHeightOption } from "./calendarSizing";

const DEFAULT_INPUT = {
  isMobile: true,
  viewportHeight: 700,
  calendarViewportTop: 120,
  scrollY: 0,
  minHeight: 360,
  bottomGap: 12,
};

describe("calendar sizing", () => {
  test("desktop uses FullCalendar's automatic height", () => {
    expect(calendarHeightOption({ ...DEFAULT_INPUT, isMobile: false })).toBe("auto");
  });

  test("mobile enforces a minimum height", () => {
    expect(
      calendarHeightOption({
        ...DEFAULT_INPUT,
        viewportHeight: 420,
        calendarViewportTop: 100,
      }),
    ).toBe(360);
  });

  test("mobile height does not grow during normal page scroll", () => {
    const initialHeight = calendarHeightOption(DEFAULT_INPUT);
    const scrolledHeight = calendarHeightOption({
      ...DEFAULT_INPUT,
      calendarViewportTop: -180,
      scrollY: 300,
    });

    expect(scrolledHeight).toBe(initialHeight);
  });

  test("mobile height changes when the viewport height changes", () => {
    expect(calendarHeightOption({ ...DEFAULT_INPUT, viewportHeight: 760 })).toBe(628);
  });
});
