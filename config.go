package main

import "flag"

type Config struct {
	Addr       string
	DBPath     string
	Host       string
	TrustProxy bool
}

func parseFlags() *Config {
	cfg := &Config{}
	flag.StringVar(&cfg.Addr, "addr", ":80", "listen address")
	flag.StringVar(&cfg.DBPath, "db", "golinks.db", "path to SQLite database")
	flag.StringVar(&cfg.Host, "host", "", "hostname for OpenSearch URLs (default: from request)")
	flag.BoolVar(&cfg.TrustProxy, "trust-proxy", false, "trust X-Forwarded-For/X-Real-IP headers")
	flag.Parse()
	return cfg
}
