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
	handler := New(&fakeStore{}, testAssets())

	resp := request(handler, http.MethodGet, "/api/health", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("health status = %d", resp.Code)
	}
	assertJSON(t, resp.Body.String(), map[string]string{"status": "ok"})

	resp = request(handler, http.MethodGet, "/", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("static status = %d", resp.Code)
	}
	if !strings.Contains(resp.Body.String(), "booky") {
		t.Fatalf("static body = %q", resp.Body.String())
	}
	for _, header := range []string{"X-Content-Type-Options", "Referrer-Policy", "Content-Security-Policy"} {
		if resp.Header().Get(header) == "" {
			t.Fatalf("%s header missing", header)
		}
	}

	resp = request(handler, http.MethodGet, "/missing.js", nil)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("missing static status = %d", resp.Code)
	}
}

func TestListBookings(t *testing.T) {
	store := &fakeStore{
		listResult: []booking.Booking{{UID: "uid-1", Name: "Family stay", Start: "2026-07-10", End: "2026-07-17"}},
	}
	handler := New(store, testAssets())

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

func TestListBookingsValidatesQuery(t *testing.T) {
	store := &fakeStore{}
	handler := New(store, testAssets())

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
	handler := New(store, testAssets())

	body := `{"uid":"client-uid","href":"href","etag":"etag","name":"Family stay","start":"2026-07-10","end":"2026-07-17","note":"note"}`
	resp := request(handler, http.MethodPost, "/api/bookings", strings.NewReader(body))
	if resp.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if store.createBooking.UID != "" || store.createBooking.Href != "" || store.createBooking.ETag != "" {
		t.Fatalf("create booking kept metadata: %#v", store.createBooking)
	}
	if store.createBooking.Name != "Family stay" || store.createBooking.Note != "note" {
		t.Fatalf("create booking = %#v", store.createBooking)
	}
}

func TestCreateBookingValidatesBeforeStore(t *testing.T) {
	store := &fakeStore{}
	handler := New(store, testAssets())

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
	handler := New(store, testAssets())

	body := `{"uid":"path-uid","href":"href","etag":"etag","name":"Family stay","start":"2026-07-10","end":"2026-07-17"}`
	resp := request(handler, http.MethodPut, "/api/bookings/path-uid", strings.NewReader(body))
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if store.updateBooking.UID != "path-uid" || store.updateBooking.Href != "href" || store.updateBooking.ETag != "etag" {
		t.Fatalf("update booking = %#v", store.updateBooking)
	}
}

func TestUpdateRejectsMismatchedUID(t *testing.T) {
	handler := New(&fakeStore{}, testAssets())
	resp := request(handler, http.MethodPut, "/api/bookings/path-uid", strings.NewReader(`{"uid":"other","name":"Family stay","start":"2026-07-10","end":"2026-07-17"}`))
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	assertErrorShape(t, resp)
}

func TestUpdateBookingValidatesBeforeStore(t *testing.T) {
	store := &fakeStore{}
	handler := New(store, testAssets())

	resp := request(handler, http.MethodPut, "/api/bookings/path-uid", strings.NewReader(`{"name":"Family stay","start":"2026-07-17","end":"2026-07-10"}`))
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	assertErrorShape(t, resp)
	if store.updateCalls != 0 {
		t.Fatalf("update calls = %d, want 0", store.updateCalls)
	}
}

func TestDeleteBookingAllowsEmptyBody(t *testing.T) {
	store := &fakeStore{}
	handler := New(store, testAssets())

	resp := request(handler, http.MethodDelete, "/api/bookings/path-uid", nil)
	if resp.Code != http.StatusNoContent {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if store.deleteBooking.UID != "path-uid" {
		t.Fatalf("delete booking = %#v", store.deleteBooking)
	}
}

func TestDeleteBookingAcceptsMetadataBody(t *testing.T) {
	store := &fakeStore{}
	handler := New(store, testAssets())

	resp := request(handler, http.MethodDelete, "/api/bookings/path-uid", strings.NewReader(`{"href":"href","etag":"etag"}`))
	if resp.Code != http.StatusNoContent {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if store.deleteBooking.Href != "href" || store.deleteBooking.ETag != "etag" {
		t.Fatalf("delete booking = %#v", store.deleteBooking)
	}
}

func TestStrictJSONAndBodyLimit(t *testing.T) {
	handler := New(&fakeStore{}, testAssets())

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
			handler := New(&fakeStore{listErr: tc.err}, testAssets())
			resp := request(handler, http.MethodGet, "/api/bookings?start=2026-07-01&end=2026-08-01", nil)
			if resp.Code != tc.want {
				t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
			}
			assertErrorShape(t, resp)
		})
	}

	handler := New(&fakeStore{}, testAssets())
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
		"index.html": {Data: []byte("<!doctype html><title>booky</title><h1>booky</h1>")},
		"app.js":     {Data: []byte("console.log('ok')")},
		"style.css":  {Data: []byte("body{}")},
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
