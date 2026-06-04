package booking

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestBookingValidateValid(t *testing.T) {
	b := Booking{
		Name:  "  Family stay  ",
		Start: "2026-07-01",
		End:   "2026-07-08",
		Note:  "  bring sheets  ",
	}

	if err := b.Validate(); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}

	normalized := b.Normalized()
	if normalized.Name != "Family stay" {
		t.Fatalf("normalized name = %q", normalized.Name)
	}
	if normalized.Note != "bring sheets" {
		t.Fatalf("normalized note = %q", normalized.Note)
	}
}

func TestBookingValidateRejectsInvalidFields(t *testing.T) {
	tests := map[string]Booking{
		"missing name": {
			Start: "2026-07-01",
			End:   "2026-07-02",
		},
		"long name": {
			Name:  strings.Repeat("a", MaxNameRunes+1),
			Start: "2026-07-01",
			End:   "2026-07-02",
		},
		"long note": {
			Name:  "Family stay",
			Start: "2026-07-01",
			End:   "2026-07-02",
			Note:  strings.Repeat("a", MaxNoteRunes+1),
		},
		"bad start": {
			Name:  "Family stay",
			Start: "2026-7-1",
			End:   "2026-07-02",
		},
		"same day end": {
			Name:  "Family stay",
			Start: "2026-07-01",
			End:   "2026-07-01",
		},
		"too long": {
			Name:  "Family stay",
			Start: "2026-07-01",
			End:   "2027-07-07",
		},
	}

	for name, b := range tests {
		t.Run(name, func(t *testing.T) {
			if err := b.Validate(); err == nil {
				t.Fatal("Validate returned nil error")
			}
		})
	}
}

func TestQueryRangeValidate(t *testing.T) {
	valid := QueryRange{Start: "2026-07-01", End: "2026-08-01"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}

	tests := map[string]QueryRange{
		"bad start": {Start: "2026/07/01", End: "2026-08-01"},
		"same day":  {Start: "2026-07-01", End: "2026-07-01"},
		"too long":  {Start: "2026-07-01", End: "2027-07-07"},
	}
	for name, r := range tests {
		t.Run(name, func(t *testing.T) {
			if err := r.Validate(); err == nil {
				t.Fatal("Validate returned nil error")
			}
		})
	}
}

func TestParseDate(t *testing.T) {
	if _, err := ParseDate("2026-07-01"); err != nil {
		t.Fatalf("ParseDate returned error: %v", err)
	}

	for _, value := range []string{"", " 2026-07-01", "2026-7-1", "2026-02-30", "2026-07-01T00:00:00Z"} {
		t.Run(value, func(t *testing.T) {
			if _, err := ParseDate(value); err == nil {
				t.Fatal("ParseDate returned nil error")
			}
		})
	}
}

func TestNewUID(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 30, 45, 0, time.FixedZone("CEST", 2*60*60))
	uid, err := NewUID(now)
	if err != nil {
		t.Fatalf("NewUID returned error: %v", err)
	}

	pattern := regexp.MustCompile(`^booky-20260701T103045Z-[0-9a-f]{32}$`)
	if !pattern.MatchString(uid) {
		t.Fatalf("uid = %q, want pattern %s", uid, pattern)
	}
}
