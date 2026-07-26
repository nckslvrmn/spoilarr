// Package web serves spoilarr's HTMX-driven UI and JSON-free HTTP endpoints.
// Templates and static assets are compiled into the binary via go:embed so the
// whole app ships as a single file.
package web

import (
	"embed"
	"html/template"
	"log/slog"
	"net/http"

	"github.com/nckslvrmn/spoilarr/internal/config"
	"github.com/nckslvrmn/spoilarr/internal/store"
)

//go:embed templates/*.html
var templatesFS embed.FS

//go:embed static/*
var staticFS embed.FS

// Server holds dependencies shared by all handlers.
type Server struct {
	cfg   config.Config
	store *store.Store
	log   *slog.Logger
	tmpl  *template.Template
	pin   pendingPIN
	jobs  jobRunner
}

// New constructs the HTTP server and parses templates.
func New(cfg config.Config, st *store.Store, log *slog.Logger) (*Server, error) {
	tmpl, err := template.New("").Funcs(template.FuncMap{"jobData": jobData}).ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	return &Server{cfg: cfg, store: st, log: log, tmpl: tmpl}, nil
}

// jobData builds the data map the job_status fragment expects, for use from
// within other templates.
func jobData(key string, running bool, protected, revealed int, message string) map[string]any {
	return map[string]any{
		"Key": key, "Running": running, "Protected": protected, "Revealed": revealed, "Message": message,
	}
}

// Handler builds the HTTP route table.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.Handle("GET /static/", http.FileServer(http.FS(staticFS)))
	mux.HandleFunc("GET /{$}", s.handleHome)

	// Setup wizard (Plex sign-in + server selection).
	mux.HandleFunc("GET /setup", s.handleSetup)
	mux.HandleFunc("POST /setup/pin", s.handleSetupPINStart)
	mux.HandleFunc("GET /setup/pin/status", s.handleSetupPINStatus)
	mux.HandleFunc("POST /setup/server", s.handleSetupServer)

	// Library browsing and protect/unprotect.
	mux.HandleFunc("GET /libraries", s.handleLibraries)
	mux.HandleFunc("GET /libraries/{section}", s.handleSection)
	mux.HandleFunc("GET /shows/{ratingKey}", s.handleShow)
	mux.HandleFunc("POST /protect-seasons", s.handleProtectSeasons)
	mux.HandleFunc("POST /protect", s.handleProtect)
	mux.HandleFunc("POST /unprotect", s.handleUnprotect)
	mux.HandleFunc("GET /plex/image", s.handlePlexImage)
	mux.HandleFunc("POST /plex/webhook", s.handlePlexWebhook)

	// Protection dashboard (apply / reveal) + per-item settings.
	mux.HandleFunc("GET /protected", s.handleProtected)
	mux.HandleFunc("POST /protect-now", s.handleProtectNow)
	mux.HandleFunc("POST /reveal", s.handleReveal)
	mux.HandleFunc("GET /protected/{ratingKey}", s.handleItemSettings)
	mux.HandleFunc("GET /protected/{ratingKey}/status", s.handleItemStatus)
	mux.HandleFunc("POST /protected/{ratingKey}/settings", s.handleSaveSettings)
	mux.HandleFunc("POST /protected/{ratingKey}/reapply", s.handleReapply)

	return s.withLogging(mux)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	token, _ := s.store.GetConfig(cfgToken)
	serverURL, _ := s.store.GetConfig(cfgServerURL)
	serverName, _ := s.store.GetConfig(cfgServerName)
	s.render(w, "home.html", map[string]any{
		"Configured": token != "" && serverURL != "",
		"ServerName": serverName,
	})
}

// render executes a template within the shared layout and handles errors.
func (s *Server) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, name, data); err != nil {
		s.log.Error("template render failed", "template", name, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

// withLogging wraps a handler with structured request logging.
func (s *Server) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		s.log.Debug("request", "method", r.Method, "path", r.URL.Path)
	})
}
