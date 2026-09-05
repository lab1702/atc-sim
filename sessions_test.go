package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"atc-sim/internal/sim"
)

func TestIndependentBrowserGames(t *testing.T) {
	s := testServer(t)
	ts := httptest.NewServer(s.handler())
	defer ts.Close()
	a, b := testBrowser(t), testBrowser(t)
	initial := browserState(t, a, ts.URL, "GET", "/api/state", "")
	other := browserState(t, b, ts.URL, "GET", "/api/state", "")
	if browserSessionID(t, a, ts.URL) == browserSessionID(t, b, ts.URL) {
		t.Fatal("independent browsers received the same game")
	}
	if !reflect.DeepEqual(initial, other) {
		t.Fatal("new games should start at the same initial state")
	}
	var departure string
	for _, aircraft := range initial.Aircraft {
		if aircraft.Phase == "holdshort" {
			departure = aircraft.ID
			break
		}
	}
	if departure == "" {
		t.Fatal("no departure ready for a clearance")
	}
	command, _ := json.Marshal(sim.Command{AircraftID: departure, Action: "takeoff"})
	cleared := browserState(t, a, ts.URL, "POST", "/api/command", string(command))
	if reflect.DeepEqual(cleared, initial) {
		t.Fatal("clearance did not change the issuing player's game")
	}
	if got := browserState(t, b, ts.URL, "GET", "/api/state", ""); !reflect.DeepEqual(got, initial) {
		t.Fatal("another player's clearance changed this game")
	}
	bState := browserState(t, b, ts.URL, "POST", "/api/control", `{"rate":2,"difficulty":"easy"}`)
	aState := browserState(t, a, ts.URL, "POST", "/api/control", `{"paused":true,"rate":8,"difficulty":"hard"}`)
	if !aState.Paused || aState.Rate != 8 || aState.Difficulty != "hard" {
		t.Fatalf("player A's controls were not applied: %+v", aState)
	}
	if got := browserState(t, b, ts.URL, "GET", "/api/state", ""); !reflect.DeepEqual(got, bState) {
		t.Fatal("pause, speed, or difficulty leaked between games")
	}
	// A second tab and a refresh reuse the browser's cookie jar.
	refreshed := &http.Client{Jar: a.Jar, Timeout: 5 * time.Second}
	if got := browserState(t, refreshed, ts.URL, "GET", "/api/state", ""); !reflect.DeepEqual(got, aState) {
		t.Fatal("refresh lost the existing game")
	}
	reset := browserState(t, refreshed, ts.URL, "POST", "/api/control", `{"reset":true}`)
	if !reflect.DeepEqual(reset, initial) {
		t.Fatal("reset did not restore this player's initial game")
	}
	if got := browserState(t, b, ts.URL, "GET", "/api/state", ""); !reflect.DeepEqual(got, bState) {
		t.Fatal("another player's reset changed this game")
	}
}

func TestSessionCookieBootstrap(t *testing.T) {
	for _, tc := range []struct {
		name, scheme, forwardedProto string
		secure                       bool
	}{
		{"http", "http", "", false},
		{"https", "https", "", true},
		{"forwarded https", "http", "https", true},
		{"forwarded http", "http", "http", false},
		{"TLS cannot be downgraded", "https", "http", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testServer(t)
			h := s.handler()
			req := httptest.NewRequest("GET", tc.scheme+"://example.test/api/state", nil)
			req.Header.Set("X-Forwarded-Proto", tc.forwardedProto)
			req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "attacker-selected-id"})
			r := httptest.NewRecorder()
			h.ServeHTTP(r, req)
			if r.Code != http.StatusOK {
				t.Fatalf("bootstrap: %d: %s", r.Code, r.Body.String())
			}
			var cookie *http.Cookie
			for _, candidate := range r.Result().Cookies() {
				if candidate.Name == sessionCookieName {
					cookie = candidate
				}
			}
			if cookie == nil || cookie.Value == "" || cookie.Value == "attacker-selected-id" {
				t.Fatalf("server must generate a new session ID: %v", cookie)
			}
			if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "" || cookie.Domain != "" {
				t.Fatalf("unexpected cookie scope or protections: %+v", cookie)
			}
			if cookie.Secure != tc.secure {
				t.Fatalf("Secure=%v, want %v", cookie.Secure, tc.secure)
			}
			if r.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("personal state must not be cached")
			}
			// With no explicit Path, browsers scope the cookie to the directory
			// of the visible bootstrap URL, including any reverse-proxy prefix.
			jar, err := cookiejar.New(nil)
			if err != nil {
				t.Fatal(err)
			}
			browserURL := *req.URL
			if tc.secure {
				browserURL.Scheme = "https"
			}
			jar.SetCookies(&browserURL, r.Result().Cookies())
			for _, path := range []string{"/api/state", "/api/control", "/api/events", "/", "/app.js", "/apix/state", "/atc/api/state"} {
				u, err := url.Parse(browserURL.Scheme + "://example.test" + path)
				if err != nil {
					t.Fatal(err)
				}
				if got, want := len(jar.Cookies(u)), strings.HasPrefix(path, "/api/"); (got == 1) != want {
					t.Fatalf("session cookie at %s: got %d, want present=%v", path, got, want)
				}
			}
			reuse := httptest.NewRequest("GET", tc.scheme+"://example.test/api/state", nil)
			reuse.AddCookie(cookie)
			r = httptest.NewRecorder()
			h.ServeHTTP(r, reuse)
			if r.Code != http.StatusOK || len(s.sessions) != 1 || s.sessions[cookie.Value] == nil {
				t.Fatal("an existing session cookie did not reuse its game")
			}
		})
	}
}

func TestGameEndpointsRequireAnEstablishedSession(t *testing.T) {
	s := testServer(t)
	h := s.handler()
	for _, sessionID := range []string{"", "unknown-session"} {
		for _, endpoint := range []struct{ method, path, body string }{
			{"POST", "/api/control", `{"reset":true}`},
			{"POST", "/api/command", `{"aircraftId":"ac-001","action":"takeoff"}`},
			{"GET", "/api/events", ""},
		} {
			r := httptest.NewRecorder()
			req := httptest.NewRequest(endpoint.method, endpoint.path, strings.NewReader(endpoint.body))
			if sessionID != "" {
				req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionID})
			}
			h.ServeHTTP(r, req)
			if r.Code != http.StatusUnauthorized {
				t.Fatalf("%s with cookie %q: got %d", endpoint.path, sessionID, r.Code)
			}
			if len(r.Result().Cookies()) != 0 || len(s.sessions) != 0 {
				t.Fatalf("%s allocated a game before bootstrap", endpoint.path)
			}
		}
	}
}

func TestCrossSiteGameRequestsDoNotAllocateSessions(t *testing.T) {
	s := testServer(t)
	h := s.handler()
	for _, path := range []string{"/api/state", "/api/events"} {
		for _, header := range []struct{ name, value string }{
			{"Origin", "https://foreign.test"},
			{"Sec-Fetch-Site", "cross-site"},
		} {
			r := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "https://example.test"+path, nil)
			req.Header.Set(header.name, header.value)
			h.ServeHTTP(r, req)
			if r.Code != http.StatusForbidden {
				t.Fatalf("%s with %s=%s: %d", path, header.name, header.value, r.Code)
			}
			if len(s.sessions) != 0 || len(r.Result().Cookies()) != 0 {
				t.Fatal("cross-site request allocated a game")
			}
			if r.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("session errors must not be cached")
			}
		}
	}
}

func TestExpectedGameRejectsRequestsAfterBrowserSessionChanges(t *testing.T) {
	s := testServer(t)
	ts := httptest.NewServer(s.handler())
	defer ts.Close()
	a, b := testBrowser(t), testBrowser(t)
	gameID := func(client *http.Client) string {
		response, err := client.Get(ts.URL + "/api/state")
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		id := response.Header.Get("X-Game-ID")
		if response.StatusCode != http.StatusOK || id == "" || id == browserSessionID(t, client, ts.URL) {
			t.Fatal("bootstrap must identify the game without exposing its session cookie")
		}
		return id
	}
	idA, idB := gameID(a), gameID(b)
	if idA == idB || gameID(a) != idA {
		t.Fatal("game identity must be unique and stable across refreshes")
	}
	initialB := browserState(t, b, ts.URL, "GET", "/api/state", "")
	for _, endpoint := range []struct{ method, path, body string }{
		{"POST", "/api/control", `{"paused":true,"rate":8}`},
		{"POST", "/api/command", `{"aircraftId":"ac-001","action":"takeoff"}`},
		{"GET", "/api/events?game=" + idA, ""},
	} {
		req, err := http.NewRequest(endpoint.method, ts.URL+endpoint.path, strings.NewReader(endpoint.body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		if endpoint.method == "POST" {
			req.Header.Set("X-Game-ID", idA)
		}
		// An old tab still expects A, but a competing bootstrap replaced its cookie with B.
		response, err := b.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("stale game identity on %s: %d", endpoint.path, response.StatusCode)
		}
	}
	if got := browserState(t, b, ts.URL, "GET", "/api/state", ""); !reflect.DeepEqual(got, initialB) {
		t.Fatal("a request from the old game changed the replacement game")
	}
	req, err := http.NewRequest("POST", ts.URL+"/api/control", strings.NewReader(`{"paused":true,"rate":4}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Game-ID", idB)
	response, err := b.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("X-Game-ID") != idB {
		t.Fatalf("matching game identity was rejected: %d", response.StatusCode)
	}
	if got := browserState(t, b, ts.URL, "GET", "/api/state", ""); !got.Paused || got.Rate != 4 {
		t.Fatal("matching game identity did not apply the command")
	}
	response, err = b.Get(ts.URL + "/api/events?game=" + idB)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("matching stream identity was rejected: %d", response.StatusCode)
	}
	outsider := testBrowser(t)
	response, err = outsider.Get(ts.URL + "/api/events?game=" + idB)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatal("the public game marker authenticated a browser without its session cookie")
	}
}

func TestExpiredSessionRequiresNewBootstrap(t *testing.T) {
	s := testServer(t)
	s.sessionTimeout = time.Minute
	ts := httptest.NewServer(s.handler())
	defer ts.Close()
	client := testBrowser(t)
	initial := browserState(t, client, ts.URL, "GET", "/api/state", "")
	browserState(t, client, ts.URL, "POST", "/api/control", `{"paused":true,"rate":8}`)
	oldID := browserSessionID(t, client, ts.URL)
	old := browserSession(t, s, client, ts.URL)
	s.mu.Lock()
	old.lastSeen = time.Now().Add(-2 * s.sessionTimeout)
	s.mu.Unlock()
	response, err := client.Post(ts.URL+"/api/control", "application/json", strings.NewReader(`{"reset":true}`))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expired game accepted a command: %d", response.StatusCode)
	}
	fresh := browserState(t, client, ts.URL, "GET", "/api/state", "")
	if browserSessionID(t, client, ts.URL) == oldID || !reflect.DeepEqual(fresh, initial) {
		t.Fatal("expired browser did not receive a fresh game and cookie")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions[oldID] != nil || len(s.sessions) != 1 {
		t.Fatal("expired game remained allocated")
	}
}

func TestSessionCapacityPreservesExistingGamesAndReclaimsIdleGames(t *testing.T) {
	s := testServer(t)
	s.maxSessions = 2
	ts := httptest.NewServer(s.handler())
	defer ts.Close()
	a, b, c := testBrowser(t), testBrowser(t), testBrowser(t)
	browserState(t, a, ts.URL, "GET", "/api/state", "")
	browserState(t, b, ts.URL, "GET", "/api/state", "")
	response, err := c.Get(ts.URL + "/api/state")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("new game at capacity: got %d", response.StatusCode)
	}
	if len(response.Cookies()) != 0 {
		t.Fatal("capacity rejection set a session cookie")
	}
	browserState(t, a, ts.URL, "POST", "/api/control", `{"paused":true}`)
	if !browserState(t, a, ts.URL, "GET", "/api/state", "").Paused {
		t.Fatal("existing game was evicted or unusable at capacity")
	}
	old := browserSession(t, s, b, ts.URL)
	s.mu.Lock()
	old.lastSeen = time.Now().Add(-2 * s.sessionTimeout)
	s.mu.Unlock()
	browserState(t, c, ts.URL, "GET", "/api/state", "")
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.sessions) != 2 {
		t.Fatalf("unexpected session count after reclamation: %d", len(s.sessions))
	}
}

func TestAdvanceOnlyTicksConnectedGamesAndExpiresIdleGames(t *testing.T) {
	s := testServer(t)
	s.sessionTimeout = time.Minute
	ts := httptest.NewServer(s.handler())
	defer ts.Close()
	a, b, c := testBrowser(t), testBrowser(t), testBrowser(t)
	for _, client := range []*http.Client{a, b, c} {
		browserState(t, client, ts.URL, "GET", "/api/state", "")
	}
	browserState(t, a, ts.URL, "POST", "/api/control", `{"rate":2}`)
	browserState(t, c, ts.URL, "POST", "/api/control", `{"paused":true}`)
	active, idle, paused := browserSession(t, s, a, ts.URL), browserSession(t, s, b, ts.URL), browserSession(t, s, c, ts.URL)
	idleID := browserSessionID(t, b, ts.URL)
	now := time.Now()
	s.mu.Lock()
	active.connections = 2 // Two tabs must still advance one game only once.
	active.lastSeen = now.Add(-2 * s.sessionTimeout)
	paused.connections = 1
	paused.lastSeen = now.Add(-2 * s.sessionTimeout)
	idle.lastSeen = now
	s.mu.Unlock()
	s.advance(now)
	for _, tc := range []struct {
		name string
		game *gameSession
		want float64
	}{{"connected", active, .2}, {"disconnected", idle, 0}, {"paused", paused, 0}} {
		tc.game.mu.Lock()
		got := tc.game.sim.Snapshot().Time
		tc.game.mu.Unlock()
		if math.Abs(got-tc.want) > 1e-9 {
			t.Fatalf("%s game time=%v, want %v", tc.name, got, tc.want)
		}
	}
	s.advance(now.Add(2 * s.sessionTimeout))
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions[idleID] != nil || len(s.sessions) != 2 {
		t.Fatal("idle game was not expired, or connected games were expired")
	}
}

func openGameStream(t *testing.T, client *http.Client, baseURL string) (*http.Response, *bufio.Reader) {
	t.Helper()
	response, err := client.Get(baseURL + "/api/events")
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "text/event-stream" {
		response.Body.Close()
		t.Fatalf("event stream: %d, %s", response.StatusCode, response.Header.Get("Content-Type"))
	}
	return response, bufio.NewReader(response.Body)
}

func streamState(t *testing.T, reader *bufio.Reader, matches func(sim.State) bool) sim.State {
	t.Helper()
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var state sim.State
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &state); err != nil {
			t.Fatal(err)
		}
		if matches == nil || matches(state) {
			return state
		}
	}
}

func waitConnections(t *testing.T, s *server, game *gameSession, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		s.mu.Lock()
		got := game.connections
		s.mu.Unlock()
		if got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("connections=%d, want %d", got, want)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestSimultaneousEventStreamsStayInTheirGamesAndReconnect(t *testing.T) {
	s := testServer(t)
	ts := httptest.NewServer(s.handler())
	defer ts.Close()
	a, b := testBrowser(t), testBrowser(t)
	browserState(t, a, ts.URL, "GET", "/api/state", "")
	browserState(t, b, ts.URL, "GET", "/api/state", "")
	gameA, gameB := browserSession(t, s, a, ts.URL), browserSession(t, s, b, ts.URL)
	aResponse, aReader := openGameStream(t, a, ts.URL)
	defer aResponse.Body.Close()
	bResponse, bReader := openGameStream(t, b, ts.URL)
	defer bResponse.Body.Close()
	streamState(t, aReader, nil)
	streamState(t, bReader, nil)
	waitConnections(t, s, gameA, 1)
	waitConnections(t, s, gameB, 1)
	browserState(t, a, ts.URL, "POST", "/api/control", `{"paused":true,"difficulty":"hard"}`)
	s.advance(time.Now())
	stateA := streamState(t, aReader, func(state sim.State) bool { return state.Paused })
	stateB := streamState(t, bReader, func(state sim.State) bool { return state.Time > 0 })
	if stateA.Time != 0 || stateA.Difficulty != "hard" || stateB.Paused || stateB.Difficulty != "normal" {
		t.Fatal("event streams did not receive their own game state")
	}
	aResponse.Body.Close()
	waitConnections(t, s, gameA, 0)
	browserState(t, a, ts.URL, "POST", "/api/control", `{"paused":false}`)
	s.advance(time.Now())
	if got := browserState(t, a, ts.URL, "GET", "/api/state", ""); got.Time != 0 || got.Difficulty != "hard" {
		t.Fatal("disconnected game progressed or was discarded")
	}
	reconnected, reader := openGameStream(t, a, ts.URL)
	defer reconnected.Body.Close()
	if got := streamState(t, reader, nil); got.Time != 0 || got.Difficulty != "hard" {
		t.Fatal("reconnect did not resume the same game")
	}
	secondTab, secondReader := openGameStream(t, a, ts.URL)
	defer secondTab.Body.Close()
	streamState(t, secondReader, nil)
	waitConnections(t, s, gameA, 2)
	s.advance(time.Now())
	if got := streamState(t, reader, func(state sim.State) bool { return state.Time > 0 }); math.Abs(got.Time-.1) > 1e-9 {
		t.Fatalf("multiple tabs advanced a game more than once: %v", got.Time)
	}
	secondTab.Body.Close()
	waitConnections(t, s, gameA, 1)
	reconnected.Body.Close()
	waitConnections(t, s, gameA, 0)
	s.mu.Lock()
	lastSeen := gameA.lastSeen
	s.mu.Unlock()
	if time.Since(lastSeen) > time.Second {
		t.Fatal("disconnect did not restart the idle retention period")
	}
}

func TestConcurrentPlayersWithSimulationLoop(t *testing.T) {
	s := testServer(t)
	ts := httptest.NewServer(s.handler())
	defer ts.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.run(ctx)
	}()
	defer func() { cancel(); <-done }()
	const players = 8
	clients := make([]*http.Client, players)
	for i := range clients {
		clients[i] = testBrowser(t)
		browserState(t, clients[i], ts.URL, "GET", "/api/state", "")
	}
	errors := make(chan error, players)
	var wg sync.WaitGroup
	for i, client := range clients {
		wg.Add(1)
		go func(i int, client *http.Client) {
			defer wg.Done()
			response, err := client.Get(ts.URL + "/api/events")
			if err != nil {
				errors <- err
				return
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusOK {
				errors <- fmt.Errorf("player %d stream: %d", i, response.StatusCode)
				return
			}
			// Wait for the actual server ticker before exercising concurrent controls.
			reader := bufio.NewReader(response.Body)
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					errors <- err
					return
				}
				if !strings.HasPrefix(line, "data: ") {
					continue
				}
				var state sim.State
				if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &state); err != nil {
					errors <- err
					return
				}
				if state.Time > 0 {
					break
				}
			}
			rate := float64(i + 1)
			for j := 0; j < 12; j++ {
				body := fmt.Sprintf(`{"rate":%g,"paused":%t}`, rate, j%2 == 0)
				response, err := client.Post(ts.URL+"/api/control", "application/json", strings.NewReader(body))
				if err != nil {
					errors <- err
					return
				}
				var state sim.State
				err = json.NewDecoder(response.Body).Decode(&state)
				response.Body.Close()
				if err != nil || response.StatusCode != http.StatusOK || state.Rate != rate || state.Paused != (j%2 == 0) {
					errors <- fmt.Errorf("player %d control: status=%d rate=%v paused=%v error=%v", i, response.StatusCode, state.Rate, state.Paused, err)
					return
				}
				response, err = client.Get(ts.URL + "/api/state")
				if err != nil {
					errors <- err
					return
				}
				err = json.NewDecoder(response.Body).Decode(&state)
				io.Copy(io.Discard, response.Body)
				response.Body.Close()
				if err != nil || response.StatusCode != http.StatusOK || state.Rate != rate {
					errors <- fmt.Errorf("player %d state leaked: status=%d rate=%v error=%v", i, response.StatusCode, state.Rate, err)
					return
				}
			}
		}(i, client)
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
}
