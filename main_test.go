package main

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
	return &server{airport: airport, sim: sim.New(airport)}
}

func TestEmbeddedAppAndState(t *testing.T) {
	s := testServer(t)
	for _, path := range []string{"/", "/app.js", "/render.js", "/vendor/three.module.js", "/vendor/three.core.js", "/api/airport", "/api/state"} {
		r := httptest.NewRecorder()
		s.handler().ServeHTTP(r, httptest.NewRequest("GET", path, nil))
		if r.Code != 200 {
			t.Fatalf("%s: %d", path, r.Code)
		}
		if r.Body.Len() == 0 {
			t.Fatalf("empty %s", path)
		}
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
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			h.ServeHTTP(r, req)
			if r.Code != tc.code {
				t.Fatalf("got %d: %s", r.Code, r.Body.String())
			}
		})
	}
	if !s.sim.Snapshot().Paused {
		t.Fatal("rejected request changed pause state")
	}
}

func TestEventStreamSendsSnapshot(t *testing.T) {
	ts := httptest.NewServer(testServer(t).handler())
	defer ts.Close()
	response, err := http.Get(ts.URL + "/api/events")
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
	var id string
	for _, a := range s.sim.Snapshot().Aircraft {
		if a.Phase == "holdshort" {
			id = a.ID
			break
		}
	}
	if id == "" {
		t.Fatal("no ready departure")
	}
	body, _ := json.Marshal(sim.Command{AircraftID: id, Action: "takeoff"})
	r := httptest.NewRecorder()
	s.handler().ServeHTTP(r, httptest.NewRequest("POST", "/api/command", strings.NewReader(string(body))))
	if r.Code != 200 {
		payload, _ := io.ReadAll(r.Result().Body)
		t.Fatalf("clearance failed: %s", payload)
	}
	for i := 0; i < 2500; i++ {
		s.sim.Tick(.1)
	}
	for _, a := range s.sim.Snapshot().Aircraft {
		if a.ID == id && a.Phase != "departure" && a.Phase != "complete" {
			t.Fatalf("departure stuck in %s", a.Phase)
		}
	}
}
