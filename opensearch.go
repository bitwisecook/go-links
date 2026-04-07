package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

func (s *server) handleOpenSearch(w http.ResponseWriter, r *http.Request) {
	host := s.cfg.Host
	if host == "" {
		host = r.Host
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if fProto := r.Header.Get("X-Forwarded-Proto"); fProto != "" {
		scheme = fProto
	}

	w.Header().Set("Content-Type", "application/opensearchdescription+xml; charset=utf-8")
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<OpenSearchDescription xmlns="http://a9.com/-/spec/opensearch/1.1/">
  <ShortName>Go Links</ShortName>
  <Description>Go link shortener - type go followed by a keyword</Description>
  <InputEncoding>UTF-8</InputEncoding>
  <OutputEncoding>UTF-8</OutputEncoding>
  <Url type="text/html" method="get" template="%s://%s/{searchTerms}"/>
  <Url type="application/x-suggestions+json" method="get" template="%s://%s/api/suggestions?q={searchTerms}"/>
  <Image height="16" width="16" type="image/svg+xml">%s://%s/static/favicon.svg</Image>
</OpenSearchDescription>`, scheme, host, scheme, host, scheme, host)
}

func (s *server) handleSuggestions(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	clientIP := getClientIP(r)

	suggestions := make([]string, 0)
	descriptions := make([]string, 0)
	urls := make([]string, 0)

	host := s.cfg.Host
	if host == "" {
		host = r.Host
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if fProto := r.Header.Get("X-Forwarded-Proto"); fProto != "" {
		scheme = fProto
	}
	baseURL := scheme + "://" + host

	// Check if query contains a space -> two-level completion
	if idx := strings.IndexByte(query, ' '); idx >= 0 {
		keyword := query[:idx]
		argPrefix := query[idx+1:]

		link, err := s.store.Get(keyword)
		if err == nil && link != nil && isLinkVisible(link, clientIP) {
			completions, err := s.store.SearchCompletions(keyword, argPrefix)
			if err == nil {
				for _, c := range completions {
					suggestions = append(suggestions, keyword+" "+c.Value)
					descriptions = append(descriptions, c.Description)
					urls = append(urls, baseURL+"/"+keyword+"/"+c.Value)
				}
			}
		}
	} else if query != "" {
		// First-level: search link names
		links, err := s.store.SearchLinks(query)
		if err == nil {
			visible := filterVisibleLinks(links, clientIP)
			for _, l := range visible {
				if len(suggestions) >= 10 {
					break
				}
				suggestions = append(suggestions, l.Name)
				desc := l.Description
				if desc == "" {
					desc = l.URL
				}
				descriptions = append(descriptions, desc)
				urls = append(urls, baseURL+"/"+l.Name)
			}
		}
	}

	// OpenSearch Suggestions format: [query, [suggestions], [descriptions], [urls]]
	result := []interface{}{query, suggestions, descriptions, urls}
	w.Header().Set("Content-Type", "application/x-suggestions+json; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(result)
}
