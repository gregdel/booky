package ical

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gregdel/booky/internal/booking"
)

const (
	dateValueLayout = "20060102"
	prodID          = "-//booky//booky//EN"
	bookyMarker     = "X-BOOKY"
)

func MarshalCalendar(b booking.Booking, now time.Time) (string, error) {
	b = b.Normalized()
	if b.UID == "" {
		return "", errors.New("uid is required")
	}
	if err := b.Validate(); err != nil {
		return "", err
	}

	start, _ := booking.ParseDate(b.Start)
	end, _ := booking.ParseDate(b.End)

	lines := []string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:" + escapeText(prodID),
		"BEGIN:VEVENT",
		"UID:" + escapeText(b.UID),
		"DTSTAMP:" + now.UTC().Format("20060102T150405Z"),
		"DTSTART;VALUE=DATE:" + start.Format(dateValueLayout),
		"DTEND;VALUE=DATE:" + end.Format(dateValueLayout),
		"SUMMARY:" + escapeText(b.Name),
	}
	if b.Note != "" {
		lines = append(lines, "DESCRIPTION:"+escapeText(b.Note))
	}
	lines = append(lines,
		bookyMarker+":1",
		"END:VEVENT",
		"END:VCALENDAR",
	)

	var out strings.Builder
	for _, line := range lines {
		out.WriteString(foldLine(line))
		out.WriteString("\r\n")
	}
	return out.String(), nil
}

func ParseCalendar(input string) ([]booking.Booking, error) {
	lines := unfoldLines(input)
	events := collectEvents(lines)
	bookings := make([]booking.Booking, 0, len(events))

	for _, lines := range events {
		b, ok, err := parseEvent(lines)
		if err != nil {
			return nil, err
		}
		if ok {
			bookings = append(bookings, b)
		}
	}

	return bookings, nil
}

type property struct {
	name   string
	params map[string]string
	value  string
}

func parseEvent(lines []string) (booking.Booking, bool, error) {
	props := make(map[string][]property)
	for _, line := range lines {
		prop, ok := parseProperty(line)
		if !ok {
			continue
		}
		props[prop.name] = append(props[prop.name], prop)
	}

	if firstValue(props, bookyMarker) != "1" {
		return booking.Booking{}, false, nil
	}

	uid := unescapeText(firstValue(props, "UID"))
	if uid == "" {
		return booking.Booking{}, false, errors.New("booky event missing UID")
	}

	startProp, ok := firstProp(props, "DTSTART")
	if !ok {
		return booking.Booking{}, false, errors.New("booky event missing DTSTART")
	}
	if !isDateProperty(startProp) {
		return booking.Booking{}, false, nil
	}
	start, err := parseICalDate(startProp.value)
	if err != nil {
		return booking.Booking{}, false, fmt.Errorf("booky event invalid DTSTART: %w", err)
	}

	end := start.AddDate(0, 0, 1)
	if endProp, ok := firstProp(props, "DTEND"); ok {
		if !isDateProperty(endProp) {
			return booking.Booking{}, false, nil
		}
		end, err = parseICalDate(endProp.value)
		if err != nil {
			return booking.Booking{}, false, fmt.Errorf("booky event invalid DTEND: %w", err)
		}
	}

	b := booking.Booking{
		UID:   uid,
		Name:  unescapeText(firstValue(props, "SUMMARY")),
		Start: start.Format(booking.DateLayout),
		End:   end.Format(booking.DateLayout),
		Note:  unescapeText(firstValue(props, "DESCRIPTION")),
	}
	if err := b.Validate(); err != nil {
		return booking.Booking{}, false, fmt.Errorf("booky event invalid booking: %w", err)
	}

	return b.Normalized(), true, nil
}

func collectEvents(lines []string) [][]string {
	var events [][]string
	var current []string
	inEvent := false

	for _, line := range lines {
		upper := strings.ToUpper(line)
		switch upper {
		case "BEGIN:VEVENT":
			inEvent = true
			current = nil
		case "END:VEVENT":
			if inEvent {
				events = append(events, current)
			}
			inEvent = false
			current = nil
		default:
			if inEvent {
				current = append(current, line)
			}
		}
	}

	return events
}

func parseProperty(line string) (property, bool) {
	before, value, ok := strings.Cut(line, ":")
	if !ok {
		return property{}, false
	}

	parts := strings.Split(before, ";")
	if parts[0] == "" {
		return property{}, false
	}

	prop := property{
		name:   strings.ToUpper(parts[0]),
		params: map[string]string{},
		value:  value,
	}
	for _, part := range parts[1:] {
		key, val, ok := strings.Cut(part, "=")
		if !ok {
			prop.params[strings.ToUpper(part)] = ""
			continue
		}
		prop.params[strings.ToUpper(key)] = strings.ToUpper(val)
	}

	return prop, true
}

func firstProp(props map[string][]property, name string) (property, bool) {
	values := props[name]
	if len(values) == 0 {
		return property{}, false
	}
	return values[0], true
}

func firstValue(props map[string][]property, name string) string {
	prop, ok := firstProp(props, name)
	if !ok {
		return ""
	}
	return prop.value
}

func isDateProperty(prop property) bool {
	if valueType, ok := prop.params["VALUE"]; ok {
		return valueType == "DATE"
	}
	return len(prop.value) == len("20060102") && allDigits(prop.value)
}

func parseICalDate(value string) (time.Time, error) {
	t, err := time.Parse(dateValueLayout, value)
	if err != nil || t.Format(dateValueLayout) != value {
		return time.Time{}, errors.New("must be YYYYMMDD")
	}
	return t, nil
}

func unfoldLines(input string) []string {
	normalized := strings.ReplaceAll(input, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	rawLines := strings.Split(normalized, "\n")
	lines := make([]string, 0, len(rawLines))

	for _, line := range rawLines {
		if line == "" {
			continue
		}
		if (line[0] == ' ' || line[0] == '\t') && len(lines) > 0 {
			lines[len(lines)-1] += line[1:]
			continue
		}
		lines = append(lines, line)
	}

	return lines
}

func foldLine(line string) string {
	const firstLimit = 75
	const continuationLimit = 74

	if len(line) <= firstLimit {
		return line
	}

	var out strings.Builder
	remaining := line
	limit := firstLimit
	first := true

	for len(remaining) > 0 {
		chunk, rest := splitAtOctets(remaining, limit)
		if !first {
			out.WriteString("\r\n ")
		}
		out.WriteString(chunk)
		remaining = rest
		limit = continuationLimit
		first = false
	}

	return out.String()
}

func splitAtOctets(value string, limit int) (string, string) {
	if len(value) <= limit {
		return value, ""
	}

	n := 0
	for i, r := range value {
		size := utf8.RuneLen(r)
		if n+size > limit {
			if i == 0 {
				return value[:size], value[size:]
			}
			return value[:i], value[i:]
		}
		n += size
	}
	return value, ""
}

func escapeText(value string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		"\n", `\n`,
		";", `\;`,
		",", `\,`,
	)
	return replacer.Replace(value)
}

func unescapeText(value string) string {
	var out strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] != '\\' || i+1 >= len(value) {
			out.WriteByte(value[i])
			continue
		}

		i++
		switch value[i] {
		case 'n', 'N':
			out.WriteByte('\n')
		case '\\', ';', ',':
			out.WriteByte(value[i])
		default:
			out.WriteByte(value[i])
		}
	}
	return out.String()
}

func allDigits(value string) bool {
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
