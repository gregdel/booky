package ical

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/gregdel/booky/internal/booking"
)

func TestMarshalCalendar(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 30, 45, 0, time.FixedZone("CEST", 2*60*60))
	b := booking.Booking{
		UID:   "booky-20260701T103045Z-0123456789abcdef0123456789abcdef",
		Name:  " Family, stay; summer ",
		Start: "2026-07-10",
		End:   "2026-07-17",
		Note:  "Bring sheets\nUse back door",
	}

	got, err := MarshalCalendar(b, now)
	if err != nil {
		t.Fatalf("MarshalCalendar returned error: %v", err)
	}

	for _, want := range []string{
		"BEGIN:VCALENDAR\r\n",
		"VERSION:2.0\r\n",
		"PRODID:-//booky//booky//EN\r\n",
		"BEGIN:VEVENT\r\n",
		"UID:booky-20260701T103045Z-0123456789abcdef0123456789abcdef\r\n",
		"DTSTAMP:20260701T103045Z\r\n",
		"DTSTART;VALUE=DATE:20260710\r\n",
		"DTEND;VALUE=DATE:20260717\r\n",
		"SUMMARY:Family\\, stay\\; summer\r\n",
		"DESCRIPTION:Bring sheets\\nUse back door\r\n",
		"X-BOOKY:1\r\n",
		"END:VEVENT\r\n",
		"END:VCALENDAR\r\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("calendar missing %q in:\n%s", want, got)
		}
	}

	if strings.Contains(got, "\n") && !strings.Contains(got, "\r\n") {
		t.Fatal("calendar did not use CRLF")
	}
}

func TestMarshalCalendarOmitsEmptyDescription(t *testing.T) {
	got, err := MarshalCalendar(validBooking(), time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("MarshalCalendar returned error: %v", err)
	}
	if strings.Contains(got, "DESCRIPTION:") {
		t.Fatalf("calendar included DESCRIPTION:\n%s", got)
	}
}

func TestMarshalCalendarRequiresValidBooking(t *testing.T) {
	b := validBooking()
	b.UID = ""
	if _, err := MarshalCalendar(b, time.Now()); err == nil {
		t.Fatal("MarshalCalendar returned nil error for missing uid")
	}

	b = validBooking()
	b.End = b.Start
	if _, err := MarshalCalendar(b, time.Now()); err == nil {
		t.Fatal("MarshalCalendar returned nil error for invalid booking")
	}
}

func TestMarshalCalendarFoldsLongLines(t *testing.T) {
	b := validBooking()
	b.Name = strings.Repeat("Reserve ", 11) + "été"

	got, err := MarshalCalendar(b, time.Now())
	if err != nil {
		t.Fatalf("MarshalCalendar returned error: %v", err)
	}

	if !strings.Contains(got, "\r\n ") {
		t.Fatalf("calendar was not folded:\n%s", got)
	}
	for _, line := range strings.Split(strings.TrimSuffix(got, "\r\n"), "\r\n") {
		if len(line) > 75 {
			t.Fatalf("line has %d octets, want <= 75: %q", len(line), line)
		}
		if !utf8.ValidString(line) {
			t.Fatalf("line split utf8 rune: %q", line)
		}
	}
}

func TestParseCalendarRoundTrip(t *testing.T) {
	b := validBooking()
	b.Note = "Bring sheets\nUse, semicolon; and slash \\"

	raw, err := MarshalCalendar(b, time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("MarshalCalendar returned error: %v", err)
	}

	bookings, err := ParseCalendar(raw)
	if err != nil {
		t.Fatalf("ParseCalendar returned error: %v", err)
	}
	if len(bookings) != 1 {
		t.Fatalf("len(bookings) = %d, want 1", len(bookings))
	}
	got := bookings[0]
	if got.UID != b.UID || got.Name != b.Name || got.Start != b.Start || got.End != b.End || got.Note != b.Note {
		t.Fatalf("booking = %#v, want %#v", got, b)
	}
}

func TestParseCalendarHandlesFoldedLFInputAndMissingDTEND(t *testing.T) {
	input := strings.Join([]string{
		"BEGIN:VCALENDAR",
		"BEGIN:VEVENT",
		"UID:booky-uid",
		"DTSTART;VALUE=DATE:20260710",
		"SUMMARY:Family",
		" stay",
		"X-BOOKY:1",
		"END:VEVENT",
		"END:VCALENDAR",
	}, "\n")

	bookings, err := ParseCalendar(input)
	if err != nil {
		t.Fatalf("ParseCalendar returned error: %v", err)
	}
	if len(bookings) != 1 {
		t.Fatalf("len(bookings) = %d, want 1", len(bookings))
	}
	if bookings[0].Name != "Familystay" {
		t.Fatalf("Name = %q, want folded value", bookings[0].Name)
	}
	if bookings[0].End != "2026-07-11" {
		t.Fatalf("End = %q, want next day", bookings[0].End)
	}
}

func TestParseCalendarIgnoresNonBookyAndUnsupportedDateTimeEvents(t *testing.T) {
	input := strings.Join([]string{
		"BEGIN:VCALENDAR",
		"BEGIN:VEVENT",
		"UID:other",
		"DTSTART;VALUE=DATE:20260710",
		"DTEND;VALUE=DATE:20260711",
		"SUMMARY:Other",
		"END:VEVENT",
		"BEGIN:VEVENT",
		"UID:booky-datetime",
		"DTSTART:20260710T120000Z",
		"DTEND:20260710T130000Z",
		"SUMMARY:Timed",
		"X-BOOKY:1",
		"END:VEVENT",
		"END:VCALENDAR",
	}, "\r\n")

	bookings, err := ParseCalendar(input)
	if err != nil {
		t.Fatalf("ParseCalendar returned error: %v", err)
	}
	if len(bookings) != 0 {
		t.Fatalf("len(bookings) = %d, want 0", len(bookings))
	}
}

func TestParseCalendarRejectsMalformedBookyEvents(t *testing.T) {
	tests := map[string]string{
		"missing uid": `
BEGIN:VCALENDAR
BEGIN:VEVENT
DTSTART;VALUE=DATE:20260710
DTEND;VALUE=DATE:20260711
SUMMARY:Family stay
X-BOOKY:1
END:VEVENT
END:VCALENDAR
`,
		"bad date": `
BEGIN:VCALENDAR
BEGIN:VEVENT
UID:booky-uid
DTSTART;VALUE=DATE:20260230
DTEND;VALUE=DATE:20260711
SUMMARY:Family stay
X-BOOKY:1
END:VEVENT
END:VCALENDAR
`,
		"missing summary": `
BEGIN:VCALENDAR
BEGIN:VEVENT
UID:booky-uid
DTSTART;VALUE=DATE:20260710
DTEND;VALUE=DATE:20260711
X-BOOKY:1
END:VEVENT
END:VCALENDAR
`,
	}

	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseCalendar(input); err == nil {
				t.Fatal("ParseCalendar returned nil error")
			}
		})
	}
}

func validBooking() booking.Booking {
	return booking.Booking{
		UID:   "booky-20260701T103045Z-0123456789abcdef0123456789abcdef",
		Name:  "Family stay",
		Start: "2026-07-10",
		End:   "2026-07-17",
	}
}
