export type CalendarHeight = number | "auto";

type CalendarHeightInput = {
  isMobile: boolean;
  viewportHeight: number;
  calendarViewportTop: number;
  scrollY: number;
  minHeight: number;
  bottomGap: number;
};

export function calendarHeightOption({
  isMobile,
  viewportHeight,
  calendarViewportTop,
  scrollY,
  minHeight,
  bottomGap,
}: CalendarHeightInput): CalendarHeight {
  if (!isMobile) {
    return "auto";
  }

  const calendarPageTop = calendarViewportTop + scrollY;
  const availableHeight = viewportHeight - calendarPageTop - bottomGap;

  return Math.max(minHeight, Math.floor(availableHeight));
}
