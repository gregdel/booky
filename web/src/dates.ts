export function dateOnly(value: string): string {
  return value.slice(0, 10);
}

export function localDateString(date: Date): string {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

export function addDays(value: string, days: number): string {
  const [year, month, day] = value.split("-").map(Number);
  const date = new Date(year, month - 1, day);
  date.setDate(date.getDate() + days);
  return localDateString(date);
}

export function addDateObjectDays(value: Date, days: number): Date {
  const date = new Date(value.getTime());
  date.setDate(date.getDate() + days);
  return date;
}

export function exclusiveEndToInclusiveLastDay(end: string): string {
  return addDays(end, -1);
}

export function inclusiveLastDayToExclusiveEnd(lastDay: string): string {
  return addDays(lastDay, 1);
}

export function calendarExclusiveEndToStoredEnd(end: string): string {
  return dateOnly(end);
}
