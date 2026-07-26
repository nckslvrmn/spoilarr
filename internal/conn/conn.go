// Package conn builds a configured Plex client from persisted settings. It is
// the single source of truth for the config keys that hold the Plex connection,
// shared by the web layer and the background poller.
package conn

import (
	"crypto/rand"
	"fmt"

	"github.com/nckslvrmn/spoilarr/internal/plex"
	"github.com/nckslvrmn/spoilarr/internal/store"
)

// Config keys for the Plex connection.
const (
	KeyClientID   = "client_id"
	KeyToken      = "plex_token"
	KeyServerURL  = "server_url"
	KeyServerName = "server_name"
	KeyMachineID  = "machine_id"
)

// ClientID returns this install's stable Plex client identifier, generating and
// persisting one on first use.
func ClientID(st *store.Store) (string, error) {
	id, err := st.GetConfig(KeyClientID)
	if err != nil {
		return "", err
	}
	if id != "" {
		return id, nil
	}
	id = randomID()
	if err := st.SetConfig(KeyClientID, id); err != nil {
		return "", err
	}
	return id, nil
}

// Client builds a Plex client from stored config (client id, token, server URL).
// Token/ServerURL may be empty if setup is incomplete; callers should check.
func Client(st *store.Store) (*plex.Client, error) {
	id, err := ClientID(st)
	if err != nil {
		return nil, err
	}
	token, err := st.GetConfig(KeyToken)
	if err != nil {
		return nil, err
	}
	serverURL, err := st.GetConfig(KeyServerURL)
	if err != nil {
		return nil, err
	}
	return plex.New(id, token, serverURL), nil
}

// Configured reports whether setup is complete enough to talk to a server.
func Configured(c *plex.Client) bool {
	return c != nil && c.Token != "" && c.ServerURL != ""
}

func randomID() string {
	var b [16]byte
	rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
