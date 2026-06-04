package httpd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gregdel/booky/internal/booking"
	"github.com/gregdel/booky/internal/caldav"
)

const maxJSONBody = 16 << 10

type Store interface {
	List(context.Context, booking.QueryRange) ([]booking.Booking, error)
	Create(context.Context, booking.Booking) (booking.Booking, error)
	Update(context.Context, booking.Booking) (booking.Booking, error)
	Delete(context.Context, booking.Booking) error
}

type Server struct {
	store  Store
	assets fs.FS
}

func New(store Store, assets fs.FS) http.Handler {
	s := &Server{store: store, assets: assets}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/bookings", s.handleBookings)
	mux.HandleFunc("/api/bookings/", s.handleBooking)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})
	mux.Handle("/", http.FileServer(http.FS(assets)))

	return securityHeaders(mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/health" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleBookings(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/bookings" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.listBookings(w, r)
	case http.MethodPost:
		s.createBooking(w, r)
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPost)
	}
}

func (s *Server) handleBooking(w http.ResponseWriter, r *http.Request) {
	uid := strings.TrimPrefix(r.URL.Path, "/api/bookings/")
	if uid == "" || strings.Contains(uid, "/") {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	switch r.Method {
	case http.MethodPut:
		s.updateBooking(w, r, uid)
	case http.MethodDelete:
		s.deleteBooking(w, r, uid)
	default:
		methodNotAllowed(w, http.MethodPut, http.MethodDelete)
	}
}

func (s *Server) listBookings(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	rng := booking.QueryRange{
		Start: q.Get("start"),
		End:   q.Get("end"),
	}
	if err := rng.Validate(); err != nil {
		writeMappedError(w, err)
		return
	}

	bookings, err := s.store.List(r.Context(), rng)
	if err != nil {
		writeMappedError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, bookings)
}

func (s *Server) createBooking(w http.ResponseWriter, r *http.Request) {
	var req bookingRequest
	if err := decodeJSON(w, r, &req, false); err != nil {
		writeDecodeError(w, err)
		return
	}

	b := booking.Booking{
		Name:  req.Name,
		Start: req.Start,
		End:   req.End,
		Note:  req.Note,
	}
	if err := b.Validate(); err != nil {
		writeMappedError(w, err)
		return
	}

	created, err := s.store.Create(r.Context(), b)
	if err != nil {
		writeMappedError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) updateBooking(w http.ResponseWriter, r *http.Request, uid string) {
	var req bookingRequest
	if err := decodeJSON(w, r, &req, false); err != nil {
		writeDecodeError(w, err)
		return
	}
	if req.UID != "" && req.UID != uid {
		writeError(w, http.StatusBadRequest, "uid does not match path")
		return
	}

	b := booking.Booking{
		UID:   uid,
		Href:  req.Href,
		ETag:  req.ETag,
		Name:  req.Name,
		Start: req.Start,
		End:   req.End,
		Note:  req.Note,
	}
	if err := b.Validate(); err != nil {
		writeMappedError(w, err)
		return
	}

	updated, err := s.store.Update(r.Context(), b)
	if err != nil {
		writeMappedError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) deleteBooking(w http.ResponseWriter, r *http.Request, uid string) {
	var req deleteRequest
	if err := decodeJSON(w, r, &req, true); err != nil {
		writeDecodeError(w, err)
		return
	}

	if err := s.store.Delete(r.Context(), booking.Booking{UID: uid, Href: req.Href, ETag: req.ETag}); err != nil {
		writeMappedError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type bookingRequest struct {
	UID   string `json:"uid"`
	Href  string `json:"href"`
	ETag  string `json:"etag"`
	Name  string `json:"name"`
	Start string `json:"start"`
	End   string `json:"end"`
	Note  string `json:"note"`
}

type deleteRequest struct {
	Href string `json:"href"`
	ETag string `json:"etag"`
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any, allowEmpty bool) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
	defer r.Body.Close()

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		if allowEmpty && errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain a single JSON object")
	}
	return nil
}

func writeDecodeError(w http.ResponseWriter, err error) {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}
	writeError(w, http.StatusBadRequest, err.Error())
}

func writeMappedError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, caldav.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, caldav.ErrConflict):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, caldav.ErrUpstream):
		writeError(w, http.StatusBadGateway, err.Error())
	default:
		writeError(w, http.StatusBadRequest, err.Error())
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func methodNotAllowed(w http.ResponseWriter, allowed ...string) {
	w.Header().Set("Allow", strings.Join(allowed, ", "))
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; font-src 'self' data:; base-uri 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}
