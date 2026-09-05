package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"atc-sim/internal/sim"
)

type proxyMount struct {
	prefix string
	server *server
}

// Match Caddy's redirect, strip_prefix, and reverse_proxy behavior. Each
// mount has its own upstream, while browsers see one shared public origin.
func testPrefixProxy(t *testing.T, mounts ...proxyMount) *httptest.Server {
	t.Helper()
	type route struct {
		prefix  string
		handler http.Handler
	}
	routes := make([]route, 0, len(mounts))
	for _, mount := range mounts {
		upstream := httptest.NewServer(mount.server.handler())
		t.Cleanup(upstream.Close)
		target, err := url.Parse(upstream.URL)
		if err != nil {
			t.Fatal(err)
		}
		proxy := httputil.NewSingleHostReverseProxy(target)
		director := proxy.Director
		proxy.Director = func(r *http.Request) {
			director(r)
			// The default Director preserves the original request Host, as
			// Caddy does for an HTTP upstream.
			r.Header.Set("X-Forwarded-Proto", "http")
		}
		routes = append(routes, route{mount.prefix, http.StripPrefix(mount.prefix, proxy)})
	}
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, route := range routes {
			if !strings.HasPrefix(r.URL.Path, route.prefix) {
				continue
			}
			if route.prefix != "" && r.URL.Path == route.prefix {
				http.Redirect(w, r, route.prefix+"/", http.StatusMovedPermanently)
				return
			}
			route.handler.ServeHTTP(w, r)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(proxy.Close)
	return proxy
}

func TestPrefixProxyServesAppAssets(t *testing.T) {
	s := testServer(t)
	proxy := testPrefixProxy(t, proxyMount{"/atc", s})
	client := testBrowser(t)
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Get(proxy.URL + "/atc")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusMovedPermanently || response.Header.Get("Location") != "/atc/" {
		t.Fatalf("mount redirect: %d, %q", response.StatusCode, response.Header.Get("Location"))
	}
	response, err = client.Get(proxy.URL + "/atc/")
	if err != nil {
		t.Fatal(err)
	}
	html, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("app HTML: %d, %v", response.StatusCode, err)
	}
	documentURL := response.Request.URL
	refs := regexp.MustCompile(`(?:href|src)="([^"]+)"`).FindAllStringSubmatch(string(html), -1)
	var assetPaths []string
	for _, ref := range refs {
		reference, err := url.Parse(ref[1])
		if err != nil {
			t.Fatal(err)
		}
		resolved := documentURL.ResolveReference(reference)
		if resolved.Host != documentURL.Host {
			continue
		}
		if !strings.HasPrefix(resolved.Path, "/atc/") {
			t.Fatalf("HTML reference %q escapes the mount: %s", ref[1], resolved.Path)
		}
		assetPaths = append(assetPaths, resolved.Path)
	}
	if len(assetPaths) < 3 {
		t.Fatalf("expected favicon, stylesheet, and app references; got %v", assetPaths)
	}
	assetPaths = append(assetPaths, "/atc/render.js", "/atc/vendor/three.module.js", "/atc/vendor/three.core.js", "/atc/api/airport")
	for _, path := range assetPaths {
		response, err := client.Get(proxy.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		size, err := io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if response.StatusCode != http.StatusOK || err != nil || size == 0 {
			t.Fatalf("%s: status=%d bytes=%d error=%v", path, response.StatusCode, size, err)
		}
		if len(response.Cookies()) != 0 {
			t.Fatalf("asset %s allocated a session", path)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.sessions) != 0 {
		t.Fatal("loading the prefixed app allocated a game before bootstrap")
	}
}

func proxyControl(t *testing.T, client *http.Client, proxyURL, prefix, body, origin string, wantStatus int) sim.State {
	t.Helper()
	req, err := http.NewRequest("POST", proxyURL+prefix+"/api/control", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", origin)
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != wantStatus {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("proxied control: %d, want %d: %s", response.StatusCode, wantStatus, payload)
	}
	if response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("proxied game response must not be cached")
	}
	var state sim.State
	if wantStatus == http.StatusOK {
		if err := json.NewDecoder(response.Body).Decode(&state); err != nil {
			t.Fatal(err)
		}
	}
	return state
}

func TestPrefixProxyScopesGamesAndPreservesOriginChecks(t *testing.T) {
	mounts := []proxyMount{{"/atc", testServer(t)}, {"/other", testServer(t)}, {"", testServer(t)}}
	proxy := testPrefixProxy(t, mounts...)
	client := testBrowser(t)
	ids := make(map[string]string)
	var initial sim.State
	for _, mount := range mounts {
		baseURL := proxy.URL + mount.prefix
		initial = browserState(t, client, baseURL, "GET", "/api/state", "")
		ids[mount.prefix] = browserSessionID(t, client, baseURL)
		for _, other := range mounts {
			if other.prefix != mount.prefix && ids[other.prefix] == ids[mount.prefix] {
				t.Fatal("separate mounts reused a session ID")
			}
		}
	}
	// A browser sends exactly the session for the externally visible API
	// directory, even when the root app and sibling apps share its origin.
	for _, mount := range mounts {
		for _, path := range []string{"/api/state", "/api/control", "/api/events", "/", "/app.js", "/apix/state"} {
			u, err := url.Parse(proxy.URL + mount.prefix + path)
			if err != nil {
				t.Fatal(err)
			}
			cookies := client.Jar.Cookies(u)
			if strings.HasPrefix(path, "/api/") {
				if len(cookies) != 1 || cookies[0].Name != sessionCookieName || cookies[0].Value != ids[mount.prefix] {
					t.Fatalf("wrong cookie scope at %s: %v", u.Path, cookies)
				}
			} else if len(cookies) != 0 {
				t.Fatalf("game cookie leaked to %s", u.Path)
			}
		}
	}
	changed := proxyControl(t, client, proxy.URL, "/atc", `{"paused":true,"rate":8,"difficulty":"hard"}`, proxy.URL, http.StatusOK)
	if !changed.Paused || changed.Rate != 8 || changed.Difficulty != "hard" {
		t.Fatal("same-origin controls did not reach the prefixed game")
	}
	proxyControl(t, client, proxy.URL, "/atc", `{"paused":false}`, "https://foreign.test", http.StatusForbidden)
	refreshed := &http.Client{Jar: client.Jar, Timeout: 5 * time.Second}
	for _, mount := range mounts {
		baseURL := proxy.URL + mount.prefix
		want := initial
		if mount.prefix == "/atc" {
			want = changed
		}
		if got := browserState(t, refreshed, baseURL, "GET", "/api/state", ""); !reflect.DeepEqual(got, want) {
			t.Fatalf("refresh lost the game or another mount changed it: %q", mount.prefix)
		}
		if browserSessionID(t, refreshed, baseURL) != ids[mount.prefix] {
			t.Fatalf("refresh replaced the session at %q", mount.prefix)
		}
		mount.server.mu.Lock()
		count := len(mount.server.sessions)
		mount.server.mu.Unlock()
		if count != 1 {
			t.Fatalf("mount %q allocated %d games for one browser", mount.prefix, count)
		}
	}
	otherPlayer := testBrowser(t)
	if got := browserState(t, otherPlayer, proxy.URL+"/atc", "GET", "/api/state", ""); !reflect.DeepEqual(got, initial) {
		t.Fatal("a second player inherited the first player's game")
	}
	if browserSessionID(t, otherPlayer, proxy.URL+"/atc") == ids["/atc"] {
		t.Fatal("independent players share a prefixed game")
	}
}

func TestPrefixProxyStreamsLiveGameUpdates(t *testing.T) {
	s := testServer(t)
	proxy := testPrefixProxy(t, proxyMount{"/atc", s})
	client := testBrowser(t)
	baseURL := proxy.URL + "/atc"
	browserState(t, client, baseURL, "GET", "/api/state", "")
	response, reader := openGameStream(t, client, baseURL)
	defer response.Body.Close()
	if response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("proxied event stream must not be cached")
	}
	initial := streamState(t, reader, nil)
	game := browserSession(t, s, client, baseURL)
	waitConnections(t, s, game, 1)
	s.advance(time.Now())
	advanced := streamState(t, reader, func(state sim.State) bool { return state.Time > initial.Time })
	if advanced.Time <= initial.Time {
		t.Fatal("proxy did not flush live simulation updates")
	}
	proxyControl(t, client, proxy.URL, "/atc", `{"paused":true}`, proxy.URL, http.StatusOK)
	streamState(t, reader, func(state sim.State) bool { return state.Paused })
	response.Body.Close()
	waitConnections(t, s, game, 0)
	reconnected, reader := openGameStream(t, client, baseURL)
	defer reconnected.Body.Close()
	if got := streamState(t, reader, nil); !got.Paused || got.Time != advanced.Time {
		t.Fatal("reconnecting through the proxy lost the game")
	}
}
