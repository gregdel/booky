package httpd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gregdel/booky/internal/booking"
	"github.com/gregdel/booky/internal/caldav"
)

func TestHealthAndStatic(t *testing.T) {
	handler := New(&fakeStore{}, testAssets(), "", "Vacation House")

	resp := request(handler, http.MethodGet, "/api/health", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("health status = %d", resp.Code)
	}
	assertJSON(t, resp.Body.String(), map[string]string{"status": "ok"})

	resp = request(handler, http.MethodGet, "/", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("static status = %d", resp.Code)
	}
	if !strings.Contains(resp.Body.String(), "Vacation House") {
		t.Fatalf("static body = %q", resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `href="/style.css"`) || !strings.Contains(resp.Body.String(), `src="/app.js"`) {
		t.Fatalf("static links = %q", resp.Body.String())
	}
	for _, want := range []string{
		`<title>Vacation House</title>`,
		`content="Vacation House"`,
		`href="/manifest.webmanifest"`,
		`href="/icon.svg"`,
		`href="/apple-touch-icon.png"`,
	} {
		if !strings.Contains(resp.Body.String(), want) {
			t.Fatalf("static body missing %q: %s", want, resp.Body.String())
		}
	}
	for _, header := range []string{"X-Content-Type-Options", "Referrer-Policy", "Content-Security-Policy"} {
		if resp.Header().Get(header) == "" {
			t.Fatalf("%s header missing", header)
		}
	}
	csp := resp.Header().Get("Content-Security-Policy")
	for _, want := range []string{"style-src 'self' 'unsafe-inline'", "font-src 'self' data:"} {
		if !strings.Contains(csp, want) {
			t.Fatalf("CSP = %q, want %q", csp, want)
		}
	}

	for _, path := range []string{"/app.js", "/style.css", "/icon.svg", "/icon-192.png", "/icon-512.png", "/apple-touch-icon.png"} {
		t.Run(path, func(t *testing.T) {
			resp := request(handler, http.MethodGet, path, nil)
			if resp.Code != http.StatusOK {
				t.Fatalf("%s status = %d", path, resp.Code)
			}
			if resp.Body.Len() == 0 {
				t.Fatalf("%s returned empty body", path)
			}
		})
	}

	resp = request(handler, http.MethodGet, "/manifest.webmanifest", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("manifest status = %d body = %s", resp.Code, resp.Body.String())
	}
	if got := resp.Header().Get("Content-Type"); got != "application/manifest+json" {
		t.Fatalf("manifest content type = %q", got)
	}
	var manifest webManifest
	decodeResponse(t, resp, &manifest)
	assertManifest(t, manifest, "Vacation House", "/", "/", "/icon-192.png", "/icon-512.png")

	resp = request(handler, http.MethodGet, "/missing.js", nil)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("missing static status = %d", resp.Code)
	}
}

func TestAppTitleEscaping(t *testing.T) {
	handler := New(&fakeStore{}, testAssets(), "", `House & "Cabin"`)

	resp := request(handler, http.MethodGet, "/", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("static status = %d", resp.Code)
	}
	for _, want := range []string{
		`<title>House &amp; &#34;Cabin&#34;</title>`,
		`<h1>House &amp; &#34;Cabin&#34;</h1>`,
		`content="House &amp; &#34;Cabin&#34;"`,
	} {
		if !strings.Contains(resp.Body.String(), want) {
			t.Fatalf("static body missing %q: %s", want, resp.Body.String())
		}
	}

	resp = request(handler, http.MethodGet, "/manifest.webmanifest", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("manifest status = %d", resp.Code)
	}
	var manifest webManifest
	decodeResponse(t, resp, &manifest)
	if manifest.Name != `House & "Cabin"` || manifest.ShortName != `House & "Cabin"` {
		t.Fatalf("manifest title = %#v", manifest)
	}
}

func TestPublicPath(t *testing.T) {
	handler := New(&fakeStore{}, testAssets(), "/REPLACE_WITH_LONG_RANDOM_PATH", "Vacation House")

	resp := request(handler, http.MethodGet, "/REPLACE_WITH_LONG_RANDOM_PATH", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("prefixed static status = %d", resp.Code)
	}
	if !strings.Contains(resp.Body.String(), `href="/REPLACE_WITH_LONG_RANDOM_PATH/style.css"`) ||
		!strings.Contains(resp.Body.String(), `src="/REPLACE_WITH_LONG_RANDOM_PATH/app.js"`) {
		t.Fatalf("prefixed static links = %q", resp.Body.String())
	}
	for _, want := range []string{
		`href="/REPLACE_WITH_LONG_RANDOM_PATH/manifest.webmanifest"`,
		`href="/REPLACE_WITH_LONG_RANDOM_PATH/icon.svg"`,
		`href="/REPLACE_WITH_LONG_RANDOM_PATH/apple-touch-icon.png"`,
	} {
		if !strings.Contains(resp.Body.String(), want) {
			t.Fatalf("prefixed static body missing %q: %s", want, resp.Body.String())
		}
	}

	resp = request(handler, http.MethodGet, "/REPLACE_WITH_LONG_RANDOM_PATH/", nil)
	if resp.Code != http.StatusMovedPermanently {
		t.Fatalf("trailing slash status = %d", resp.Code)
	}
	if resp.Header().Get("Location") != "/REPLACE_WITH_LONG_RANDOM_PATH" {
		t.Fatalf("trailing slash location = %q", resp.Header().Get("Location"))
	}

	for _, path := range []string{"/REPLACE_WITH_LONG_RANDOM_PATH/api/health", "/REPLACE_WITH_LONG_RANDOM_PATH/app.js", "/REPLACE_WITH_LONG_RANDOM_PATH/style.css", "/REPLACE_WITH_LONG_RANDOM_PATH/index.html", "/REPLACE_WITH_LONG_RANDOM_PATH/icon.svg", "/REPLACE_WITH_LONG_RANDOM_PATH/icon-192.png", "/REPLACE_WITH_LONG_RANDOM_PATH/icon-512.png", "/REPLACE_WITH_LONG_RANDOM_PATH/apple-touch-icon.png"} {
		t.Run(path, func(t *testing.T) {
			resp := request(handler, http.MethodGet, path, nil)
			if resp.Code != http.StatusOK {
				t.Fatalf("%s status = %d body = %s", path, resp.Code, resp.Body.String())
			}
		})
	}

	resp = request(handler, http.MethodGet, "/REPLACE_WITH_LONG_RANDOM_PATH/manifest.webmanifest", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("prefixed manifest status = %d body = %s", resp.Code, resp.Body.String())
	}
	var manifest webManifest
	decodeResponse(t, resp, &manifest)
	assertManifest(t, manifest, "Vacation House", "/REPLACE_WITH_LONG_RANDOM_PATH", "/REPLACE_WITH_LONG_RANDOM_PATH", "/REPLACE_WITH_LONG_RANDOM_PATH/icon-192.png", "/REPLACE_WITH_LONG_RANDOM_PATH/icon-512.png")

	for _, path := range []string{"/", "/api/health", "/app.js", "/style.css", "/manifest.webmanifest", "/REPLACE_WITH_LONG_RANDOM_PATH-extra/api/health"} {
		t.Run(path, func(t *testing.T) {
			resp := request(handler, http.MethodGet, path, nil)
			if resp.Code != http.StatusNotFound {
				t.Fatalf("%s status = %d body = %s", path, resp.Code, resp.Body.String())
			}
		})
	}
}

func TestListBookings(t *testing.T) {
	store := &fakeStore{
		listResult: []booking.Booking{{UID: "uid-1", Name: "Family stay", Start: "2026-07-10", End: "2026-07-17"}},
	}
	handler := testHandler(store)

	resp := request(handler, http.MethodGet, "/api/bookings?start=2026-07-01&end=2026-08-01", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if store.listRange.Start != "2026-07-01" || store.listRange.End != "2026-08-01" {
		t.Fatalf("range = %#v", store.listRange)
	}
	var got []booking.Booking
	decodeResponse(t, resp, &got)
	if len(got) != 1 || got[0].UID != "uid-1" {
		t.Fatalf("bookings = %#v", got)
	}
}

func TestListBookingsReturnsEmptyArray(t *testing.T) {
	handler := testHandler(&fakeStore{})

	resp := request(handler, http.MethodGet, "/api/bookings?start=2026-07-01&end=2026-08-01", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if strings.TrimSpace(resp.Body.String()) != "[]" {
		t.Fatalf("body = %q, want []", resp.Body.String())
	}
}

func TestListBookingsValidatesQuery(t *testing.T) {
	store := &fakeStore{}
	handler := testHandler(store)

	resp := request(handler, http.MethodGet, "/api/bookings?start=2026-07-01", nil)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	assertErrorShape(t, resp)
	if store.listCalls != 0 {
		t.Fatalf("list calls = %d, want 0", store.listCalls)
	}
}

func TestCreateBookingIgnoresClientMetadata(t *testing.T) {
	store := &fakeStore{
		createResult: booking.Booking{UID: "server-uid", Name: "Family stay", Start: "2026-07-10", End: "2026-07-17"},
	}
	handler := testHandler(store)

	body := `{"uid":"client-uid","etag":"etag","name":"Family stay","start":"2026-07-10","end":"2026-07-17","note":"note"}`
	resp := request(handler, http.MethodPost, "/api/bookings", strings.NewReader(body))
	if resp.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if store.createBooking.UID != "" || store.createBooking.ETag != "" {
		t.Fatalf("create booking kept metadata: %#v", store.createBooking)
	}
	if store.createBooking.Name != "Family stay" || store.createBooking.Note != "note" {
		t.Fatalf("create booking = %#v", store.createBooking)
	}
}

func TestCreateBookingRejectsHref(t *testing.T) {
	store := &fakeStore{}
	handler := testHandler(store)

	resp := request(handler, http.MethodPost, "/api/bookings", strings.NewReader(`{"href":"href","name":"Family stay","start":"2026-07-10","end":"2026-07-17"}`))
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	assertErrorShape(t, resp)
	if store.createCalls != 0 {
		t.Fatalf("create calls = %d, want 0", store.createCalls)
	}
}

func TestCreateBookingValidatesBeforeStore(t *testing.T) {
	store := &fakeStore{}
	handler := testHandler(store)

	resp := request(handler, http.MethodPost, "/api/bookings", strings.NewReader(`{"name":"","start":"2026-07-10","end":"2026-07-17"}`))
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	assertErrorShape(t, resp)
	if store.createCalls != 0 {
		t.Fatalf("create calls = %d, want 0", store.createCalls)
	}
}

func TestUpdateBookingUsesPathUID(t *testing.T) {
	store := &fakeStore{
		updateResult: booking.Booking{UID: "path-uid", Name: "Family stay", Start: "2026-07-10", End: "2026-07-17"},
	}
	handler := testHandler(store)

	body := `{"uid":"path-uid","etag":"  etag  ","name":"Family stay","start":"2026-07-10","end":"2026-07-17"}`
	resp := request(handler, http.MethodPut, "/api/bookings/path-uid", strings.NewReader(body))
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if store.updateBooking.UID != "path-uid" || store.updateBooking.ETag != "etag" {
		t.Fatalf("update booking = %#v", store.updateBooking)
	}
}

func TestUpdateBookingRequiresETag(t *testing.T) {
	tests := map[string]string{
		"missing": `{"name":"Family stay","start":"2026-07-10","end":"2026-07-17"}`,
		"blank":   `{"etag":"   ","name":"Family stay","start":"2026-07-10","end":"2026-07-17"}`,
	}

	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			store := &fakeStore{}
			handler := testHandler(store)

			resp := request(handler, http.MethodPut, "/api/bookings/path-uid", strings.NewReader(body))
			if resp.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
			}
			assertErrorMessage(t, resp, "etag is required")
			if store.updateCalls != 0 {
				t.Fatalf("update calls = %d, want 0", store.updateCalls)
			}
		})
	}
}

func TestUpdateBookingRejectsHref(t *testing.T) {
	store := &fakeStore{}
	handler := testHandler(store)

	resp := request(handler, http.MethodPut, "/api/bookings/path-uid", strings.NewReader(`{"href":"href","name":"Family stay","start":"2026-07-10","end":"2026-07-17"}`))
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	assertErrorShape(t, resp)
	if store.updateCalls != 0 {
		t.Fatalf("update calls = %d, want 0", store.updateCalls)
	}
}

func TestUpdateRejectsMismatchedUID(t *testing.T) {
	handler := testHandler(&fakeStore{})
	resp := request(handler, http.MethodPut, "/api/bookings/path-uid", strings.NewReader(`{"uid":"other","name":"Family stay","start":"2026-07-10","end":"2026-07-17"}`))
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	assertErrorShape(t, resp)
}

func TestUpdateBookingValidatesBeforeStore(t *testing.T) {
	store := &fakeStore{}
	handler := testHandler(store)

	resp := request(handler, http.MethodPut, "/api/bookings/path-uid", strings.NewReader(`{"etag":"etag","name":"Family stay","start":"2026-07-17","end":"2026-07-10"}`))
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	assertErrorShape(t, resp)
	if store.updateCalls != 0 {
		t.Fatalf("update calls = %d, want 0", store.updateCalls)
	}
}

func TestDeleteBookingRequiresETag(t *testing.T) {
	tests := map[string]ioReader{
		"empty":   nil,
		"missing": strings.NewReader(`{}`),
		"blank":   strings.NewReader(`{"etag":"   "}`),
	}

	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			store := &fakeStore{}
			handler := testHandler(store)

			resp := request(handler, http.MethodDelete, "/api/bookings/path-uid", body)
			if resp.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
			}
			assertErrorMessage(t, resp, "etag is required")
			if store.deleteCalls != 0 {
				t.Fatalf("delete calls = %d, want 0", store.deleteCalls)
			}
		})
	}
}

func TestDeleteBookingAcceptsETagBody(t *testing.T) {
	store := &fakeStore{}
	handler := testHandler(store)

	resp := request(handler, http.MethodDelete, "/api/bookings/path-uid", strings.NewReader(`{"etag":"  etag  "}`))
	if resp.Code != http.StatusNoContent {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if store.deleteBooking.ETag != "etag" {
		t.Fatalf("delete booking = %#v", store.deleteBooking)
	}
}

func TestDeleteBookingRejectsHref(t *testing.T) {
	store := &fakeStore{}
	handler := testHandler(store)

	resp := request(handler, http.MethodDelete, "/api/bookings/path-uid", strings.NewReader(`{"href":"href","etag":"etag"}`))
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	assertErrorShape(t, resp)
	if store.deleteCalls != 0 {
		t.Fatalf("delete calls = %d, want 0", store.deleteCalls)
	}
}

func TestMutationConflictsReturnConflict(t *testing.T) {
	handler := testHandler(&fakeStore{
		updateErr: caldav.ErrConflict,
		deleteErr: caldav.ErrConflict,
	})

	resp := request(handler, http.MethodPut, "/api/bookings/path-uid", strings.NewReader(`{"etag":"etag","name":"Family stay","start":"2026-07-10","end":"2026-07-17"}`))
	if resp.Code != http.StatusConflict {
		t.Fatalf("update status = %d body = %s", resp.Code, resp.Body.String())
	}
	assertErrorShape(t, resp)

	resp = request(handler, http.MethodDelete, "/api/bookings/path-uid", strings.NewReader(`{"etag":"etag"}`))
	if resp.Code != http.StatusConflict {
		t.Fatalf("delete status = %d body = %s", resp.Code, resp.Body.String())
	}
	assertErrorShape(t, resp)
}

func TestStrictJSONAndBodyLimit(t *testing.T) {
	handler := testHandler(&fakeStore{})

	resp := request(handler, http.MethodPost, "/api/bookings", strings.NewReader(`{"name":"Family stay","start":"2026-07-10","end":"2026-07-17","extra":true}`))
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("unknown field status = %d", resp.Code)
	}
	assertErrorShape(t, resp)

	large := bytes.NewBufferString(`{"name":"` + strings.Repeat("a", maxJSONBody) + `"}`)
	resp = request(handler, http.MethodPost, "/api/bookings", large)
	if resp.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("large body status = %d body = %s", resp.Code, resp.Body.String())
	}
	assertErrorShape(t, resp)
}

func TestErrorMappingAndMethods(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "not found", err: caldav.ErrNotFound, want: http.StatusNotFound},
		{name: "conflict", err: caldav.ErrConflict, want: http.StatusConflict},
		{name: "upstream", err: caldav.ErrUpstream, want: http.StatusBadGateway},
		{name: "validation", err: errors.New("bad request"), want: http.StatusBadRequest},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			handler := testHandler(&fakeStore{listErr: tc.err})
			resp := request(handler, http.MethodGet, "/api/bookings?start=2026-07-01&end=2026-08-01", nil)
			if resp.Code != tc.want {
				t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
			}
			assertErrorShape(t, resp)
		})
	}

	handler := testHandler(&fakeStore{})
	resp := request(handler, http.MethodPatch, "/api/bookings", nil)
	if resp.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", resp.Code)
	}
	assertErrorShape(t, resp)

	resp = request(handler, http.MethodGet, "/api/missing", nil)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d", resp.Code)
	}
	assertErrorShape(t, resp)
}

type fakeStore struct {
	listRange  booking.QueryRange
	listResult []booking.Booking
	listErr    error
	listCalls  int

	createBooking booking.Booking
	createResult  booking.Booking
	createErr     error
	createCalls   int

	updateBooking booking.Booking
	updateResult  booking.Booking
	updateErr     error
	updateCalls   int

	deleteBooking booking.Booking
	deleteErr     error
	deleteCalls   int
}

func (s *fakeStore) List(ctx context.Context, r booking.QueryRange) ([]booking.Booking, error) {
	s.listCalls++
	s.listRange = r
	return s.listResult, s.listErr
}

func (s *fakeStore) Create(ctx context.Context, b booking.Booking) (booking.Booking, error) {
	s.createCalls++
	s.createBooking = b
	if s.createErr != nil {
		return booking.Booking{}, s.createErr
	}
	return s.createResult, nil
}

func (s *fakeStore) Update(ctx context.Context, b booking.Booking) (booking.Booking, error) {
	s.updateCalls++
	s.updateBooking = b
	if s.updateErr != nil {
		return booking.Booking{}, s.updateErr
	}
	return s.updateResult, nil
}

func (s *fakeStore) Delete(ctx context.Context, b booking.Booking) error {
	s.deleteCalls++
	s.deleteBooking = b
	return s.deleteErr
}

func request(handler http.Handler, method, path string, body ioReader) *httptest.ResponseRecorder {
	var reader ioReader
	if body != nil {
		reader = body
	}
	req := httptest.NewRequest(method, path, reader)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	return resp
}

type ioReader interface {
	Read([]byte) (int, error)
}

func testAssets() fs.FS {
	return fstest.MapFS{
		"index.html":           {Data: []byte(`<!doctype html><title>{{APP_TITLE}}</title><meta name="apple-mobile-web-app-title" content="{{APP_TITLE}}"><link rel="manifest" href="{{PUBLIC_PATH}}/manifest.webmanifest"><link rel="icon" type="image/svg+xml" href="{{PUBLIC_PATH}}/icon.svg"><link rel="apple-touch-icon" href="{{PUBLIC_PATH}}/apple-touch-icon.png"><link rel="stylesheet" href="{{PUBLIC_PATH}}/style.css"><script src="{{PUBLIC_PATH}}/app.js"></script><h1>{{APP_TITLE}}</h1>`)},
		"app.js":               {Data: []byte("console.log('ok')")},
		"style.css":            {Data: []byte("body{}")},
		"icon.svg":             {Data: []byte("<svg></svg>")},
		"icon-192.png":         {Data: []byte("png192")},
		"icon-512.png":         {Data: []byte("png512")},
		"apple-touch-icon.png": {Data: []byte("png180")},
	}
}

func testHandler(store Store) http.Handler {
	return New(store, testAssets(), "", "booky")
}

type webManifest struct {
	Name            string         `json:"name"`
	ShortName       string         `json:"short_name"`
	ID              string         `json:"id"`
	StartURL        string         `json:"start_url"`
	Scope           string         `json:"scope"`
	Display         string         `json:"display"`
	ThemeColor      string         `json:"theme_color"`
	BackgroundColor string         `json:"background_color"`
	Icons           []manifestIcon `json:"icons"`
}

type manifestIcon struct {
	Src   string `json:"src"`
	Sizes string `json:"sizes"`
	Type  string `json:"type"`
}

func assertManifest(t *testing.T, manifest webManifest, title, startURL, scope, icon192, icon512 string) {
	t.Helper()
	if manifest.Name != title || manifest.ShortName != title {
		t.Fatalf("manifest title = %#v, want %q", manifest, title)
	}
	if manifest.ID != startURL || manifest.StartURL != startURL || manifest.Scope != scope {
		t.Fatalf("manifest navigation = %#v", manifest)
	}
	if manifest.Display != "standalone" || manifest.ThemeColor != "#4f6f8f" || manifest.BackgroundColor != "#f6f4ef" {
		t.Fatalf("manifest display = %#v", manifest)
	}
	if len(manifest.Icons) != 2 {
		t.Fatalf("manifest icons = %#v", manifest.Icons)
	}
	wantIcons := []manifestIcon{
		{Src: icon192, Sizes: "192x192", Type: "image/png"},
		{Src: icon512, Sizes: "512x512", Type: "image/png"},
	}
	for i, want := range wantIcons {
		if manifest.Icons[i] != want {
			t.Fatalf("manifest icon[%d] = %#v, want %#v", i, manifest.Icons[i], want)
		}
	}
}

func decodeResponse(t *testing.T, resp *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.Unmarshal(resp.Body.Bytes(), dst); err != nil {
		t.Fatalf("decode response: %v; body = %s", err, resp.Body.String())
	}
}

func assertJSON(t *testing.T, got string, want map[string]string) {
	t.Helper()
	var decoded map[string]string
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("decode JSON: %v; body = %s", err, got)
	}
	for key, value := range want {
		if decoded[key] != value {
			t.Fatalf("JSON[%q] = %q, want %q", key, decoded[key], value)
		}
	}
}

func assertErrorShape(t *testing.T, resp *httptest.ResponseRecorder) {
	t.Helper()
	var decoded map[string]string
	if err := json.Unmarshal(resp.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("decode error response: %v; body = %s", err, resp.Body.String())
	}
	if decoded["error"] == "" {
		t.Fatalf("error response missing message: %v", decoded)
	}
}

func assertErrorMessage(t *testing.T, resp *httptest.ResponseRecorder, want string) {
	t.Helper()
	var decoded map[string]string
	if err := json.Unmarshal(resp.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("decode error response: %v; body = %s", err, resp.Body.String())
	}
	if decoded["error"] != want {
		t.Fatalf("error = %q, want %q", decoded["error"], want)
	}
}
