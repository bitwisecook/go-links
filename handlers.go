package main

import (
	"encoding/json"
	"html/template"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	reservedNames = map[string]bool{
		"admin": true, "api": true, "static": true,
		"opensearch.xml": true, "favicon.ico": true,
	}
	validNameRe = regexp.MustCompile(`^[a-z0-9\-]+$`)
)

// isValidURLTemplate checks that a URL uses http or https scheme.
// URLs with {args}/{1} placeholders are normalized before parsing.
func isValidURLTemplate(rawURL string) bool {
	// Replace placeholders so url.Parse can handle it
	normalized := strings.ReplaceAll(rawURL, "{args}", "PLACEHOLDER")
	for i := 1; i <= 9; i++ {
		normalized = strings.ReplaceAll(normalized, "{"+strconv.Itoa(i)+"}", "PLACEHOLDER")
	}
	u, err := url.Parse(normalized)
	if err != nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

type server struct {
	store     *Store
	cfg       *Config
	tmpls     *template.Template
	adminNets []*net.IPNet // parsed admin CIDRs
}

func newServer(store *Store, cfg *Config) *server {
	s := &server{
		store: store,
		cfg:   cfg,
		tmpls: parseTemplates(),
	}
	// Parse admin CIDRs
	if cfg.AdminCIDR != "" {
		for _, cidr := range strings.Split(cfg.AdminCIDR, ",") {
			cidr = strings.TrimSpace(cidr)
			if cidr == "" {
				continue
			}
			_, network, err := net.ParseCIDR(cidr)
			if err == nil {
				s.adminNets = append(s.adminNets, network)
			}
		}
	}
	return s
}

func (s *server) isAdminAllowed(r *http.Request) bool {
	if len(s.adminNets) == 0 {
		return true // no restriction configured
	}
	clientIP := getClientIP(r)
	for _, network := range s.adminNets {
		if network.Contains(clientIP) {
			return true
		}
	}
	return false
}

func (s *server) handler() http.Handler {
	mux := http.NewServeMux()

	// Static assets
	mux.Handle("GET /static/", http.FileServerFS(staticFS))

	// Explicit routes
	mux.HandleFunc("GET /{$}", s.handleExplore)
	mux.HandleFunc("GET /admin", s.adminGuard(s.handleAdmin))
	mux.HandleFunc("GET /admin/new", s.adminGuard(s.handleEdit))
	mux.HandleFunc("GET /admin/edit/{name}", s.adminGuard(s.handleEdit))
	mux.HandleFunc("POST /admin/save", s.adminGuard(s.handleSave))
	mux.HandleFunc("POST /admin/delete/{name}", s.adminGuard(s.handleDelete))
	mux.HandleFunc("GET /opensearch.xml", s.handleOpenSearch)
	mux.HandleFunc("GET /api/suggestions", s.handleSuggestions)
	mux.HandleFunc("GET /api/links", s.handleAPILinks)

	// Catch-all redirect
	mux.HandleFunc("GET /{keyword}", s.handleRedirect)
	mux.HandleFunc("GET /{keyword}/{args...}", s.handleRedirect)

	return ipMiddleware(s.cfg.TrustProxy, mux)
}

// adminGuard wraps a handler with admin CIDR check.
func (s *server) adminGuard(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.isAdminAllowed(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		h(w, r)
	}
}

func (s *server) handleExplore(w http.ResponseWriter, r *http.Request) {
	s.renderTemplate(w, "explore.html", nil)
}

func (s *server) handleRedirect(w http.ResponseWriter, r *http.Request) {
	keyword := strings.ToLower(r.PathValue("keyword"))
	rawArgs := r.PathValue("args")

	// Handle OpenSearch-style queries: "keyword args" comes as a single keyword
	if rawArgs == "" && strings.Contains(keyword, " ") {
		parts := strings.SplitN(keyword, " ", 2)
		keyword = parts[0]
		rawArgs = parts[1]
	}

	// Also handle "keyword+args" (URL-encoded spaces)
	if rawArgs == "" && strings.Contains(keyword, "+") {
		parts := strings.SplitN(keyword, "+", 2)
		keyword = parts[0]
		rawArgs = strings.ReplaceAll(parts[1], "+", " ")
	}

	link, err := s.store.Get(keyword)
	if err != nil || link == nil {
		http.NotFound(w, r)
		return
	}

	clientIP := getClientIP(r)
	if !isLinkVisible(link, clientIP) {
		http.NotFound(w, r)
		return
	}

	// Normalize args: replace path separators with spaces
	args := strings.ReplaceAll(rawArgs, "/", " ")
	args = strings.TrimSpace(args)

	// JS snippet execution
	if link.JSSnippet != "" {
		// Use PathEscape to encode spaces as %20 (not +) for correct JS decoding
		s.renderTemplate(w, "redirect.html", map[string]interface{}{
			"Link":        link,
			"Args":        args,
			"ArgsEncoded": url.PathEscape(args),
			"URLEncoded":  url.PathEscape(link.URL),
		})
		return
	}

	// Build redirect URL
	targetURL := link.URL
	if strings.Contains(targetURL, "{args}") {
		targetURL = strings.ReplaceAll(targetURL, "{args}", url.QueryEscape(args))
	} else if strings.Contains(targetURL, "{1}") {
		// Positional args
		argParts := strings.Fields(args)
		for i, part := range argParts {
			placeholder := "{" + strconv.Itoa(i+1) + "}"
			targetURL = strings.ReplaceAll(targetURL, placeholder, url.QueryEscape(part))
		}
		// Clean up unused placeholders
		for i := len(argParts) + 1; i <= 9; i++ {
			placeholder := "{" + strconv.Itoa(i) + "}"
			targetURL = strings.ReplaceAll(targetURL, placeholder, "")
		}
	} else if args != "" {
		// Append as query parameter
		if strings.Contains(targetURL, "?") {
			targetURL += "&q=" + url.QueryEscape(args)
		} else {
			targetURL += "?q=" + url.QueryEscape(args)
		}
	}

	http.Redirect(w, r, targetURL, http.StatusFound)
}

func (s *server) handleAdmin(w http.ResponseWriter, r *http.Request) {
	links, err := s.store.All()
	if err != nil {
		http.Error(w, "Failed to load links", http.StatusInternalServerError)
		return
	}
	clientIP := getClientIP(r)
	visible := filterVisibleLinks(links, clientIP)
	s.renderTemplate(w, "admin.html", map[string]interface{}{
		"Links": visible,
	})
}

func (s *server) handleEdit(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var link *Link
	var completions []Completion

	if name != "" {
		var err error
		link, err = s.store.Get(name)
		if err != nil || link == nil {
			http.NotFound(w, r)
			return
		}
		// Note: no CIDR check here - edit page must remain accessible
		// even after autosave changes CIDRs to exclude the current user.
		// The CIDR warning in the UI handles this case.
		completions, _ = s.store.allCompletions(name)
	}

	s.renderTemplate(w, "edit.html", map[string]interface{}{
		"Link":        link,
		"Completions": completions,
		"IsNew":       link == nil,
		"ClientIP":    getClientIP(r).String(),
	})
}

func (s *server) handleSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid form data"})
		return
	}

	name := strings.ToLower(strings.TrimSpace(r.FormValue("name")))
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Name is required"})
		return
	}
	if !validNameRe.MatchString(name) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Name must contain only lowercase letters, numbers, and hyphens"})
		return
	}
	if len(name) > 64 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Name must be 64 characters or fewer"})
		return
	}
	if reservedNames[name] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "'" + name + "' is a reserved name"})
		return
	}

	urlVal := strings.TrimSpace(r.FormValue("url"))
	if urlVal == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "URL is required"})
		return
	}
	// Validate URL scheme: only allow http/https (or templates containing placeholders)
	if !isValidURLTemplate(urlVal) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "URL must use http:// or https:// scheme"})
		return
	}

	clientIP := getClientIP(r)

	// Check if this is a new link or edit
	isNew := r.FormValue("is_new") == "true"
	var link *Link
	if isNew {
		existing, _ := s.store.Get(name)
		if existing != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "Link '" + name + "' already exists"})
			return
		}
		link = &Link{
			Name:      name,
			CreatedAt: time.Now().UTC(),
		}
	} else {
		var err error
		link, err = s.store.Get(name)
		if err != nil || link == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Link not found"})
			return
		}
		// Enforce CIDR visibility: only users who can see the link can edit it
		if !isLinkVisible(link, clientIP) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "Access denied"})
			return
		}
	}

	link.URL = urlVal
	link.Description = strings.TrimSpace(r.FormValue("description"))
	link.Tags = strings.TrimSpace(r.FormValue("tags"))
	link.CIDRAllow = strings.TrimSpace(r.FormValue("cidr_allow"))
	link.JSSnippet = r.FormValue("js_snippet")
	link.CompletionsJS = r.FormValue("completions_js")

	if err := s.store.Save(link); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to save: " + err.Error()})
		return
	}

	// Parse and save completions
	completionsRaw := strings.TrimSpace(r.FormValue("completions"))
	var completions []Completion
	if completionsRaw != "" {
		for _, line := range strings.Split(completionsRaw, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, "|", 2)
			c := Completion{Value: strings.TrimSpace(parts[0])}
			if len(parts) > 1 {
				c.Description = strings.TrimSpace(parts[1])
			}
			completions = append(completions, c)
		}
	}
	if err := s.store.SetCompletions(name, completions); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to save completions: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "name": name})
}

func (s *server) handleDelete(w http.ResponseWriter, r *http.Request) {
	name := strings.ToLower(strings.TrimSpace(r.PathValue("name")))
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Name is required"})
		return
	}

	// Enforce CIDR visibility: only users who can see the link can delete it
	link, err := s.store.Get(name)
	if err != nil || link == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Link not found"})
		return
	}
	clientIP := getClientIP(r)
	if !isLinkVisible(link, clientIP) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Access denied"})
		return
	}

	if err := s.store.Delete(name); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to delete: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type apiLink struct {
	Name        string   `json:"name"`
	URL         string   `json:"url"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	HasArgs     bool     `json:"has_args"`
	HasJS       bool     `json:"has_js"`
	Restricted  bool     `json:"restricted"`
}

func (s *server) handleAPILinks(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	var links []*Link
	var err error
	if query != "" {
		links, err = s.store.SearchLinks(query)
	} else {
		links, err = s.store.All()
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	clientIP := getClientIP(r)
	visible := filterVisibleLinks(links, clientIP)

	if visible == nil {
		visible = []*Link{}
	}
	apiLinks := make([]apiLink, 0, len(visible))
	for _, l := range visible {
		tags := l.TagList()
		if tags == nil {
			tags = []string{}
		}
		apiLinks = append(apiLinks, apiLink{
			Name:        l.Name,
			URL:         l.URL,
			Description: l.Description,
			Tags:        tags,
			HasArgs:     strings.Contains(l.URL, "{args}") || strings.Contains(l.URL, "{1}"),
			HasJS:       l.JSSnippet != "",
			Restricted:  l.CIDRAllow != "",
		})
	}

	writeJSON(w, http.StatusOK, apiLinks)
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (s *server) renderTemplate(w http.ResponseWriter, name string, data interface{}) {
	if err := s.tmpls.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
	}
}
