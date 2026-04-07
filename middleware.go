package main

import (
	"context"
	"net"
	"net/http"
	"strings"
)

type ctxKey string

const clientIPKey ctxKey = "clientIP"

func ipMiddleware(trustProxy bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := extractClientIP(r, trustProxy)
		ctx := context.WithValue(r.Context(), clientIPKey, ip)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func getClientIP(r *http.Request) net.IP {
	if ip, ok := r.Context().Value(clientIPKey).(net.IP); ok {
		return ip
	}
	return net.ParseIP("127.0.0.1")
}

func extractClientIP(r *http.Request, trustProxy bool) net.IP {
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if ip := net.ParseIP(strings.TrimSpace(parts[0])); ip != nil {
				return ip
			}
		}
		if xri := r.Header.Get("X-Real-IP"); xri != "" {
			if ip := net.ParseIP(strings.TrimSpace(xri)); ip != nil {
				return ip
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return net.ParseIP(r.RemoteAddr)
	}
	return net.ParseIP(host)
}

func isLinkVisible(link *Link, clientIP net.IP) bool {
	cidrs := link.CIDRList()
	if len(cidrs) == 0 {
		return true
	}
	for _, cidr := range cidrs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(clientIP) {
			return true
		}
	}
	return false
}

func filterVisibleLinks(links []*Link, clientIP net.IP) []*Link {
	var visible []*Link
	for _, link := range links {
		if isLinkVisible(link, clientIP) {
			visible = append(visible, link)
		}
	}
	return visible
}
