package app

import (
	"math"
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// proxyTrust lists the networks whose forwarding headers are believed. It is
// empty unless configured, and then every forwarding header is ignored.
type proxyTrust []netip.Prefix

func (p proxyTrust) contains(addr netip.Addr) bool {
	return addr.IsValid() && slices.ContainsFunc(p, func(prefix netip.Prefix) bool { return prefix.Contains(addr) })
}

// normalizeAddr makes equal addresses compare equal: IPv4-mapped IPv6 becomes
// IPv4, and zones are dropped.
func normalizeAddr(addr netip.Addr) netip.Addr {
	return addr.Unmap().WithZone("")
}

// peerAddr returns the address of the connection's other end, or the zero Addr
// if it cannot be parsed.
func peerAddr(r *http.Request) netip.Addr {
	addrPort, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return netip.Addr{}
	}
	return normalizeAddr(addrPort.Addr())
}

// clientAddr returns the address to attribute a request to. Forwarded entries
// are walked from the right, because each trusted proxy appends the peer it saw
// and anything further left is client-supplied. The walk stops at the first
// entry that is untrusted or unparsable.
func (p proxyTrust) clientAddr(r *http.Request) netip.Addr {
	client := peerAddr(r)
	if !p.contains(client) {
		return client
	}
	entries := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for _, entry := range slices.Backward(entries) {
		addr, err := netip.ParseAddr(strings.TrimSpace(entry))
		if err != nil {
			return client
		}
		client = normalizeAddr(addr)
		if !p.contains(client) {
			return client
		}
	}
	return client
}

func (p proxyTrust) overTLS(r *http.Request) bool {
	return r.TLS != nil || (p.contains(peerAddr(r)) && r.Header.Get("X-Forwarded-Proto") == "https")
}

func securityHeaders(trust proxyTrust) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "no-referrer")
			h.Set("Cache-Control", "no-store")
			h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
			if trust.overTLS(r) {
				h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}

// cors allows exactly the listed origins. Credentials stay disallowed because
// tokens travel in the Authorization header, not in cookies. Clients that send
// no Origin, such as native apps, are unaffected.
func cors(origins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(origins))
	for _, origin := range origins {
		allowed[origin] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Add("Vary", "Origin")
			origin := r.Header.Get("Origin")
			_, ok := allowed[origin]
			if ok {
				h.Set("Access-Control-Allow-Origin", origin)
			}
			if r.Method != http.MethodOptions || origin == "" || r.Header.Get("Access-Control-Request-Method") == "" {
				next.ServeHTTP(w, r)
				return
			}
			if ok {
				h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE")
				h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
				h.Set("Access-Control-Max-Age", "600")
			}
			w.WriteHeader(http.StatusNoContent)
		})
	}
}

const (
	limiterMaxEntries    = 100_000
	limiterSweepInterval = time.Minute
	// An IPv6 customer usually controls a whole /64, so limiting single addresses
	// would let one client spend a nearly endless supply of them.
	limiterIPv6PrefixBits = 64
)

// limiterKey is the unit one client is limited by: an IPv4 address or an IPv6
// /64. An invalid address maps to one shared key, so it stays limited.
func limiterKey(addr netip.Addr) netip.Prefix {
	if !addr.IsValid() {
		return netip.Prefix{}
	}
	addr = normalizeAddr(addr)
	bits := addr.BitLen()
	if addr.Is6() {
		bits = limiterIPv6PrefixBits
	}
	prefix, err := addr.Prefix(bits)
	if err != nil {
		return netip.Prefix{}
	}
	return prefix
}

type bucket struct {
	tokens  float64
	updated time.Time
}

// ipLimiter is a token bucket per client. Holding at most maxEntries, it stays
// bounded under address-rotation floods, and it never evicts live state to make
// room: clients that do not fit share one overflow bucket, so they stay limited
// and nobody's exhausted budget is reset. A bucket that has fully refilled is
// the same as no bucket, so sweeping those loses nothing.
//
// It keeps no goroutine; sweeps run during requests.
type ipLimiter struct {
	mu         sync.Mutex
	buckets    map[netip.Prefix]*bucket
	overflow   bucket
	perMinute  float64
	maxEntries int
	lastSweep  time.Time
	now        func() time.Time
}

func newIPLimiter(perMinute, maxEntries int, now func() time.Time) *ipLimiter {
	start := now()
	return &ipLimiter{
		buckets:    make(map[netip.Prefix]*bucket),
		overflow:   bucket{tokens: float64(perMinute), updated: start},
		perMinute:  float64(perMinute),
		maxEntries: maxEntries,
		lastSweep:  start,
		now:        now,
	}
}

func (l *ipLimiter) refilled(b *bucket, now time.Time) float64 {
	gained := float64(now.Sub(b.updated)) * l.perMinute / float64(time.Minute)
	return min(l.perMinute, b.tokens+gained)
}

// allow spends one token for key. When refused, it returns how long until a
// token is available.
func (l *ipLimiter) allow(key netip.Prefix) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	if now.Sub(l.lastSweep) >= limiterSweepInterval {
		l.sweep(now)
	}

	b, ok := l.buckets[key]
	switch {
	case ok:
	case len(l.buckets) < l.maxEntries:
		b = &bucket{tokens: l.perMinute, updated: now}
		l.buckets[key] = b
	default:
		b = &l.overflow
	}

	b.tokens = l.refilled(b, now)
	b.updated = now
	if b.tokens < 1 {
		return false, time.Duration((1 - b.tokens) * float64(time.Minute) / l.perMinute)
	}
	b.tokens--
	return true, 0
}

// sweep drops buckets that have fully refilled. The caller holds the lock,
// except in tests that use the limiter from one goroutine.
func (l *ipLimiter) sweep(now time.Time) {
	for key, b := range l.buckets {
		if l.refilled(b, now) >= l.perMinute {
			delete(l.buckets, key)
		}
	}
	l.lastSweep = now
}

func (l *ipLimiter) size() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}

func rateLimit(limiter *ipLimiter, trust proxyTrust) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ok, retry := limiter.allow(limiterKey(trust.clientAddr(r)))
			if !ok {
				w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(retry.Seconds()))))
				errResp(w, http.StatusTooManyRequests, "too many requests")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
