package main

import "flag"

type Config struct {
	Addr          string
	DBPath        string
	Host          string
	TrustProxy    bool
	AdminCIDR     string // CIDR allowlist for admin endpoints, empty = unrestricted
	TLSSkipVerify bool   // skip TLS verification for app detection/completion fetches
}

func parseFlags() *Config {
	cfg := &Config{}
	flag.StringVar(&cfg.Addr, "addr", ":80", "listen address")
	flag.StringVar(&cfg.DBPath, "db", "golinks.db", "path to SQLite database")
	flag.StringVar(&cfg.Host, "host", "", "hostname for OpenSearch URLs (default: from request)")
	flag.BoolVar(&cfg.TrustProxy, "trust-proxy", false, "trust X-Forwarded-For/X-Real-IP headers")
	flag.StringVar(&cfg.AdminCIDR, "admin-cidr", "", "CIDR allowlist for admin access (e.g. 192.168.1.0/24,10.0.0.0/8). Empty = unrestricted")
	flag.BoolVar(&cfg.TLSSkipVerify, "tls-skip-verify", false, "skip TLS certificate verification for app detection/completion fetches")
	flag.Parse()
	return cfg
}
