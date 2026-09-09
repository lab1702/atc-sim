package main

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"atc-sim/internal/sim"
)

func testServer(t *testing.T) *server {
	t.Helper()
	raw, err := assets.ReadFile("data/dtw.json")
	if err != nil {
		t.Fatal(err)
	}
	var airport sim.Airport
	if err = json.Unmarshal(raw, &airport); err != nil {
		t.Fatal(err)
	}
	return newServer(airport)
}

func testBrowser(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Jar: jar, Timeout: 5 * time.Second}
}

func browserState(t *testing.T, client *http.Client, baseURL, method, path, body string) sim.State {
	t.Helper()
	req, err := http.NewRequest(method, baseURL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("%s %s: %d: %s", method, path, response.StatusCode, payload)
	}
	var state sim.State
	if err := json.NewDecoder(response.Body).Decode(&state); err != nil {
		t.Fatal(err)
	}
	return state
}

func browserSessionID(t *testing.T, client *http.Client, baseURL string) string {
	t.Helper()
	u, err := url.Parse(baseURL + "/api/state")
	if err != nil {
		t.Fatal(err)
	}
	for _, cookie := range client.Jar.Cookies(u) {
		if cookie.Name == sessionCookieName {
			return cookie.Value
		}
	}
	t.Fatal("browser has no game session cookie")
	return ""
}

func browserSession(t *testing.T, s *server, client *http.Client, baseURL string) *gameSession {
	t.Helper()
	id := browserSessionID(t, client, baseURL)
	s.mu.Lock()
	defer s.mu.Unlock()
	game := s.sessions[id]
	if game == nil {
		t.Fatal("browser session not found")
	}
	return game
}

func TestEmbeddedAppAndState(t *testing.T) {
	s := testServer(t)
	for _, path := range []string{"/", "/app.js", "/events.js", "/events-worker.js", "/render.js", "/vendor/three.module.js", "/vendor/three.core.js", "/api/airport"} {
		r := httptest.NewRecorder()
		s.handler().ServeHTTP(r, httptest.NewRequest("GET", path, nil))
		if r.Code != 200 {
			t.Fatalf("%s: %d", path, r.Code)
		}
		if r.Body.Len() == 0 {
			t.Fatalf("empty %s", path)
		}
		if len(r.Result().Cookies()) != 0 {
			t.Fatalf("%s allocated a cookie", path)
		}
	}
	if len(s.sessions) != 0 {
		t.Fatal("shared assets allocated a game")
	}
	r := httptest.NewRecorder()
	s.handler().ServeHTTP(r, httptest.NewRequest("GET", "/api/state", nil))
	var state sim.State
	if err := json.Unmarshal(r.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if len(state.Aircraft) < 3 || len(state.Runways) != 6 {
		t.Fatalf("unexpected starting state: %d flights / %d runways", len(state.Aircraft), len(state.Runways))
	}
}

func TestHTTPCommandsAndValidation(t *testing.T) {
	s := testServer(t)
	h := s.handler()
	bootstrap := httptest.NewRecorder()
	h.ServeHTTP(bootstrap, httptest.NewRequest("GET", "/api/state", nil))
	var cookie *http.Cookie
	for _, candidate := range bootstrap.Result().Cookies() {
		if candidate.Name == sessionCookieName {
			cookie = candidate
		}
	}
	if cookie == nil {
		t.Fatal("bootstrap did not set session cookie")
	}
	tests := []struct {
		name, path, body, origin string
		code                     int
	}{
		{"pause", "/api/control", `{"paused":true}`, "", 200},
		{"invalid rate", "/api/control", `{"rate":-1}`, "", 400},
		{"unknown field", "/api/control", `{"paussed":true}`, "", 400},
		{"extra JSON", "/api/control", `{"paused":true} {"paused":false}`, "", 400},
		{"foreign origin", "/api/control", `{"paused":false}`, "https://example.com", 403},
		{"invalid aircraft", "/api/command", `{"aircraftId":"missing","action":"takeoff"}`, "", 409},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRecorder()
			req := httptest.NewRequest("POST", tc.path, strings.NewReader(tc.body))
			req.Host = "127.0.0.1:8080"
			req.Header.Set("Content-Type", "application/json")
			req.AddCookie(cookie)
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			h.ServeHTTP(r, req)
			if r.Code != tc.code {
				t.Fatalf("got %d: %s", r.Code, r.Body.String())
			}
		})
	}
	if !s.sessions[cookie.Value].sim.Snapshot().Paused {
		t.Fatal("rejected request changed pause state")
	}
}

func TestEventStreamSendsSnapshot(t *testing.T) {
	ts := httptest.NewServer(testServer(t).handler())
	defer ts.Close()
	client := testBrowser(t)
	browserState(t, client, ts.URL, "GET", "/api/state", "")
	response, err := client.Get(ts.URL + "/api/events")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatal("missing event stream content type")
	}
	line, err := bufio.NewReader(response.Body).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	var state sim.State
	if err = json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &state); err != nil {
		t.Fatal(err)
	}
	if len(state.Aircraft) == 0 {
		t.Fatal("stream had no aircraft")
	}
}

func TestClearanceThroughHTTP(t *testing.T) {
	s := testServer(t)
	ts := httptest.NewServer(s.handler())
	defer ts.Close()
	client := testBrowser(t)
	initial := browserState(t, client, ts.URL, "GET", "/api/state", "")
	var id string
	for _, a := range initial.Aircraft {
		if a.Phase == "holdshort" {
			id = a.ID
			break
		}
	}
	if id == "" {
		t.Fatal("no ready departure")
	}
	body, _ := json.Marshal(sim.Command{AircraftID: id, Action: "takeoff"})
	browserState(t, client, ts.URL, "POST", "/api/command", string(body))
	game := browserSession(t, s, client, ts.URL)
	game.mu.Lock()
	defer game.mu.Unlock()
	for i := 0; i < 2500; i++ {
		game.sim.Tick(.1)
	}
	for _, a := range game.sim.Snapshot().Aircraft {
		if a.ID == id && a.Phase != "departure" && a.Phase != "complete" {
			t.Fatalf("departure stuck in %s", a.Phase)
		}
	}
}
