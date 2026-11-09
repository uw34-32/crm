package app

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func noContent(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }

func TestSecurityHeaders(t *testing.T) {
	h := securityHeaders(nil)(http.HandlerFunc(noContent))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	for header, want := range map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Referrer-Policy":         "no-referrer",
		"Cache-Control":           "no-store",
		"Content-Security-Policy": "default-src 'none'; frame-ancestors 'none'",
	} {
		if got := rec.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
}

func TestHSTSOnlyOverTrustedHTTPS(t *testing.T) {
	proxies := proxyTrust{netip.MustParsePrefix("10.0.0.0/8")}
	tests := []struct {
		name   string
		trust  proxyTrust
		remote string
		proto  string
		want   bool
	}{
		{"plain HTTP", nil, "192.0.2.1:1", "", false},
		{"forwarded proto from an untrusted peer", nil, "192.0.2.1:1", "https", false},
		{"forwarded proto from a trusted proxy", proxies, "10.0.0.1:1", "https", true},
		{"trusted proxy forwarding HTTP", proxies, "10.0.0.1:1", "http", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := securityHeaders(tt.trust)(http.HandlerFunc(noContent))
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tt.remote
			if tt.proto != "" {
				r.Header.Set("X-Forwarded-Proto", tt.proto)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, r)
			if got := rec.Header().Get("Strict-Transport-Security") != ""; got != tt.want {
				t.Errorf("HSTS present = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCORS(t *testing.T) {
	const allowed = "http://localhost:5173"
	h := cors([]string{allowed, "tauri://localhost"})(http.HandlerFunc(noContent))

	preflight := func(origin string) *http.Request {
		r := httptest.NewRequest(http.MethodOptions, "/auth/login", nil)
		r.Header.Set("Origin", origin)
		r.Header.Set("Access-Control-Request-Method", http.MethodPost)
		return r
	}
	get := func(origin string) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		return r
	}

	tests := []struct {
		name       string
		req        *http.Request
		wantOrigin string
		wantMethod string
	}{
		{"preflight from an allowed origin", preflight(allowed), allowed, "POST"},
		{"preflight from the desktop origin", preflight("tauri://localhost"), "tauri://localhost", "POST"},
		{"preflight from another origin", preflight("https://evil.example"), "", ""},
		{"origin differing by port", get("http://localhost:5174"), "", ""},
		{"origin differing by scheme", get("https://localhost:5173"), "", ""},
		{"request from an allowed origin", get(allowed), allowed, ""},
		{"request from another origin", get("https://evil.example"), "", ""},
		{"null origin", get("null"), "", ""},
		{"native client without an origin", get(""), "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, tt.req)
			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != tt.wantOrigin {
				t.Errorf("Allow-Origin = %q, want %q", got, tt.wantOrigin)
			}
			if got := rec.Header().Get("Vary"); got != "Origin" {
				t.Errorf("Vary = %q, want Origin", got)
			}
			if tt.wantMethod != "" && !strings.Contains(rec.Header().Get("Access-Control-Allow-Methods"), tt.wantMethod) {
				t.Errorf("Allow-Methods = %q, want %s", rec.Header().Get("Access-Control-Allow-Methods"), tt.wantMethod)
			}
			// Tokens travel in the Authorization header, so cookies are never needed.
			if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
				t.Errorf("Allow-Credentials = %q, want unset", got)
			}
		})
	}
}

func TestCORSAnswersPreflightWithoutCallingHandler(t *testing.T) {
	called := false
	h := cors([]string{"http://localhost:5173"})(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	r := httptest.NewRequest(http.MethodOptions, "/", nil)
	r.Header.Set("Origin", "http://localhost:5173")
	r.Header.Set("Access-Control-Request-Method", http.MethodPost)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if called {
		t.Error("preflight reached the handler")
	}
	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}
}

func TestClientAddr(t *testing.T) {
	proxies := proxyTrust{netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("2001:db8:ffff::/48")}
	tests := []struct {
		name   string
		trust  proxyTrust
		remote string
		xff    []string
		want   string
	}{
		{"peer without proxies", nil, "203.0.113.9:1234", nil, "203.0.113.9"},
		{"forwarded header ignored unless configured", nil, "203.0.113.9:1234", []string{"198.51.100.1"}, "203.0.113.9"},
		{"forwarded header ignored from an untrusted peer", proxies, "203.0.113.9:1234", []string{"198.51.100.1"}, "203.0.113.9"},
		{"trusted proxy names the client", proxies, "10.0.0.1:1234", []string{"198.51.100.1"}, "198.51.100.1"},
		{"spoofed left entries ignored", proxies, "10.0.0.1:1234", []string{"1.1.1.1, 198.51.100.1, 10.0.0.2"}, "198.51.100.1"},
		{"split header lines", proxies, "10.0.0.1:1234", []string{"1.1.1.1", "198.51.100.1"}, "198.51.100.1"},
		{"garbage stops the walk at the last trusted hop", proxies, "10.0.0.1:1234", []string{"nonsense, 10.0.0.2"}, "10.0.0.2"},
		{"trusted proxy without a header", proxies, "10.0.0.1:1234", nil, "10.0.0.1"},
		{"IPv6 peer", nil, "[2001:db8::1]:1234", nil, "2001:db8::1"},
		{"IPv6 zone dropped", nil, "[fe80::1%eth0]:1234", nil, "fe80::1"},
		{"IPv4-mapped IPv6 peer", nil, "[::ffff:203.0.113.9]:1234", nil, "203.0.113.9"},
		{"IPv4-mapped trusted proxy", proxies, "[::ffff:10.0.0.1]:1234", []string{"198.51.100.1"}, "198.51.100.1"},
		{"IPv6 trusted proxy", proxies, "[2001:db8:ffff::1]:1234", []string{"2001:db8::5"}, "2001:db8::5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tt.remote
			for _, v := range tt.xff {
				r.Header.Add("X-Forwarded-For", v)
			}
			if got := tt.trust.clientAddr(r); got != netip.MustParseAddr(tt.want) {
				t.Errorf("clientAddr = %v, want %s", got, tt.want)
			}
		})
	}
}

func TestLimiterKey(t *testing.T) {
	addr := netip.MustParseAddr
	if limiterKey(addr("2001:db8:1:2::1")) != limiterKey(addr("2001:db8:1:2:ffff::9")) {
		t.Error("addresses in one IPv6 /64 must share a key")
	}
	if limiterKey(addr("2001:db8:1:2::1")) == limiterKey(addr("2001:db8:1:3::1")) {
		t.Error("addresses in different IPv6 /64s must not share a key")
	}
	if limiterKey(addr("192.0.2.1")) == limiterKey(addr("192.0.2.2")) {
		t.Error("IPv4 addresses must not share a key")
	}
	if limiterKey(addr("::ffff:192.0.2.1")) != limiterKey(addr("192.0.2.1")) {
		t.Error("an IPv4-mapped address must share the IPv4 key")
	}
}

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func newFakeClock() *fakeClock { return &fakeClock{now: time.Unix(1_700_000_000, 0)} }

func ipKey(s string) netip.Prefix { return limiterKey(netip.MustParseAddr(s)) }

func TestLimiterTripsAndRefills(t *testing.T) {
	clock := newFakeClock()
	l := newIPLimiter(3, 100, clock.Now)
	key := ipKey("192.0.2.1")

	for i := range 3 {
		if ok, _ := l.allow(key); !ok {
			t.Fatalf("request %d refused inside the limit", i+1)
		}
	}
	ok, retry := l.allow(key)
	if ok {
		t.Fatal("request over the limit allowed")
	}
	if retry <= 0 || retry > 20*time.Second {
		t.Errorf("retry = %v, want the 20 seconds one token takes", retry)
	}
	if ok, _ := l.allow(ipKey("192.0.2.2")); !ok {
		t.Error("another address was limited")
	}

	clock.Advance(20 * time.Second)
	if ok, _ := l.allow(key); !ok {
		t.Error("no token after the refill interval")
	}
	if ok, _ := l.allow(key); ok {
		t.Error("only one token should have refilled")
	}
}

func TestLimiterKeepsLimitsWhenFull(t *testing.T) {
	clock := newFakeClock()
	l := newIPLimiter(2, 2, clock.Now)
	a, b := ipKey("192.0.2.1"), ipKey("192.0.2.2")

	for _, key := range []netip.Prefix{a, a, b} {
		if ok, _ := l.allow(key); !ok {
			t.Fatal("request refused inside the limit")
		}
	}
	// The map is full. New addresses must not evict or reset anyone.
	for i := range 50 {
		l.allow(ipKey("198.51.100." + strconv.Itoa(i)))
	}
	if ok, _ := l.allow(a); ok {
		t.Error("a full map reset an exhausted address")
	}
	// New addresses share one bucket instead of going unlimited.
	if ok, _ := l.allow(ipKey("203.0.113.1")); ok {
		t.Error("addresses beyond capacity were not limited")
	}
	if got := l.size(); got != 2 {
		t.Errorf("size = %d, want the map capped at 2", got)
	}
}

func TestLimiterCleansStaleEntries(t *testing.T) {
	clock := newFakeClock()
	l := newIPLimiter(60, 2, clock.Now)
	l.allow(ipKey("192.0.2.1"))
	l.allow(ipKey("192.0.2.2"))
	if got := l.size(); got != 2 {
		t.Fatalf("size = %d, want 2", got)
	}

	clock.Advance(2 * time.Minute)
	if ok, _ := l.allow(ipKey("192.0.2.3")); !ok {
		t.Error("a fresh address was refused after the map should have emptied")
	}
	if got := l.size(); got != 1 {
		t.Errorf("size = %d, want only the fresh address", got)
	}
}

// An entry with room to refill is still state, and dropping it would hand the
// address a fresh burst.
func TestLimiterKeepsEntriesStillRefilling(t *testing.T) {
	clock := newFakeClock()
	l := newIPLimiter(1, 10, clock.Now)
	key := ipKey("192.0.2.1")
	l.allow(key)
	clock.Advance(59 * time.Second)
	l.sweep(clock.Now())
	if ok, _ := l.allow(key); ok {
		t.Error("a sweep forgot an exhausted address")
	}
}

func TestLimiterIsSafeForConcurrentUse(t *testing.T) {
	l := newIPLimiter(1000, 10, time.Now)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			for j := range 200 {
				l.allow(ipKey("192.0.2." + strconv.Itoa((i*j)%250)))
			}
		})
	}
	wg.Wait()
}

func TestRateLimitMiddleware(t *testing.T) {
	h := rateLimit(newIPLimiter(2, 100, newFakeClock().Now), nil)(http.HandlerFunc(noContent))
	send := func(remote, spoof string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/", nil)
		r.RemoteAddr = remote
		if spoof != "" {
			r.Header.Set("X-Forwarded-For", spoof)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec
	}
	send("203.0.113.9:1", "")
	send("203.0.113.9:2", "")

	// Rotating the forwarded header must not buy a new bucket.
	rec := send("203.0.113.9:3", "198.51.100.77")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("Retry-After missing")
	}
	if got := send("203.0.113.10:1", "").Code; got != http.StatusNoContent {
		t.Errorf("another address got %d, want 204", got)
	}
}
