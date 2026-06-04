package caldav

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gregdel/booky/internal/booking"
	"github.com/gregdel/booky/internal/config"
	"github.com/gregdel/booky/internal/ical"
)

var (
	ErrNotFound = errors.New("caldav: not found")
	ErrConflict = errors.New("caldav: conflict")
	ErrUpstream = errors.New("caldav: upstream failure")
)

const defaultTimeout = 15 * time.Second

type Client struct {
	collection *url.URL
	user       string
	pass       string
	httpClient *http.Client
	now        func() time.Time
}

func New(cfg config.CalDAVConfig, httpClient *http.Client) (*Client, error) {
	collection, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse caldav url: %w", err)
	}
	if collection.Scheme != "http" && collection.Scheme != "https" {
		return nil, errors.New("caldav url must use http or https")
	}
	if collection.Host == "" {
		return nil, errors.New("caldav url must be absolute")
	}
	if !strings.HasSuffix(collection.Path, "/") {
		return nil, errors.New("caldav url must end with /")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultTimeout}
	}

	return &Client{
		collection: collection,
		user:       cfg.User,
		pass:       cfg.Pass,
		httpClient: httpClient,
		now:        time.Now,
	}, nil
}

func (c *Client) List(ctx context.Context, r booking.QueryRange) ([]booking.Booking, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}

	body, err := reportBody(r)
	if err != nil {
		return nil, err
	}
	req, err := c.request(ctx, "REPORT", c.collection.String(), strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Depth", "1")
	req.Header.Set("Content-Type", "application/xml; charset=utf-8")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: REPORT: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: read REPORT response: %v", ErrUpstream, err)
	}
	if resp.StatusCode != http.StatusMultiStatus {
		return nil, mapStatus(resp.StatusCode, "REPORT")
	}

	return c.parseMultiStatus(data)
}

func (c *Client) Create(ctx context.Context, b booking.Booking) (booking.Booking, error) {
	b = b.Normalized()
	if b.UID == "" {
		uid, err := booking.NewUID(c.now())
		if err != nil {
			return booking.Booking{}, err
		}
		b.UID = uid
	}
	if err := b.Validate(); err != nil {
		return booking.Booking{}, err
	}

	target := c.uidURL(b.UID).String()
	body, err := ical.MarshalCalendar(b, c.now())
	if err != nil {
		return booking.Booking{}, err
	}
	req, err := c.request(ctx, http.MethodPut, target, strings.NewReader(body))
	if err != nil {
		return booking.Booking{}, err
	}
	req.Header.Set("Content-Type", "text/calendar; charset=utf-8")
	req.Header.Set("If-None-Match", "*")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return booking.Booking{}, fmt.Errorf("%w: PUT create: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()
	if !isWriteSuccess(resp.StatusCode) {
		return booking.Booking{}, mapStatus(resp.StatusCode, "PUT create")
	}

	b.Href = target
	b.ETag = resp.Header.Get("ETag")
	return b, nil
}

func (c *Client) Update(ctx context.Context, b booking.Booking) (booking.Booking, error) {
	b = b.Normalized()
	if err := b.Validate(); err != nil {
		return booking.Booking{}, err
	}

	target, err := c.targetURL(b)
	if err != nil {
		return booking.Booking{}, err
	}
	body, err := ical.MarshalCalendar(b, c.now())
	if err != nil {
		return booking.Booking{}, err
	}
	req, err := c.request(ctx, http.MethodPut, target, strings.NewReader(body))
	if err != nil {
		return booking.Booking{}, err
	}
	req.Header.Set("Content-Type", "text/calendar; charset=utf-8")
	if b.ETag != "" {
		req.Header.Set("If-Match", b.ETag)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return booking.Booking{}, fmt.Errorf("%w: PUT update: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()
	if !isWriteSuccess(resp.StatusCode) {
		return booking.Booking{}, mapStatus(resp.StatusCode, "PUT update")
	}

	b.Href = target
	if etag := resp.Header.Get("ETag"); etag != "" {
		b.ETag = etag
	}
	return b, nil
}

func (c *Client) Delete(ctx context.Context, b booking.Booking) error {
	target, err := c.targetURL(b)
	if err != nil {
		return err
	}
	req, err := c.request(ctx, http.MethodDelete, target, nil)
	if err != nil {
		return err
	}
	if b.ETag != "" {
		req.Header.Set("If-Match", b.ETag)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: DELETE: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return mapStatus(resp.StatusCode, "DELETE")
	}
	return nil
}

func (c *Client) request(ctx context.Context, method, target string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.user, c.pass)
	return req, nil
}

func (c *Client) parseMultiStatus(data []byte) ([]booking.Booking, error) {
	var ms multiStatus
	if err := xml.NewDecoder(bytes.NewReader(data)).Decode(&ms); err != nil {
		return nil, fmt.Errorf("%w: parse multistatus: %v", ErrUpstream, err)
	}

	var bookings []booking.Booking
	for _, response := range ms.Responses {
		status := responseStatus(response)
		if status != 0 && status != http.StatusOK {
			if status == http.StatusNotFound {
				continue
			}
			return nil, mapStatus(status, "REPORT response")
		}

		calendarData := response.CalendarData()
		if strings.TrimSpace(calendarData) == "" {
			continue
		}
		parsed, err := ical.ParseCalendar(calendarData)
		if err != nil {
			return nil, fmt.Errorf("%w: parse calendar data: %v", ErrUpstream, err)
		}

		href := c.resolveHref(response.Href)
		etag := response.ETag()
		for _, b := range parsed {
			b.Href = href
			b.ETag = etag
			bookings = append(bookings, b)
		}
	}

	return bookings, nil
}

func responseStatus(response davResponse) int {
	for _, propstat := range response.Propstats {
		if status := parseHTTPStatus(propstat.Status); status != 0 {
			return status
		}
	}
	return parseHTTPStatus(response.Status)
}

func parseHTTPStatus(value string) int {
	fields := strings.Fields(value)
	for _, field := range fields {
		if len(field) == 3 {
			status, err := strconv.Atoi(field)
			if err == nil {
				return status
			}
		}
	}
	return 0
}

func (c *Client) targetURL(b booking.Booking) (string, error) {
	if b.Href != "" {
		return c.resolveHref(b.Href), nil
	}
	if b.UID != "" {
		return c.uidURL(b.UID).String(), nil
	}
	return "", errors.New("href or uid is required")
}

func (c *Client) uidURL(uid string) *url.URL {
	u := *c.collection
	u.Path = c.collection.Path + uid + ".ics"
	u.RawPath = c.collection.EscapedPath() + url.PathEscape(uid) + ".ics"
	return &u
}

func (c *Client) resolveHref(href string) string {
	u, err := url.Parse(href)
	if err != nil {
		return href
	}
	return c.collection.ResolveReference(u).String()
}

func reportBody(r booking.QueryRange) (string, error) {
	start, err := booking.ParseDate(r.Start)
	if err != nil {
		return "", err
	}
	end, err := booking.ParseDate(r.End)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<C:calendar-query xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav">
  <D:prop>
    <D:getetag/>
    <C:calendar-data/>
  </D:prop>
  <C:filter>
    <C:comp-filter name="VCALENDAR">
      <C:comp-filter name="VEVENT">
        <C:time-range start="%s" end="%s"/>
      </C:comp-filter>
    </C:comp-filter>
  </C:filter>
</C:calendar-query>`, caldavTime(start), caldavTime(end)), nil
}

func caldavTime(t time.Time) string {
	return t.UTC().Format("20060102T150405Z")
}

func mapStatus(status int, op string) error {
	switch status {
	case http.StatusNotFound:
		return fmt.Errorf("%w: %s returned %d", ErrNotFound, op, status)
	case http.StatusConflict, http.StatusPreconditionFailed:
		return fmt.Errorf("%w: %s returned %d", ErrConflict, op, status)
	default:
		return fmt.Errorf("%w: %s returned %d", ErrUpstream, op, status)
	}
}

func isWriteSuccess(status int) bool {
	return status == http.StatusOK || status == http.StatusCreated || status == http.StatusNoContent
}

type multiStatus struct {
	Responses []davResponse `xml:"response"`
}

type davResponse struct {
	Href      string        `xml:"href"`
	Status    string        `xml:"status"`
	Propstats []davPropstat `xml:"propstat"`
}

func (r davResponse) ETag() string {
	for _, propstat := range r.Propstats {
		if propstat.Prop.GetETag != "" {
			return propstat.Prop.GetETag
		}
	}
	return ""
}

func (r davResponse) CalendarData() string {
	for _, propstat := range r.Propstats {
		if propstat.Prop.CalendarData != "" {
			return propstat.Prop.CalendarData
		}
	}
	return ""
}

type davPropstat struct {
	Status string  `xml:"status"`
	Prop   davProp `xml:"prop"`
}

type davProp struct {
	GetETag      string `xml:"getetag"`
	CalendarData string `xml:"calendar-data"`
}
