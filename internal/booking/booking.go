package booking

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	DateLayout      = "2006-01-02"
	MaxNameRunes    = 120
	MaxNoteRunes    = 1000
	MaxBookingDays  = 370
	MaxQueryDays    = 370
	uidRandomBytes  = 16
	uidRandomHexLen = uidRandomBytes * 2
)

type Booking struct {
	UID   string `json:"uid,omitempty"`
	ETag  string `json:"etag,omitempty"`
	Name  string `json:"name"`
	Start string `json:"start"`
	End   string `json:"end"`
	Note  string `json:"note,omitempty"`
}

type QueryRange struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

func (b Booking) Normalized() Booking {
	b.Name = strings.TrimSpace(b.Name)
	b.Note = strings.TrimSpace(b.Note)
	return b
}

func (b Booking) Validate() error {
	b = b.Normalized()

	var errs []error
	if b.Name == "" {
		errs = append(errs, errors.New("name is required"))
	}
	if utf8.RuneCountInString(b.Name) > MaxNameRunes {
		errs = append(errs, fmt.Errorf("name must be at most %d characters", MaxNameRunes))
	}
	if utf8.RuneCountInString(b.Note) > MaxNoteRunes {
		errs = append(errs, fmt.Errorf("note must be at most %d characters", MaxNoteRunes))
	}

	start, startErr := ParseDate(b.Start)
	if startErr != nil {
		errs = append(errs, fmt.Errorf("start: %w", startErr))
	}
	end, endErr := ParseDate(b.End)
	if endErr != nil {
		errs = append(errs, fmt.Errorf("end: %w", endErr))
	}
	if startErr == nil && endErr == nil {
		if !end.After(start) {
			errs = append(errs, errors.New("end must be after start"))
		} else if daysBetween(start, end) > MaxBookingDays {
			errs = append(errs, fmt.Errorf("booking range must be at most %d days", MaxBookingDays))
		}
	}

	return errors.Join(errs...)
}

func NormalizeETag(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("etag is required")
	}
	return value, nil
}

func (r QueryRange) Validate() error {
	var errs []error

	start, startErr := ParseDate(r.Start)
	if startErr != nil {
		errs = append(errs, fmt.Errorf("start: %w", startErr))
	}
	end, endErr := ParseDate(r.End)
	if endErr != nil {
		errs = append(errs, fmt.Errorf("end: %w", endErr))
	}
	if startErr == nil && endErr == nil {
		if !end.After(start) {
			errs = append(errs, errors.New("end must be after start"))
		} else if daysBetween(start, end) > MaxQueryDays {
			errs = append(errs, fmt.Errorf("query range must be at most %d days", MaxQueryDays))
		}
	}

	return errors.Join(errs...)
}

func ParseDate(value string) (time.Time, error) {
	if strings.TrimSpace(value) != value || value == "" {
		return time.Time{}, errors.New("must be YYYY-MM-DD")
	}

	t, err := time.Parse(DateLayout, value)
	if err != nil || t.Format(DateLayout) != value {
		return time.Time{}, errors.New("must be YYYY-MM-DD")
	}
	return t, nil
}

func NewUID(now time.Time) (string, error) {
	var random [uidRandomBytes]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("generate uid randomness: %w", err)
	}

	return fmt.Sprintf("booky-%s-%s",
		now.UTC().Format("20060102T150405Z"),
		hex.EncodeToString(random[:]),
	), nil
}

func daysBetween(start, end time.Time) int {
	return int(end.Sub(start).Hours() / 24)
}
