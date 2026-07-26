package web

import (
	"fmt"
	"net/http"
	"os"
)

func (s *Server) handleManage(w http.ResponseWriter, r *http.Request) {
	token, _ := s.store.GetConfig(cfgToken)
	serverURL, _ := s.store.GetConfig(cfgServerURL)
	if token == "" || serverURL == "" {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	serverName, _ := s.store.GetConfig(cfgServerName)
	items, _ := s.store.ListProtectedItems()
	running, msg := s.jobs.status(allJobKey)
	s.render(w, "manage.html", map[string]any{
		"ServerName":     serverName,
		"ProtectedCount": len(items),
		"Running":        running,
		"Message":        msg,
	})
}

// allJobKey is the job key for library-wide operations (reveal all).
const allJobKey = "__all__"

func (s *Server) handleRevealAll(w http.ResponseWriter, r *http.Request) {
	s.jobs.start(allJobKey, func() string {
		revealed, errs := s.revealAllBlocking()
		return fmt.Sprintf("Revealed %d item(s), %d error(s).", revealed, errs)
	})
	s.renderManageStatus(w)
}

func (s *Server) handleManageStatus(w http.ResponseWriter, r *http.Request) {
	s.renderManageStatus(w)
}

func (s *Server) renderManageStatus(w http.ResponseWriter) {
	running, msg := s.jobs.status(allJobKey)
	s.render(w, "manage_status", map[string]any{"Running": running, "Message": msg})
}

// handleDisconnect runs synchronously (a plain form POST, not htmx) so the
// destructive work finishes before the redirect lands on setup.
func (s *Server) handleDisconnect(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.render(w, "error.html", map[string]any{"Error": err.Error()})
		return
	}
	revealFirst := r.FormValue("reveal") == "on"
	reset := r.FormValue("mode") == "reset"

	// Reveal while the backups still exist, so the library isn't left modified.
	if revealFirst {
		s.revealAllBlocking()
	}

	s.store.DeleteConfig(cfgToken, cfgServerURL, cfgServerName, cfgMachineID)

	if reset {
		s.store.ResetData()
		for _, dir := range []string{s.cfg.OriginalsDir(), s.cfg.GeneratedDir()} {
			os.RemoveAll(dir)
			os.MkdirAll(dir, 0o755)
		}
	}
	http.Redirect(w, r, "/setup", http.StatusSeeOther)
}
