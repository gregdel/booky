import { describe, expect, test } from "bun:test";

import {
  ApiError,
  deleteBookingPayload,
  mutationFailureMessage,
  shouldRefetchAfterMutationFailure,
  updateBookingPayload,
  type Booking,
} from "./bookings";

describe("booking mutation payloads", () => {
  test("edit payload includes trimmed etag and omits href", () => {
    const booking = {
      uid: "booky-uid",
      href: "ignored",
      etag: "  \"etag-old\"  ",
      name: "Family stay",
      start: "2026-07-10",
      end: "2026-07-17",
      note: "note",
    } as Booking & { href: string };

    const payload = updateBookingPayload(booking, "booky-uid");

    expect(payload).toEqual({
      uid: "booky-uid",
      etag: "\"etag-old\"",
      name: "Family stay",
      start: "2026-07-10",
      end: "2026-07-17",
      note: "note",
    });
    expect("href" in payload).toBe(false);
  });

  test("delete payload includes trimmed etag and omits href", () => {
    const booking = {
      uid: "booky-uid",
      href: "ignored",
      etag: "  \"etag-old\"  ",
      name: "Family stay",
      start: "2026-07-10",
      end: "2026-07-17",
    } as Booking & { href: string };

    const payload = deleteBookingPayload(booking);

    expect(payload).toEqual({ etag: "\"etag-old\"" });
    expect("href" in payload).toBe(false);
  });

  test("mutation payloads require etag", () => {
    const booking: Booking = {
      uid: "booky-uid",
      etag: "  ",
      name: "Family stay",
      start: "2026-07-10",
      end: "2026-07-17",
    };

    expect(() => updateBookingPayload(booking, "booky-uid")).toThrow("etag is required");
    expect(() => deleteBookingPayload(booking)).toThrow("etag is required");
  });
});

describe("booking mutation failures", () => {
  test("conflicts use a clear refresh message", () => {
    const error = new ApiError(409, "caldav: conflict");

    expect(mutationFailureMessage("save", error)).toBe(
      "Could not save booking because it changed elsewhere. Bookings were refreshed.",
    );
    expect(shouldRefetchAfterMutationFailure(error)).toBe(true);
  });

  test("missing etag uses a clear refresh message", () => {
    const error = new ApiError(400, "etag is required");

    expect(mutationFailureMessage("delete", error)).toBe(
      "Could not delete booking because its version is missing. Bookings were refreshed.",
    );
    expect(shouldRefetchAfterMutationFailure(error)).toBe(true);
  });
});
