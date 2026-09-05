package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"atc-sim/internal/sim"
)

const (
	sessionCookieName     = "atc_session"
	defaultSessionTimeout = 30 * time.Minute
	defaultMaxSessions    = 256
)

type gameSession struct {
	id  string // Public identity marker, never accepted as a session credential.
	mu  sync.Mutex
	sim *sim.Simulation

	// The server mutex protects lifecycle fields; mu protects the simulation.
	lastSeen    time.Time
	connections int
}

func newServer(airport sim.Airport) *server {
	return &server{
		airport: airport, sessions: make(map[string]*gameSession),
		sessionTimeout: defaultSessionTimeout, maxSessions: defaultMaxSessions,
	}
}

// Only the state bootstrap may create a game. Commands and stream reconnects
// must first identify an existing game, so stale commands cannot reset it.
func (s *server) getSession(w http.ResponseWriter, r *http.Request, create bool) (*gameSession, bool) {
	return s.session(w, r, create, false)
}

func (s *server) session(w http.ResponseWriter, r *http.Request, create, connect bool) (*gameSession, bool) {
	now := time.Now()
	s.mu.Lock()
	s.expireSessions(now)
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		if game := s.sessions[cookie.Value]; game != nil {
			expected := r.Header.Get("X-Game-ID")
			if connect {
				expected = r.URL.Query().Get("game")
			}
			if !create && expected != "" && expected != game.id {
				s.mu.Unlock()
				apiError(w, "Your game changed in another tab. Reconnect before sending instructions.", http.StatusUnauthorized)
				return nil, false
			}
			game.lastSeen = now
			if connect {
				game.connections++
			}
			s.mu.Unlock()
			w.Header().Set("X-Game-ID", game.id)
			return game, true
		}
	}
	if !create {
		s.mu.Unlock()
		apiError(w, "Your game session has ended. Reconnect to start a new game.", http.StatusUnauthorized)
		return nil, false
	}
	if len(s.sessions) >= s.maxSessions {
		s.mu.Unlock()
		w.Header().Set("Retry-After", "60")
		apiError(w, "All game slots are in use. Please try again shortly.", http.StatusServiceUnavailable)
		return nil, false
	}
	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		s.mu.Unlock()
		apiError(w, "A game session could not be created. Please try again.", http.StatusInternalServerError)
		return nil, false
	}
	id := hex.EncodeToString(token[:])
	// A separate, non-reversible marker lets the browser bind its displayed
	// game to commands without exposing the HttpOnly session credential.
	marker := sha256.Sum256(token[:])
	game := &gameSession{id: hex.EncodeToString(marker[:]), sim: sim.New(s.airport), lastSeen: now}
	s.sessions[id] = game
	s.mu.Unlock()
	// Omit Path so the browser scopes the cookie to the external bootstrap
	// directory (/api or, behind a prefix-stripping proxy, /atc/api). The
	// backend cannot infer a stripped public prefix from its request path.
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: id, HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		// Caddy supplies the public scheme when terminating HTTPS. This can
		// only enable Secure; forwarded headers never establish game identity.
		Secure: r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
	})
	w.Header().Set("X-Session-Created", "true")
	w.Header().Set("X-Game-ID", game.id)
	return game, true
}

// expireSessions must be called with the server mutex held. Connected streams
// keep games alive even when the player has paused or hidden the page.
func (s *server) expireSessions(now time.Time) {
	for id, game := range s.sessions {
		if game.connections == 0 && now.Sub(game.lastSeen) >= s.sessionTimeout {
			delete(s.sessions, id)
		}
	}
}

func (s *server) endConnection(game *gameSession, now time.Time) {
	// Serialize the last disconnect with a tick already in progress. A ticker
	// that collected this game earlier must not advance it after this returns.
	game.mu.Lock()
	defer game.mu.Unlock()
	s.mu.Lock()
	game.connections--
	game.lastSeen = now
	s.mu.Unlock()
}

func (s *server) advance(now time.Time) {
	s.mu.Lock()
	s.expireSessions(now)
	active := make([]*gameSession, 0, len(s.sessions))
	for _, game := range s.sessions {
		if game.connections > 0 {
			active = append(active, game)
		}
	}
	s.mu.Unlock()
	for _, game := range active {
		game.mu.Lock()
		s.mu.Lock()
		connected := game.connections > 0
		s.mu.Unlock()
		if connected {
			state := game.sim.Snapshot()
			if !state.Paused {
				game.sim.Tick(0.1 * state.Rate)
			}
		}
		game.mu.Unlock()
	}
}
