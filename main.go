package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"time"

	"atc-sim/internal/sim"
)

//go:embed web data/dtw.json
var assets embed.FS

type server struct {
	mu      sync.Mutex
	sim     *sim.Simulation
	airport sim.Airport
	webFS   fs.FS
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "HTTP listen address")
	dev := flag.Bool("dev", false, "Serve UI assets from ./web for development")
	flag.Parse()
	raw, err := assets.ReadFile("data/dtw.json")
	if err != nil {
		log.Fatal(err)
	}
	var airport sim.Airport
	if err := json.Unmarshal(raw, &airport); err != nil {
		log.Fatal(err)
	}
	s := &server{airport: airport, sim: sim.New(airport)}
	if *dev {
		s.webFS = os.DirFS("web")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go s.run(ctx)
	h := &http.Server{Addr: *addr, Handler: s.handler(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = h.Shutdown(shutdown)
	}()
	log.Printf("DTW tower simulator: http://%s", *addr)
	if err := h.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func (s *server) run(ctx context.Context) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			state := s.sim.Snapshot()
			if !state.Paused {
				s.sim.Tick(0.1 * state.Rate)
			}
			s.mu.Unlock()
		}
	}
}

func (s *server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/airport", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, s.airport) })
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		writeJSON(w, s.sim.Snapshot())
	})
	mux.HandleFunc("GET /api/events", s.events)
	mux.HandleFunc("POST /api/command", func(w http.ResponseWriter, r *http.Request) {
		var cmd sim.Command
		if !decode(w, r, &cmd) {
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if err := s.sim.Command(cmd); err != nil {
			apiError(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, s.sim.Snapshot())
	})
	mux.HandleFunc("POST /api/control", func(w http.ResponseWriter, r *http.Request) {
		var ctrl sim.Control
		if !decode(w, r, &ctrl) {
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if err := s.sim.Control(ctrl); err != nil {
			apiError(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, s.sim.Snapshot())
	})
	web, _ := fs.Sub(assets, "web")
	if s.webFS != nil {
		web = s.webFS
	}
	mux.Handle("GET /", http.FileServer(http.FS(web)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'self'")
		if r.Method == "POST" {
			origin := r.Header.Get("Origin")
			if origin != "" {
				parsed, err := url.Parse(origin)
				if err != nil || parsed.Host != r.Host {
					apiError(w, "Cross-origin commands are disabled", http.StatusForbidden)
					return
				}
			}
			if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
				apiError(w, "Cross-site commands are disabled", http.StatusForbidden)
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	defer r.Body.Close()
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		apiError(w, "Invalid command: "+err.Error(), http.StatusBadRequest)
		return false
	}
	if err := d.Decode(new(any)); err != io.EOF {
		apiError(w, "Expected one JSON object", http.StatusBadRequest)
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}
func apiError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func (s *server) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		s.mu.Lock()
		data, err := json.Marshal(s.sim.Snapshot())
		s.mu.Unlock()
		if err != nil {
			return
		}
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(5 * time.Second))
		if _, err = fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			return
		}
		flusher.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}
