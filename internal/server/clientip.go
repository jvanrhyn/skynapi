package server

import (
	"net"
	"net/http"
	"strings"
)

// clientIP rewrites r.RemoteAddr to the client address reported by the trusted
// reverse proxies in front of this server, so logging and rate limiting key on
// the real caller rather than the proxy.
//
// trustedProxyCount is how many proxies sit between the internet and this
// process. Each one appends the address it saw to X-Forwarded-For, so the entry
// trustedProxyCount from the right is the last value a trusted hop wrote —
// everything to its left was supplied by the client and is forgeable. A count
// of 0 trusts no proxy and leaves the TCP peer address in place.
//
// True-Client-IP and X-Real-IP are deliberately ignored. They carry a single
// value with no hop history, so a forged header is indistinguishable from a
// genuine one and any client could mint itself a fresh rate-limit bucket per
// request.
func clientIP(trustedProxyCount int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if ip := forwardedFor(r, trustedProxyCount); ip != "" {
				r.RemoteAddr = ip
			}
			next.ServeHTTP(w, r)
		})
	}
}

// forwardedFor returns the client address contributed by the outermost trusted
// proxy, or "" when there is no trustworthy value to use.
func forwardedFor(r *http.Request, trustedProxyCount int) string {
	if trustedProxyCount <= 0 {
		return ""
	}

	// A client may send several X-Forwarded-For headers, and each may carry a
	// comma-separated list; flatten both into one ordered list of hops.
	var hops []string
	for _, header := range r.Header.Values("X-Forwarded-For") {
		for _, hop := range strings.Split(header, ",") {
			if hop = strings.TrimSpace(hop); hop != "" {
				hops = append(hops, hop)
			}
		}
	}
	if len(hops) == 0 {
		return ""
	}

	idx := len(hops) - trustedProxyCount
	if idx < 0 {
		// Fewer hops than configured proxies: the chain is shorter than
		// expected, so the leftmost entry is the closest to trustworthy.
		idx = 0
	}
	if net.ParseIP(hops[idx]) == nil {
		return ""
	}
	return hops[idx]
}
