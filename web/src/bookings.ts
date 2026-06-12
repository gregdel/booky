export type Booking = {
  uid?: string;
  etag?: string;
  name: string;
  start: string;
  end: string;
  note?: string;
};

export type BookingMutationAction = "save" | "delete";

export class ApiError extends Error {
  readonly status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

export function updateBookingPayload(booking: Booking, uid: string): Booking {
  return {
    uid,
    etag: requireETag(booking.etag),
    name: booking.name,
    start: booking.start,
    end: booking.end,
    note: booking.note,
  };
}

export function deleteBookingPayload(booking: Booking): { etag: string } {
  return {
    etag: requireETag(booking.etag),
  };
}

export function mutationFailureMessage(action: BookingMutationAction, error: unknown): string {
  if (isConflictError(error)) {
    return `Could not ${action} booking because it changed elsewhere. Bookings were refreshed.`;
  }
  if (isMissingETagError(error)) {
    return `Could not ${action} booking because its version is missing. Bookings were refreshed.`;
  }
  return errorMessage(error);
}

export function shouldRefetchAfterMutationFailure(error: unknown): boolean {
  return isConflictError(error) || isMissingETagError(error);
}

export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : "Request failed";
}

function requireETag(value: string | undefined): string {
  const etag = value?.trim() || "";
  if (!etag) {
    throw new Error("etag is required");
  }
  return etag;
}

function isConflictError(error: unknown): boolean {
  return error instanceof ApiError && error.status === 409;
}

function isMissingETagError(error: unknown): boolean {
  if (error instanceof ApiError) {
    return error.status === 400 && error.message === "etag is required";
  }
  return error instanceof Error && error.message === "etag is required";
}
