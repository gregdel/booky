import { Calendar, type EventApi, type EventInput, type EventSourceFuncArg } from "fullcalendar";
import {
  addDateObjectDays,
  calendarExclusiveEndToStoredEnd,
  dateOnly,
  exclusiveEndToInclusiveLastDay,
  inclusiveLastDayToExclusiveEnd,
  localDateString,
} from "./dates";
import {
  ApiError,
  deleteBookingPayload,
  errorMessage,
  mutationFailureMessage,
  shouldRefetchAfterMutationFailure,
  updateBookingPayload,
  type Booking,
} from "./bookings";

const API_BASE = `${scriptBasePath()}/api`;
const EVENT_PALETTE = [
  "#174A63",
  "#1F6F6B",
  "#2F6B4F",
  "#7A5B2E",
  "#A94838",
  "#66518A",
  "#59612F",
  "#7A4D3D",
] as const;
const EVENT_TEXT_COLOR = "#ffffff";
const LAST_DAY_ERROR = "Last day must be on or after start.";
const MOBILE_CALENDAR_QUERY = "(max-width: 720px)";
const MOBILE_CALENDAR_MIN_HEIGHT = 360;
const MOBILE_CALENDAR_BOTTOM_GAP = 12;

type BookingEvent = EventApi & {
  extendedProps: {
    etag?: string;
    note?: string;
  };
};

const statusEl = requireElement<HTMLElement>("status");
const addButton = requireElement<HTMLButtonElement>("add-booking");
const dialog = requireElement<HTMLDialogElement>("booking-dialog");
const form = requireElement<HTMLFormElement>("booking-form");
const dialogTitle = requireElement<HTMLHeadingElement>("dialog-title");
const closeButton = requireElement<HTMLButtonElement>("close-dialog");
const cancelButton = requireElement<HTMLButtonElement>("cancel-booking");
const deleteButton = requireElement<HTMLButtonElement>("delete-booking");
const saveButton = requireElement<HTMLButtonElement>("save-booking");
const formError = requireElement<HTMLElement>("form-error");
const nameInput = requireElement<HTMLInputElement>("booking-name");
const startInput = requireElement<HTMLInputElement>("booking-start");
const endInput = requireElement<HTMLInputElement>("booking-end");
const noteInput = requireElement<HTMLTextAreaElement>("booking-note");

let calendar: Calendar;
let activeBooking: Booking | null = null;
let requestCalendarSizingUpdate = () => {};

document.addEventListener("DOMContentLoaded", () => {
  const calendarEl = requireElement<HTMLElement>("calendar");
  const mobileCalendarMedia = window.matchMedia(MOBILE_CALENDAR_QUERY);
  const calendarHeight = calendarHeightOption(calendarEl, mobileCalendarMedia);

  calendar = new Calendar(calendarEl, {
    initialView: "dayGridMonth",
    height: calendarHeight,
    firstDay: 1,
    selectable: true,
    editable: true,
    eventResizableFromStart: true,
    dayMaxEvents: true,
    displayEventTime: false,
    events: loadEvents,
    select: (selection) => {
      openDialog(
        bookingToDialogValues({
          name: "",
          start: dateOnly(selection.startStr),
          end: calendarExclusiveEndToStoredEnd(selection.endStr),
        }),
      );
      calendar.unselect();
    },
    eventClick: (info) => {
      openDialog(bookingToDialogValues(eventToStoredBooking(info.event as BookingEvent)));
    },
    eventDrop: async (info) => {
      await moveBooking(info.event as BookingEvent, info.revert);
    },
    eventResize: async (info) => {
      await moveBooking(info.event as BookingEvent, info.revert);
    },
    loading: (isLoading) => {
      if (isLoading) {
        setStatus("Loading bookings");
      }
    },
  });

  requestCalendarSizingUpdate = bindCalendarSizing(calendarEl, mobileCalendarMedia, calendar, calendarHeight);
  calendar.render();
  bindControls();
});

function bindCalendarSizing(
  calendarEl: HTMLElement,
  media: MediaQueryList,
  calendar: Calendar,
  initialHeight: number | "auto",
): () => void {
  let calendarHeight = initialHeight;
  let pendingFrame = 0;

  const requestUpdate = () => {
    if (pendingFrame !== 0) {
      return;
    }
    pendingFrame = window.requestAnimationFrame(() => {
      pendingFrame = 0;
      const nextHeight = calendarHeightOption(calendarEl, media);
      if (nextHeight !== calendarHeight) {
        calendarHeight = nextHeight;
        calendar.setOption("height", nextHeight);
      }
    });
  };

  window.addEventListener("resize", requestUpdate);
  window.visualViewport?.addEventListener("resize", requestUpdate);
  window.visualViewport?.addEventListener("scroll", requestUpdate);
  media.addEventListener("change", requestUpdate);
  requestUpdate();

  return requestUpdate;
}

function calendarHeightOption(calendarEl: HTMLElement, media: MediaQueryList): number | "auto" {
  if (!media.matches) {
    return "auto";
  }

  const viewportHeight = window.visualViewport?.height ?? window.innerHeight;
  const availableHeight =
    viewportHeight - calendarEl.getBoundingClientRect().top - MOBILE_CALENDAR_BOTTOM_GAP;

  return Math.max(MOBILE_CALENDAR_MIN_HEIGHT, Math.floor(availableHeight));
}

function bindControls(): void {
  addButton.addEventListener("click", () => {
    const today = localDateString(new Date());
    openDialog({ name: "", start: today, end: today });
  });

  closeButton.addEventListener("click", closeDialog);
  cancelButton.addEventListener("click", closeDialog);
  deleteButton.addEventListener("click", () => {
    void deleteActiveBooking();
  });

  form.addEventListener("submit", (event) => {
    event.preventDefault();
    void saveBooking();
  });

  startInput.addEventListener("input", updateLastDayConstraint);
  endInput.addEventListener("input", updateLastDayConstraint);

  dialog.addEventListener("close", () => {
    formError.textContent = "";
    endInput.setCustomValidity("");
  });
}

async function loadEvents(
  info: EventSourceFuncArg,
  success: (events: EventInput[]) => void,
  failure: (error: Error) => void,
): Promise<void> {
  try {
    const start = dateOnly(info.startStr);
    const end = dateOnly(info.endStr);
    const bookings = await apiFetch<Booking[]>(
      `/bookings?start=${encodeURIComponent(start)}&end=${encodeURIComponent(end)}`,
    );
    success(bookings.map(bookingToEvent));
    setStatus(`${bookings.length} booking${bookings.length === 1 ? "" : "s"}`);
  } catch (error) {
    const message = errorMessage(error);
    setStatus(message, true);
    failure(error instanceof Error ? error : new Error(message));
  }
}

function bookingToEvent(booking: Booking): EventInput {
  return {
    id: booking.uid,
    title: booking.name,
    start: booking.start,
    end: booking.end,
    allDay: true,
    ...eventColorsForName(booking.name),
    extendedProps: {
      etag: booking.etag || "",
      note: booking.note || "",
    },
  };
}

async function moveBooking(event: BookingEvent, revert: () => void): Promise<void> {
  const booking = eventToStoredBooking(event);
  if (!booking.uid) {
    revert();
    setStatus("Booking is missing an id", true);
    return;
  }

  try {
    const updated = await apiFetch<Booking>(`/bookings/${encodeURIComponent(booking.uid)}`, {
      method: "PUT",
      body: JSON.stringify(updateBookingPayload(booking, booking.uid)),
    });
    updateEvent(event, updated);
    setStatus("Booking saved");
  } catch (error) {
    revert();
    setStatus(mutationFailureMessage("save", error), true);
    calendar.refetchEvents();
  }
}

function openDialog(booking: Booking): void {
  activeBooking = booking.uid ? { ...booking } : null;
  dialogTitle.textContent = booking.uid ? "Edit booking" : "New booking";
  nameInput.value = booking.name || "";
  startInput.value = booking.start || "";
  endInput.value = booking.end || "";
  noteInput.value = booking.note || "";
  deleteButton.hidden = !booking.uid;
  formError.textContent = "";
  updateLastDayConstraint();
  dialog.showModal();
  nameInput.focus();
}

function closeDialog(): void {
  dialog.close();
}

async function saveBooking(): Promise<void> {
  clearFormError();
  if (!validateLastDay()) {
    return;
  }
  const booking = formBooking();
  const isEdit = Boolean(activeBooking?.uid);
  const path = isEdit ? `/bookings/${encodeURIComponent(activeBooking!.uid!)}` : "/bookings";

  try {
    setBusy(true);
    await apiFetch<Booking>(path, {
      method: isEdit ? "PUT" : "POST",
      body: JSON.stringify(isEdit ? updateBookingPayload(booking, activeBooking!.uid!) : booking),
    });
    closeDialog();
    setStatus("Booking saved");
    calendar.refetchEvents();
  } catch (error) {
    formError.textContent = isEdit ? mutationFailureMessage("save", error) : errorMessage(error);
    if (isEdit && shouldRefetchAfterMutationFailure(error)) {
      calendar.refetchEvents();
    }
  } finally {
    setBusy(false);
  }
}

async function deleteActiveBooking(): Promise<void> {
  if (!activeBooking?.uid) {
    return;
  }

  clearFormError();
  try {
    setBusy(true);
    await apiFetch<null>(`/bookings/${encodeURIComponent(activeBooking.uid)}`, {
      method: "DELETE",
      body: JSON.stringify(deleteBookingPayload(activeBooking)),
    });
    calendar.getEventById(activeBooking.uid)?.remove();
    closeDialog();
    setStatus("Booking deleted");
    calendar.refetchEvents();
  } catch (error) {
    formError.textContent = mutationFailureMessage("delete", error);
    if (shouldRefetchAfterMutationFailure(error)) {
      calendar.refetchEvents();
    }
  } finally {
    setBusy(false);
  }
}

function formBooking(): Booking {
  const booking: Booking = {
    name: nameInput.value.trim(),
    start: startInput.value,
    end: inclusiveLastDayToExclusiveEnd(endInput.value),
    note: noteInput.value.trim(),
  };

  if (activeBooking) {
    booking.etag = activeBooking.etag || "";
  }

  return booking;
}

function eventToStoredBooking(event: BookingEvent): Booking {
  return {
    uid: event.id,
    etag: event.extendedProps.etag || "",
    name: event.title,
    start: dateOnly(event.startStr || localDateString(requiredDate(event.start))),
    end: calendarExclusiveEndToStoredEnd(
      event.endStr || localDateString(addDateObjectDays(requiredDate(event.start), 1)),
    ),
    note: event.extendedProps.note || "",
  };
}

function bookingToDialogValues(booking: Booking): Booking {
  return {
    ...booking,
    end: exclusiveEndToInclusiveLastDay(booking.end),
  };
}

function updateEvent(event: BookingEvent, booking: Booking): void {
  const colors = eventColorsForName(booking.name);
  event.setProp("title", booking.name);
  event.setProp("backgroundColor", colors.backgroundColor);
  event.setProp("borderColor", colors.borderColor);
  event.setProp("textColor", colors.textColor);
  event.setStart(booking.start);
  event.setEnd(booking.end);
  event.setExtendedProp("etag", booking.etag || "");
  event.setExtendedProp("note", booking.note || "");
}

function eventColorsForName(name: string): {
  backgroundColor: string;
  borderColor: string;
  textColor: string;
} {
  const color = EVENT_PALETTE[hashString(normalizedColorKey(name)) % EVENT_PALETTE.length];
  return {
    backgroundColor: color,
    borderColor: color,
    textColor: EVENT_TEXT_COLOR,
  };
}

function normalizedColorKey(name: string): string {
  return name.normalize("NFKC").trim().replace(/\s+/g, " ").toLowerCase();
}

function hashString(value: string): number {
  let hash = 0x811c9dc5;
  for (let i = 0; i < value.length; i += 1) {
    hash ^= value.charCodeAt(i);
    hash = Math.imul(hash, 0x01000193);
  }
  return hash >>> 0;
}

async function apiFetch<T>(path: string, options: RequestInit = {}): Promise<T> {
  const headers = {
    Accept: "application/json",
    ...(options.body ? { "Content-Type": "application/json" } : {}),
    ...(options.headers || {}),
  };
  const response = await fetch(`${API_BASE}${path}`, { ...options, headers });
  if (response.status === 204) {
    return null as T;
  }

  const payload = await readJSON(response);
  if (!response.ok) {
    throw new ApiError(response.status, payload.error || `Request failed with HTTP ${response.status}`);
  }
  return payload as T;
}

async function readJSON(response: Response): Promise<{ error?: string } & Record<string, unknown>> {
  const text = await response.text();
  if (!text) {
    return {};
  }
  try {
    return JSON.parse(text) as { error?: string } & Record<string, unknown>;
  } catch {
    return { error: "Server returned invalid JSON" };
  }
}

function setStatus(message: string, isError = false): void {
  statusEl.textContent = message;
  statusEl.dataset.state = isError ? "error" : "ok";
  requestCalendarSizingUpdate();
}

function clearFormError(): void {
  formError.textContent = "";
}

function setBusy(isBusy: boolean): void {
  saveButton.disabled = isBusy;
  deleteButton.disabled = isBusy;
}

function updateLastDayConstraint(): void {
  endInput.min = startInput.value;
  if (!startInput.value || !endInput.value || endInput.value >= startInput.value) {
    endInput.setCustomValidity("");
    if (formError.textContent === LAST_DAY_ERROR) {
      formError.textContent = "";
    }
    return;
  }
  endInput.setCustomValidity(LAST_DAY_ERROR);
}

function validateLastDay(): boolean {
  updateLastDayConstraint();
  if (!startInput.value || !endInput.value || endInput.value >= startInput.value) {
    return true;
  }
  formError.textContent = LAST_DAY_ERROR;
  endInput.reportValidity();
  return false;
}

function requiredDate(value: Date | null): Date {
  if (!value) {
    throw new Error("Booking is missing a date");
  }
  return value;
}

function requireElement<T extends HTMLElement>(id: string): T {
  const element = document.getElementById(id);
  if (!element) {
    throw new Error(`Missing element #${id}`);
  }
  return element as T;
}

function scriptBasePath(): string {
  const scriptURL = import.meta.url;
  if (!scriptURL) {
    return "";
  }

  const path = new URL(scriptURL).pathname;
  return path.endsWith("/app.js") ? path.slice(0, -"/app.js".length) : "";
}
