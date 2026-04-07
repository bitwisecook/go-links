package main

import (
	"crypto/tls"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// AppInfo describes a detected application.
type AppInfo struct {
	Type        string `json:"type"`         // e.g. "jellyfin", "plex"
	Name        string `json:"name"`         // human-readable name
	Version     string `json:"version"`      // detected version
	Icon        string `json:"icon"`         // icon identifier for CSS class
	NeedsAPIKey bool   `json:"needs_api_key"`
	APIKeyHint  string `json:"api_key_hint"` // where to find the API key
	URLTemplate string `json:"url_template"` // suggested URL template with {args}
	Description string `json:"description"`  // suggested description
}

var appDetectors = []struct {
	name   string
	detect func(baseURL string, client *http.Client) *AppInfo
}{
	{"jellyfin", detectJellyfin},
	{"plex", detectPlex},
	{"sonarr", detectSonarr},
	{"radarr", detectRadarr},
	{"sabnzbd", detectSABnzbd},
	{"proxmox", detectProxmox},
	{"netbox", detectNetbox},
	{"openwebui", detectOpenWebUI},
}

// tlsSkipVerify is set from Config at startup.
var tlsSkipVerify bool

func newDetectClient() *http.Client {
	transport := &http.Transport{}
	if tlsSkipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return &http.Client{
		Timeout:   5 * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}
}

// DetectApp tries to identify the app at the given URL.
func DetectApp(rawURL string) *AppInfo {
	baseURL := extractBaseURL(rawURL)
	if baseURL == "" {
		return nil
	}
	client := newDetectClient()
	for _, d := range appDetectors {
		if info := d.detect(baseURL, client); info != nil {
			return info
		}
	}
	return nil
}

func extractBaseURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	if u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

func httpGet(client *http.Client, url string) ([]byte, int, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB max
	return body, resp.StatusCode, err
}

// --- Jellyfin ---

func detectJellyfin(baseURL string, client *http.Client) *AppInfo {
	body, status, err := httpGet(client, baseURL+"/System/Info/Public")
	if err != nil || status != 200 {
		return nil
	}
	var info struct {
		ServerName  string `json:"ServerName"`
		Version     string `json:"Version"`
		ProductName string `json:"ProductName"`
	}
	if json.Unmarshal(body, &info) != nil {
		return nil
	}
	if !strings.Contains(strings.ToLower(info.ProductName), "jellyfin") {
		return nil
	}
	name := info.ServerName
	if name == "" {
		name = "Jellyfin"
	}
	return &AppInfo{
		Type:        "jellyfin",
		Name:        name,
		Version:     info.Version,
		Icon:        "jellyfin",
		NeedsAPIKey: true,
		APIKeyHint:  "Dashboard → API Keys → Create",
		URLTemplate: baseURL + "/web/index.html#!/search.html?query={args}",
		Description: name + " media server",
	}
}

// --- Plex ---

func detectPlex(baseURL string, client *http.Client) *AppInfo {
	body, status, err := httpGet(client, baseURL+"/identity")
	if err != nil || status != 200 {
		return nil
	}
	// Plex returns XML by default
	var identity struct {
		XMLName          xml.Name `xml:"MediaContainer"`
		MachineID        string   `xml:"machineIdentifier,attr"`
		Version          string   `xml:"version,attr"`
		FriendlyName     string   `xml:"friendlyName,attr"`
	}
	if xml.Unmarshal(body, &identity) != nil || identity.MachineID == "" {
		return nil
	}
	name := identity.FriendlyName
	if name == "" {
		name = "Plex"
	}
	return &AppInfo{
		Type:        "plex",
		Name:        name,
		Version:     identity.Version,
		Icon:        "plex",
		NeedsAPIKey: true,
		APIKeyHint:  "Settings → Account → Authorized Devices (X-Plex-Token from URL)",
		URLTemplate: baseURL + "/web/index.html#!/search?query={args}",
		Description: name + " media server",
	}
}

// --- Sonarr ---

func detectSonarr(baseURL string, client *http.Client) *AppInfo {
	_, status, err := httpGet(client, baseURL+"/ping")
	if err != nil || status != 200 {
		return nil
	}
	// Verify it's Sonarr by checking the HTML page
	body, status2, _ := httpGet(client, baseURL+"/")
	if status2 == 200 && strings.Contains(strings.ToLower(string(body)), "sonarr") {
		return &AppInfo{
			Type:        "sonarr",
			Name:        "Sonarr",
			Icon:        "sonarr",
			NeedsAPIKey: true,
			APIKeyHint:  "Settings → General → API Key",
			URLTemplate: baseURL + "/series/{args}",
			Description: "Sonarr TV show manager",
		}
	}
	return nil
}

// --- Radarr ---

func detectRadarr(baseURL string, client *http.Client) *AppInfo {
	_, status, err := httpGet(client, baseURL+"/ping")
	if err != nil || status != 200 {
		return nil
	}
	body, status2, _ := httpGet(client, baseURL+"/")
	if status2 == 200 && strings.Contains(strings.ToLower(string(body)), "radarr") {
		return &AppInfo{
			Type:        "radarr",
			Name:        "Radarr",
			Icon:        "radarr",
			NeedsAPIKey: true,
			APIKeyHint:  "Settings → General → API Key",
			URLTemplate: baseURL + "/movie/{args}",
			Description: "Radarr movie manager",
		}
	}
	return nil
}

// --- SABnzbd ---

func detectSABnzbd(baseURL string, client *http.Client) *AppInfo {
	body, status, err := httpGet(client, baseURL+"/sabnzbd/api?mode=version")
	if err != nil || status != 200 {
		return nil
	}
	version := strings.TrimSpace(string(body))
	if version == "" || len(version) > 20 {
		return nil
	}
	return &AppInfo{
		Type:        "sabnzbd",
		Name:        "SABnzbd",
		Version:     version,
		Icon:        "sabnzbd",
		NeedsAPIKey: true,
		APIKeyHint:  "Config → General → API Key",
		URLTemplate: baseURL + "/sabnzbd/",
		Description: "SABnzbd download manager",
	}
}

// --- Proxmox ---

func detectProxmox(baseURL string, client *http.Client) *AppInfo {
	body, status, err := httpGet(client, baseURL+"/api2/json/version")
	if err != nil {
		return nil
	}
	// Proxmox returns 401 on unauthenticated requests, but the login page is identifiable
	if status == 401 || status == 200 {
		// Check if it looks like Proxmox
		if status == 200 {
			var ver struct {
				Data struct {
					Version string `json:"version"`
					Release string `json:"release"`
				} `json:"data"`
			}
			if json.Unmarshal(body, &ver) == nil && ver.Data.Version != "" {
				return &AppInfo{
					Type:        "proxmox",
					Name:        "Proxmox VE",
					Version:     ver.Data.Version,
					Icon:        "proxmox",
					NeedsAPIKey: true,
					APIKeyHint:  "Datacenter → Permissions → API Tokens → Add (PVEAPIToken=USER@REALM!TOKENID=SECRET)",
					URLTemplate: baseURL,
					Description: "Proxmox VE hypervisor",
				}
			}
		}
		// Try fetching the login page
		body2, _, _ := httpGet(client, baseURL+"/")
		if strings.Contains(strings.ToLower(string(body2)), "proxmox") {
			return &AppInfo{
				Type:        "proxmox",
				Name:        "Proxmox VE",
				Icon:        "proxmox",
				NeedsAPIKey: true,
				APIKeyHint:  "Datacenter → Permissions → API Tokens → Add",
				URLTemplate: baseURL,
				Description: "Proxmox VE hypervisor",
			}
		}
	}
	return nil
}

// --- NetBox ---

func detectNetbox(baseURL string, client *http.Client) *AppInfo {
	body, status, err := httpGet(client, baseURL+"/api/status/")
	if err != nil {
		return nil
	}
	if status == 200 || status == 403 {
		if status == 200 {
			var info struct {
				NetboxVersion string `json:"netbox-version"`
			}
			if json.Unmarshal(body, &info) == nil && info.NetboxVersion != "" {
				return &AppInfo{
					Type:        "netbox",
					Name:        "NetBox",
					Version:     info.NetboxVersion,
					Icon:        "netbox",
					NeedsAPIKey: true,
					APIKeyHint:  "Admin → API Tokens → Add Token",
					URLTemplate: baseURL + "/search/?q={args}",
					Description: "NetBox network documentation",
				}
			}
		}
		// Check HTML for NetBox branding
		body2, _, _ := httpGet(client, baseURL+"/")
		if strings.Contains(strings.ToLower(string(body2)), "netbox") {
			return &AppInfo{
				Type:        "netbox",
				Name:        "NetBox",
				Icon:        "netbox",
				NeedsAPIKey: true,
				APIKeyHint:  "Admin → API Tokens → Add Token",
				URLTemplate: baseURL + "/search/?q={args}",
				Description: "NetBox network documentation",
			}
		}
	}
	return nil
}

// --- Open WebUI ---

func detectOpenWebUI(baseURL string, client *http.Client) *AppInfo {
	_, status, err := httpGet(client, baseURL+"/health")
	if err != nil || status != 200 {
		return nil
	}
	// Verify with the main page
	body, _, _ := httpGet(client, baseURL+"/")
	pageStr := strings.ToLower(string(body))
	if strings.Contains(pageStr, "open webui") || strings.Contains(pageStr, "openwebui") {
		return &AppInfo{
			Type:        "openwebui",
			Name:        "Open WebUI",
			Icon:        "openwebui",
			NeedsAPIKey: true,
			APIKeyHint:  "Settings → Account → API Keys",
			URLTemplate: baseURL,
			Description: "Open WebUI LLM chat",
		}
	}
	return nil
}

// --- HTTP handler for /api/detect ---

func (s *server) handleDetect(w http.ResponseWriter, r *http.Request) {
	rawURL := strings.TrimSpace(r.URL.Query().Get("url"))
	if rawURL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url parameter required"})
		return
	}
	info := DetectApp(rawURL)
	if info == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"detected": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"detected": true, "app": info})
}
