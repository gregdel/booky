package httpd

import (
	"context"
	"encoding/json"
	"errors"
	"html"
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
	store      Store
	assets     fs.FS
	publicPath string
	appTitle   string
}

func New(store Store, assets fs.FS, publicPath, appTitle string) http.Handler {
	publicPath = cleanPublicPath(publicPath)
	appTitle = strings.TrimSpace(appTitle)
	if appTitle == "" {
		appTitle = "booky"
	}
	s := &Server{store: store, assets: assets, publicPath: publicPath, appTitle: appTitle}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/bookings", s.handleBookings)
	mux.HandleFunc("/api/bookings/", s.handleBooking)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})
	mux.HandleFunc("/", s.handleAsset)

	return securityHeaders(s.withPublicPath(mux))
}

func cleanPublicPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "/" {
		return ""
	}
	return strings.TrimRight(path, "/")
}

func (s *Server) withPublicPath(next http.Handler) http.Handler {
	if s.publicPath == "" {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == s.publicPath+"/" {
			http.Redirect(w, r, s.publicPath, http.StatusMovedPermanently)
			return
		}
		if r.URL.Path == s.publicPath {
			r = r.Clone(r.Context())
			r.URL.Path = "/"
			r.URL.RawPath = ""
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, s.publicPath+"/") {
			http.StripPrefix(s.publicPath, next).ServeHTTP(w, r)
			return
		}
		writeError(w, http.StatusNotFound, "not found")
	})
}

func (s *Server) handleAsset(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" || r.URL.Path == "/index.html" {
		s.handleIndex(w, r)
		return
	}
	if r.URL.Path == "/manifest.webmanifest" {
		s.handleManifest(w, r)
		return
	}
	http.FileServer(http.FS(s.assets)).ServeHTTP(w, r)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	index, err := fs.ReadFile(s.assets, "index.html")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "index not found")
		return
	}

	body := strings.ReplaceAll(string(index), "{{PUBLIC_PATH}}", html.EscapeString(s.publicPath))
	body = strings.ReplaceAll(body, "{{APP_TITLE}}", html.EscapeString(s.appTitle))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, body)
}

func (s *Server) handleManifest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		methodNotAllowed(w, http.MethodGet, http.MethodHead)
		return
	}

	publicPath := s.publicPath
	startURL := publicPath
	if startURL == "" {
		startURL = "/"
	}
	manifest := map[string]any{
		"name":             s.appTitle,
		"short_name":       s.appTitle,
		"id":               startURL,
		"start_url":        startURL,
		"scope":            startURL,
		"display":          "standalone",
		"theme_color":      "#4f6f8f",
		"background_color": "#f6f4ef",
		"icons": []map[string]string{
			{
				"src":   publicPath + "/icon-192.png",
				"sizes": "192x192",
				"type":  "image/png",
			},
			{
				"src":   publicPath + "/icon-512.png",
				"sizes": "512x512",
				"type":  "image/png",
			},
		},
	}

	w.Header().Set("Content-Type", "application/manifest+json")
	if r.Method == http.MethodHead {
		return
	}
	_ = json.NewEncoder(w).Encode(manifest)
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
	if bookings == nil {
		bookings = []booking.Booking{}
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
