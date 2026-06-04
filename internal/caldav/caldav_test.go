package caldav

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gregdel/booky/internal/booking"
	"github.com/gregdel/booky/internal/config"
	"github.com/gregdel/booky/internal/ical"
)

func TestListSendsReportAndParsesMultiStatus(t *testing.T) {
	event, err := ical.MarshalCalendar(booking.Booking{
		UID:   "booky-uid",
		Name:  "Family stay",
		Start: "2026-07-10",
		End:   "2026-07-17",
	}, time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("MarshalCalendar returned error: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "REPORT" {
			t.Fatalf("method = %s, want REPORT", r.Method)
		}
		if r.URL.Path != "/remote.php/dav/calendars/family-house/vacation-house/" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if user, pass, ok := r.BasicAuth(); !ok || user != "family-house" || pass != "app-password" {
			t.Fatalf("BasicAuth = %q/%q/%v", user, pass, ok)
		}
		if got := r.Header.Get("Depth"); got != "1" {
			t.Fatalf("Depth = %q, want 1", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		for _, want := range []string{
			"<C:calendar-query",
			"<D:getetag/>",
			"<C:calendar-data/>",
			`<C:time-range start="20260701T000000Z" end="20260801T000000Z"/>`,
		} {
			if !strings.Contains(string(body), want) {
				t.Fatalf("REPORT body missing %q:\n%s", want, body)
			}
		}

		w.WriteHeader(207)
		_, _ = w.Write([]byte(multistatusXML("/remote.php/dav/calendars/family-house/vacation-house/booky-uid.ics", `"etag-1"`, event)))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	got, err := client.List(context.Background(), booking.QueryRange{Start: "2026-07-01", End: "2026-08-01"})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].UID != "booky-uid" || got[0].Name != "Family stay" || got[0].ETag != `"etag-1"` {
		t.Fatalf("booking = %#v", got[0])
	}
	if got[0].Href != server.URL+"/remote.php/dav/calendars/family-house/vacation-house/booky-uid.ics" {
		t.Fatalf("Href = %q", got[0].Href)
	}
}

func TestListHandlesNamespacesMultipleResponsesAndIgnoredEvents(t *testing.T) {
	bookyEvent, err := ical.MarshalCalendar(booking.Booking{
		UID:   "booky-uid",
		Name:  "Family stay",
		Start: "2026-07-10",
		End:   "2026-07-17",
	}, time.Now())
	if err != nil {
		t.Fatalf("MarshalCalendar returned error: %v", err)
	}
	otherEvent := strings.Join([]string{
		"BEGIN:VCALENDAR",
		"BEGIN:VEVENT",
		"UID:other",
		"DTSTART;VALUE=DATE:20260710",
		"DTEND;VALUE=DATE:20260711",
		"SUMMARY:Other",
		"END:VEVENT",
		"END:VCALENDAR",
	}, "\r\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(207)
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<D:multistatus xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav">
  <D:response>
    <D:href>booky-uid.ics</D:href>
    <D:propstat><D:status>HTTP/1.1 200 OK</D:status><D:prop><D:getetag>etag-1</D:getetag><C:calendar-data>` + escapeXML(bookyEvent) + `</C:calendar-data></D:prop></D:propstat>
  </D:response>
  <D:response>
    <D:href>other.ics</D:href>
    <D:propstat><D:status>HTTP/1.1 200 OK</D:status><D:prop><D:getetag>etag-2</D:getetag><C:calendar-data>` + escapeXML(otherEvent) + `</C:calendar-data></D:prop></D:propstat>
  </D:response>
</D:multistatus>`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	got, err := client.List(context.Background(), booking.QueryRange{Start: "2026-07-01", End: "2026-08-01"})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].Href != server.URL+"/remote.php/dav/calendars/family-house/vacation-house/booky-uid.ics" {
		t.Fatalf("Href = %q", got[0].Href)
	}
}

func TestListMapsErrors(t *testing.T) {
	tests := map[int]error{
		http.StatusNotFound:            ErrNotFound,
		http.StatusConflict:            ErrConflict,
		http.StatusPreconditionFailed:  ErrConflict,
		http.StatusInternalServerError: ErrUpstream,
	}

	for status, want := range tests {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
			}))
			defer server.Close()

			client := newTestClient(t, server.URL)
			_, err := client.List(context.Background(), booking.QueryRange{Start: "2026-07-01", End: "2026-08-01"})
			if !errors.Is(err, want) {
				t.Fatalf("err = %v, want %v", err, want)
			}
		})
	}
}

func TestListMapsMalformedUpstreamData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(207)
		_, _ = w.Write([]byte(multistatusXML("booky-uid.ics", "etag-1", `BEGIN:VCALENDAR
BEGIN:VEVENT
UID:booky-uid
DTSTART;VALUE=DATE:20260230
SUMMARY:Broken
X-BOOKY:1
END:VEVENT
END:VCALENDAR`)))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.List(context.Background(), booking.QueryRange{Start: "2026-07-01", End: "2026-08-01"})
	if !errors.Is(err, ErrUpstream) {
		t.Fatalf("err = %v, want ErrUpstream", err)
	}
}

func TestCreateGeneratesUIDAndSendsPut(t *testing.T) {
	var seenBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("method = %s, want PUT", r.Method)
		}
		if !regexp.MustCompile(`/booky-20260701T120000Z-[0-9a-f]{32}\.ics$`).MatchString(r.URL.Path) {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("If-None-Match"); got != "*" {
			t.Fatalf("If-None-Match = %q, want *", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		seenBody = string(body)
		w.Header().Set("ETag", `"etag-created"`)
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	client.now = func() time.Time { return time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC) }
	got, err := client.Create(context.Background(), booking.Booking{Name: "Family stay", Start: "2026-07-10", End: "2026-07-17"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if !strings.HasPrefix(got.UID, "booky-20260701T120000Z-") {
		t.Fatalf("UID = %q", got.UID)
	}
	if got.ETag != `"etag-created"` {
		t.Fatalf("ETag = %q", got.ETag)
	}
	if !strings.Contains(seenBody, "\r\n") || !strings.Contains(seenBody, "X-BOOKY:1\r\n") {
		t.Fatalf("body is not expected iCalendar:\n%s", seenBody)
	}
}

func TestCreateMapsCollisionToConflict(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPreconditionFailed)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.Create(context.Background(), booking.Booking{UID: "booky-uid", Name: "Family stay", Start: "2026-07-10", End: "2026-07-17"})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
}

func TestUpdateUsesHrefOptionalIfMatchAndRefreshesETag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("method = %s, want PUT", r.Method)
		}
		if r.URL.Path != "/remote.php/dav/calendars/family-house/vacation-house/custom.ics" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("If-Match"); got != `"etag-old"` {
			t.Fatalf("If-Match = %q", got)
		}
		w.Header().Set("ETag", `"etag-new"`)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	got, err := client.Update(context.Background(), booking.Booking{
		UID:   "booky-uid",
		Href:  "/remote.php/dav/calendars/family-house/vacation-house/custom.ics",
		ETag:  `"etag-old"`,
		Name:  "Family stay",
		Start: "2026-07-10",
		End:   "2026-07-17",
	})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if got.ETag != `"etag-new"` {
		t.Fatalf("ETag = %q", got.ETag)
	}
}

func TestUpdateUsesUIDFallbackAndOmitsEmptyIfMatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/remote.php/dav/calendars/family-house/vacation-house/booky-uid.ics" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("If-Match"); got != "" {
			t.Fatalf("If-Match = %q, want empty", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.Update(context.Background(), booking.Booking{UID: "booky-uid", Name: "Family stay", Start: "2026-07-10", End: "2026-07-17"})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
}

func TestUpdateEscapesUIDFallbackAsPathSegment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawPath != "/remote.php/dav/calendars/family-house/vacation-house/booky%2Fuid%20with%20space.ics" {
			t.Fatalf("RawPath = %q", r.URL.RawPath)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.Update(context.Background(), booking.Booking{UID: "booky/uid with space", Name: "Family stay", Start: "2026-07-10", End: "2026-07-17"})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
}

func TestUpdateValidatesBeforeNetwork(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.Update(context.Background(), booking.Booking{UID: "booky-uid", Name: "", Start: "2026-07-10", End: "2026-07-17"})
	if err == nil {
		t.Fatal("Update returned nil error")
	}
	if calls != 0 {
		t.Fatalf("network calls = %d, want 0", calls)
	}
}

func TestUpdateMapsStaleETag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPreconditionFailed)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.Update(context.Background(), booking.Booking{UID: "booky-uid", Name: "Family stay", Start: "2026-07-10", End: "2026-07-17"})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
}

func TestDeleteUsesOptionalIfMatchAndMapsStatuses(t *testing.T) {
	tests := map[string]struct {
		status int
		want   error
	}{
		"ok":        {status: http.StatusOK},
		"noContent": {status: http.StatusNoContent},
		"notFound":  {status: http.StatusNotFound, want: ErrNotFound},
		"stale":     {status: http.StatusPreconditionFailed, want: ErrConflict},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodDelete {
					t.Fatalf("method = %s, want DELETE", r.Method)
				}
				if got := r.Header.Get("If-Match"); got != `"etag-old"` {
					t.Fatalf("If-Match = %q", got)
				}
				w.WriteHeader(tc.status)
			}))
			defer server.Close()

			client := newTestClient(t, server.URL)
			err := client.Delete(context.Background(), booking.Booking{UID: "booky-uid", ETag: `"etag-old"`})
			if tc.want == nil && err != nil {
				t.Fatalf("Delete returned error: %v", err)
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestDeleteRequiresTarget(t *testing.T) {
	client := newTestClient(t, "http://example.test")
	err := client.Delete(context.Background(), booking.Booking{})
	if err == nil {
		t.Fatal("Delete returned nil error")
	}
}

func newTestClient(t *testing.T, baseURL string) *Client {
	t.Helper()

	u, err := url.Parse(baseURL)
	if err != nil {
		t.Fatalf("parse base URL: %v", err)
	}
	u.Path = "/remote.php/dav/calendars/family-house/vacation-house/"
	client, err := New(config.CalDAVConfig{
		URL:  u.String(),
		User: "family-house",
		Pass: "app-password",
	}, nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	return client
}

func multistatusXML(href, etag, calendarData string) string {
	return `<?xml version="1.0"?>
<multistatus xmlns="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav">
  <response>
    <href>` + href + `</href>
    <propstat>
      <prop>
        <getetag>` + etag + `</getetag>
        <C:calendar-data>` + escapeXML(calendarData) + `</C:calendar-data>
      </prop>
      <status>HTTP/1.1 200 OK</status>
    </propstat>
  </response>
</multistatus>`
}

func escapeXML(value string) string {
	value = strings.ReplaceAll(value, "&", "&amp;")
	value = strings.ReplaceAll(value, "<", "&lt;")
	value = strings.ReplaceAll(value, ">", "&gt;")
	return value
}
