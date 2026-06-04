import { Calendar, type EventApi, type EventInput, type EventSourceFuncArg } from "fullcalendar";

const API_BASE = "/api";

type Booking = {
  uid?: string;
  href?: string;
  etag?: string;
  name: string;
  start: string;
  end: string;
  note?: string;
};

type BookingEvent = EventApi & {
  extendedProps: {
    href?: string;
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

document.addEventListener("DOMContentLoaded", () => {
  calendar = new Calendar(requireElement<HTMLElement>("calendar"), {
    initialView: "dayGridMonth",
    height: "auto",
    firstDay: 1,
    selectable: true,
    editable: true,
    eventResizableFromStart: true,
    dayMaxEvents: true,
    displayEventTime: false,
    events: loadEvents,
    select: (selection) => {
      openDialog({
        name: "",
        start: dateOnly(selection.startStr),
        end: dateOnly(selection.endStr),
      });
      calendar.unselect();
    },
    eventClick: (info) => {
      openDialog(eventToBooking(info.event as BookingEvent));
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

  calendar.render();
  bindControls();
});

function bindControls(): void {
  addButton.addEventListener("click", () => {
    const today = localDateString(new Date());
    const tomorrow = addDays(today, 1);
    openDialog({ name: "", start: today, end: tomorrow });
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

  dialog.addEventListener("close", () => {
    formError.textContent = "";
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
    extendedProps: {
      href: booking.href || "",
      etag: booking.etag || "",
      note: booking.note || "",
    },
  };
}

async function moveBooking(event: BookingEvent, revert: () => void): Promise<void> {
  const booking = eventToBooking(event);
  if (!booking.uid) {
    revert();
    setStatus("Booking is missing an id", true);
    return;
  }

  try {
    const updated = await apiFetch<Booking>(`/bookings/${encodeURIComponent(booking.uid)}`, {
      method: "PUT",
      body: JSON.stringify(booking),
    });
    updateEvent(event, updated);
    setStatus("Booking saved");
  } catch (error) {
    revert();
    setStatus(errorMessage(error), true);
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
  dialog.showModal();
  nameInput.focus();
}

function closeDialog(): void {
  dialog.close();
}

async function saveBooking(): Promise<void> {
  clearFormError();
  const booking = formBooking();
  const isEdit = Boolean(activeBooking?.uid);
  const path = isEdit ? `/bookings/${encodeURIComponent(activeBooking!.uid!)}` : "/bookings";

  try {
    setBusy(true);
    const saved = await apiFetch<Booking>(path, {
      method: isEdit ? "PUT" : "POST",
      body: JSON.stringify(isEdit ? { ...booking, uid: activeBooking!.uid } : booking),
    });
    closeDialog();
    setStatus("Booking saved");
    calendar.refetchEvents();
  } catch (error) {
    formError.textContent = errorMessage(error);
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
      body: JSON.stringify({
        href: activeBooking.href || "",
        etag: activeBooking.etag || "",
      }),
    });
    calendar.getEventById(activeBooking.uid)?.remove();
    closeDialog();
    setStatus("Booking deleted");
    calendar.refetchEvents();
  } catch (error) {
    formError.textContent = errorMessage(error);
  } finally {
    setBusy(false);
  }
}

function formBooking(): Booking {
  const booking: Booking = {
    name: nameInput.value.trim(),
    start: startInput.value,
    end: endInput.value,
    note: noteInput.value.trim(),
  };

  if (activeBooking) {
    booking.href = activeBooking.href || "";
    booking.etag = activeBooking.etag || "";
  }

  return booking;
}

function eventToBooking(event: BookingEvent): Booking {
  return {
    uid: event.id,
    href: event.extendedProps.href || "",
    etag: event.extendedProps.etag || "",
    name: event.title,
    start: dateOnly(event.startStr || localDateString(requiredDate(event.start))),
    end: dateOnly(event.endStr || localDateString(addDateObjectDays(requiredDate(event.start), 1))),
    note: event.extendedProps.note || "",
  };
}

function updateEvent(event: BookingEvent, booking: Booking): void {
  event.setProp("title", booking.name);
  event.setStart(booking.start);
  event.setEnd(booking.end);
  event.setExtendedProp("href", booking.href || "");
  event.setExtendedProp("etag", booking.etag || "");
  event.setExtendedProp("note", booking.note || "");
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
    throw new Error(payload.error || `Request failed with HTTP ${response.status}`);
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
}

function clearFormError(): void {
  formError.textContent = "";
}

function setBusy(isBusy: boolean): void {
  saveButton.disabled = isBusy;
  deleteButton.disabled = isBusy;
}

function dateOnly(value: string): string {
  return value.slice(0, 10);
}

function localDateString(date: Date): string {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

function addDays(value: string, days: number): string {
  const [year, month, day] = value.split("-").map(Number);
  const date = new Date(year, month - 1, day);
  date.setDate(date.getDate() + days);
  return localDateString(date);
}

function addDateObjectDays(value: Date, days: number): Date {
  const date = new Date(value.getTime());
  date.setDate(date.getDate() + days);
  return date;
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

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : "Request failed";
}
