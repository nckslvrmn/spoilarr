package web

import (
	"net/http"
	"strings"
	"sync"

	"github.com/nckslvrmn/spoilarr/internal/plex"
)

// pendingPIN holds the in-flight sign-in PIN between the "start" request and the
// polling requests. Single-user local app, so one in-memory slot is enough; a
// process restart mid-flow just means restarting the wizard.
type pendingPIN struct {
	mu       sync.Mutex
	id       int
	clientID string
	active   bool
}

func (p *pendingPIN) set(id int, clientID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.id, p.clientID, p.active = id, clientID, true
}

func (p *pendingPIN) get() (int, string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.id, p.clientID, p.active
}

func (p *pendingPIN) clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.active = false
}

// handleSetup renders the setup wizard, routing to the right step based on what
// is already configured.
func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	token, _ := s.store.GetConfig(cfgToken)
	serverURL, _ := s.store.GetConfig(cfgServerURL)
	serverName, _ := s.store.GetConfig(cfgServerName)

	data := map[string]any{}
	switch {
	case token != "" && serverURL != "":
		data["Step"] = "done"
		data["ServerName"] = serverName
		data["WebhookURL"] = s.webhookURL(r)
	case token != "":
		// Signed in but no server chosen — discover and offer the picker.
		data["Step"] = "server"
		if client, err := s.plexClient(); err == nil {
			if servers, err := client.ListServers(r.Context()); err == nil {
				data["Servers"] = servers
			} else {
				s.log.Warn("server discovery failed", "err", err)
			}
		}
	default:
		data["Step"] = "auth"
	}
	s.render(w, "setup.html", data)
}

// handleSetupPINStart creates a plex.tv PIN and returns the "authorize" fragment
// that links the user to Plex and begins polling for completion.
func (s *Server) handleSetupPINStart(w http.ResponseWriter, r *http.Request) {
	client, err := s.plexClient()
	if err != nil {
		s.fragmentError(w, "setup", err)
		return
	}
	pin, err := client.CreatePIN(r.Context())
	if err != nil {
		s.fragmentError(w, "setup", err)
		return
	}
	s.pin.set(pin.ID, client.ClientID)
	s.render(w, "setup_pin_pending", map[string]any{"AuthURL": client.AuthURL(pin.Code)})
}

// handleSetupPINStatus is polled by htmx. It reports whether the PIN has been
// authorized yet; once it has, it stores the token and advances to server
// selection (which replaces the polling element, ending the poll).
func (s *Server) handleSetupPINStatus(w http.ResponseWriter, r *http.Request) {
	id, clientID, active := s.pin.get()
	if !active {
		s.render(w, "setup_auth", nil)
		return
	}
	client := plex.New(clientID, "", "")
	pin, err := client.CheckPIN(r.Context(), id)
	if err != nil {
		s.fragmentError(w, "setup", err)
		return
	}
	if pin.AuthToken == "" {
		// Still waiting — re-render just the poll element so the auth link above
		// it stays put and htmx keeps polling.
		s.render(w, "setup_poll", nil)
		return
	}

	// Authorized. Persist the token, discover servers, move to server step.
	// The poll element requested this; retarget the swap at the whole wizard so
	// the server picker replaces the entire step (and polling stops).
	s.pin.clear()
	if err := s.store.SetConfig(cfgToken, pin.AuthToken); err != nil {
		s.fragmentError(w, "setup", err)
		return
	}
	w.Header().Set("HX-Retarget", "#wizard")
	w.Header().Set("HX-Reswap", "innerHTML")
	s.renderServerStep(w, r)
}

// renderServerStep discovers reachable servers and renders the picker.
func (s *Server) renderServerStep(w http.ResponseWriter, r *http.Request) {
	client, err := s.plexClient()
	if err != nil {
		s.fragmentError(w, "setup", err)
		return
	}
	servers, err := client.ListServers(r.Context())
	if err != nil {
		s.fragmentError(w, "setup", err)
		return
	}
	s.render(w, "setup_server", map[string]any{"Servers": servers})
}

// handleSetupServer validates and stores the chosen server connection.
func (s *Server) handleSetupServer(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fragmentError(w, "setup", err)
		return
	}
	// A manual URL, if given, wins over the discovered radio choice.
	serverURL := strings.TrimSpace(r.FormValue("manual_url"))
	serverName := strings.TrimSpace(r.FormValue("manual_name"))
	if serverURL == "" {
		// choice is "Name|https://uri"
		if choice := r.FormValue("choice"); choice != "" {
			if i := strings.Index(choice, "|"); i >= 0 {
				serverName, serverURL = choice[:i], choice[i+1:]
			}
		}
	}
	if serverURL == "" {
		s.render(w, "setup_server_error", map[string]any{"Error": "no server selected"})
		return
	}
	if serverName == "" {
		serverName = serverURL
	}

	client, err := s.plexClient()
	if err != nil {
		s.fragmentError(w, "setup", err)
		return
	}
	client.ServerURL = serverURL

	id, err := client.Ping(r.Context())
	if err != nil {
		s.render(w, "setup_server_error", map[string]any{"Error": err.Error()})
		return
	}

	s.store.SetConfig(cfgServerURL, serverURL)
	s.store.SetConfig(cfgServerName, serverName)
	s.store.SetConfig(cfgMachineID, id.MachineIdentifier)
	s.render(w, "setup_done", map[string]any{
		"ServerName": serverName,
		"Version":    id.Version,
		"WebhookURL": s.webhookURL(r),
	})
}

// fragmentError renders an inline error fragment for htmx swaps.
func (s *Server) fragmentError(w http.ResponseWriter, _ string, err error) {
	s.log.Error("setup error", "err", err)
	s.render(w, "fragment_error", map[string]any{"Error": err.Error()})
}
