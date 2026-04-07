package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// FetchAppCompletions fetches completions for a link based on its app type.
// Returns nil if the app type is unknown or no API key is set.
func FetchAppCompletions(link *Link) ([]Completion, error) {
	if link.AppType == "" || link.AppAPIKey == "" {
		return nil, nil
	}
	baseURL := extractBaseURL(link.URL)
	if baseURL == "" {
		return nil, fmt.Errorf("cannot extract base URL from %s", link.URL)
	}
	client := newDetectClient()

	switch link.AppType {
	case "jellyfin":
		return fetchJellyfinCompletions(baseURL, link.AppAPIKey, client)
	case "plex":
		return fetchPlexCompletions(baseURL, link.AppAPIKey, client)
	case "sonarr":
		return fetchSonarrCompletions(baseURL, link.AppAPIKey, client)
	case "radarr":
		return fetchRadarrCompletions(baseURL, link.AppAPIKey, client)
	case "sabnzbd":
		return fetchSABnzbdCompletions(baseURL, link.AppAPIKey, client)
	case "proxmox":
		return fetchProxmoxCompletions(baseURL, link.AppAPIKey, client)
	case "netbox":
		return fetchNetboxCompletions(baseURL, link.AppAPIKey, client)
	case "openwebui":
		return fetchOpenWebUICompletions(baseURL, link.AppAPIKey, client)
	default:
		return nil, nil
	}
}

func appHTTPGet(client *http.Client, url string, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body := make([]byte, 0, 4096)
	buf := make([]byte, 4096)
	total := 0
	for total < 1<<20 { // 1MB max
		n, err := resp.Body.Read(buf)
		if n > 0 {
			body = append(body, buf[:n]...)
			total += n
		}
		if err != nil {
			break
		}
	}
	return body, nil
}

// --- Jellyfin completions ---

func fetchJellyfinCompletions(baseURL, apiKey string, client *http.Client) ([]Completion, error) {
	headers := map[string]string{
		"Authorization": fmt.Sprintf(`MediaBrowser Token="%s"`, apiKey),
	}
	u := baseURL + "/Items?Recursive=true&IncludeItemTypes=Movie,Series&Fields=Name,ProductionYear&Limit=200&SortBy=SortName&SortOrder=Ascending"
	body, err := appHTTPGet(client, u, headers)
	if err != nil {
		return nil, err
	}
	var result struct {
		Items []struct {
			Name           string `json:"Name"`
			Type           string `json:"Type"`
			ProductionYear int    `json:"ProductionYear"`
		} `json:"Items"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	var completions []Completion
	for _, item := range result.Items {
		desc := item.Type
		if item.ProductionYear > 0 {
			desc = fmt.Sprintf("%s (%d)", item.Type, item.ProductionYear)
		}
		completions = append(completions, Completion{Value: item.Name, Description: desc})
	}
	return completions, nil
}

// --- Plex completions ---

func fetchPlexCompletions(baseURL, apiKey string, client *http.Client) ([]Completion, error) {
	headers := map[string]string{
		"X-Plex-Token": apiKey,
		"Accept":       "application/json",
	}
	// Get all library sections, then items from each
	body, err := appHTTPGet(client, baseURL+"/library/sections", headers)
	if err != nil {
		return nil, err
	}
	var sections struct {
		MediaContainer struct {
			Directory []struct {
				Key   string `json:"key"`
				Title string `json:"title"`
				Type  string `json:"type"`
			} `json:"Directory"`
		} `json:"MediaContainer"`
	}
	if err := json.Unmarshal(body, &sections); err != nil {
		return nil, err
	}
	var completions []Completion
	for _, section := range sections.MediaContainer.Directory {
		if section.Type != "movie" && section.Type != "show" {
			continue
		}
		itemsBody, err := appHTTPGet(client, baseURL+"/library/sections/"+section.Key+"/all?X-Plex-Container-Start=0&X-Plex-Container-Size=200", headers)
		if err != nil {
			continue
		}
		var items struct {
			MediaContainer struct {
				Metadata []struct {
					Title string `json:"title"`
					Year  int    `json:"year"`
					Type  string `json:"type"`
				} `json:"Metadata"`
			} `json:"MediaContainer"`
		}
		if json.Unmarshal(itemsBody, &items) != nil {
			continue
		}
		for _, m := range items.MediaContainer.Metadata {
			desc := m.Type
			if m.Year > 0 {
				desc = fmt.Sprintf("%s (%d)", m.Type, m.Year)
			}
			completions = append(completions, Completion{Value: m.Title, Description: desc})
		}
	}
	return completions, nil
}

// --- Sonarr completions ---

func fetchSonarrCompletions(baseURL, apiKey string, client *http.Client) ([]Completion, error) {
	headers := map[string]string{"X-Api-Key": apiKey}
	body, err := appHTTPGet(client, baseURL+"/api/v3/series", headers)
	if err != nil {
		return nil, err
	}
	var series []struct {
		Title      string `json:"title"`
		Year       int    `json:"year"`
		Status     string `json:"status"`
		SeasonCount int   `json:"seasonCount"`
	}
	if err := json.Unmarshal(body, &series); err != nil {
		return nil, err
	}
	var completions []Completion
	for _, s := range series {
		desc := fmt.Sprintf("%d seasons", s.SeasonCount)
		if s.Year > 0 {
			desc = fmt.Sprintf("%d, %s", s.Year, desc)
		}
		completions = append(completions, Completion{Value: s.Title, Description: desc})
	}
	return completions, nil
}

// --- Radarr completions ---

func fetchRadarrCompletions(baseURL, apiKey string, client *http.Client) ([]Completion, error) {
	headers := map[string]string{"X-Api-Key": apiKey}
	body, err := appHTTPGet(client, baseURL+"/api/v3/movie", headers)
	if err != nil {
		return nil, err
	}
	var movies []struct {
		Title  string `json:"title"`
		Year   int    `json:"year"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &movies); err != nil {
		return nil, err
	}
	var completions []Completion
	for _, m := range movies {
		desc := ""
		if m.Year > 0 {
			desc = fmt.Sprintf("%d", m.Year)
		}
		completions = append(completions, Completion{Value: m.Title, Description: desc})
	}
	return completions, nil
}

// --- SABnzbd completions ---

func fetchSABnzbdCompletions(baseURL, apiKey string, client *http.Client) ([]Completion, error) {
	u := fmt.Sprintf("%s/sabnzbd/api?mode=history&output=json&apikey=%s&limit=100", baseURL, url.QueryEscape(apiKey))
	body, err := appHTTPGet(client, u, nil)
	if err != nil {
		return nil, err
	}
	var result struct {
		History struct {
			Slots []struct {
				Name     string `json:"name"`
				Status   string `json:"status"`
				Category string `json:"category"`
			} `json:"slots"`
		} `json:"history"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	var completions []Completion
	for _, s := range result.History.Slots {
		desc := s.Status
		if s.Category != "" {
			desc = s.Category + " - " + s.Status
		}
		completions = append(completions, Completion{Value: s.Name, Description: desc})
	}
	return completions, nil
}

// --- Proxmox completions ---

func fetchProxmoxCompletions(baseURL, apiKey string, client *http.Client) ([]Completion, error) {
	headers := map[string]string{"Authorization": "PVEAPIToken=" + apiKey}
	body, err := appHTTPGet(client, baseURL+"/api2/json/cluster/resources?type=vm", headers)
	if err != nil {
		return nil, err
	}
	var result struct {
		Data []struct {
			Name   string `json:"name"`
			VMID   int    `json:"vmid"`
			Status string `json:"status"`
			Type   string `json:"type"`
			Node   string `json:"node"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	var completions []Completion
	for _, vm := range result.Data {
		desc := fmt.Sprintf("VMID %d, %s on %s (%s)", vm.VMID, vm.Type, vm.Node, vm.Status)
		completions = append(completions, Completion{Value: vm.Name, Description: desc})
	}
	return completions, nil
}

// --- NetBox completions ---

func fetchNetboxCompletions(baseURL, apiKey string, client *http.Client) ([]Completion, error) {
	headers := map[string]string{"Authorization": "Token " + apiKey}
	var completions []Completion

	// Fetch devices
	body, err := appHTTPGet(client, baseURL+"/api/dcim/devices/?limit=200", headers)
	if err == nil {
		var result struct {
			Results []struct {
				Name   string `json:"name"`
				Role   *struct{ Name string } `json:"role"`
				Site   *struct{ Name string } `json:"site"`
				Status *struct{ Label string } `json:"status"`
			} `json:"results"`
		}
		if json.Unmarshal(body, &result) == nil {
			for _, d := range result.Results {
				var parts []string
				if d.Role != nil {
					parts = append(parts, d.Role.Name)
				}
				if d.Site != nil {
					parts = append(parts, d.Site.Name)
				}
				completions = append(completions, Completion{Value: d.Name, Description: strings.Join(parts, ", ")})
			}
		}
	}

	// Fetch VMs
	body, err = appHTTPGet(client, baseURL+"/api/virtualization/virtual-machines/?limit=200", headers)
	if err == nil {
		var result struct {
			Results []struct {
				Name    string `json:"name"`
				Cluster *struct{ Name string } `json:"cluster"`
				Status  *struct{ Label string } `json:"status"`
			} `json:"results"`
		}
		if json.Unmarshal(body, &result) == nil {
			for _, vm := range result.Results {
				desc := "VM"
				if vm.Cluster != nil {
					desc = "VM on " + vm.Cluster.Name
				}
				completions = append(completions, Completion{Value: vm.Name, Description: desc})
			}
		}
	}

	return completions, nil
}

// --- Open WebUI completions ---

func fetchOpenWebUICompletions(baseURL, apiKey string, client *http.Client) ([]Completion, error) {
	headers := map[string]string{"Authorization": "Bearer " + apiKey}
	body, err := appHTTPGet(client, baseURL+"/api/models", headers)
	if err != nil {
		return nil, err
	}
	var result struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &result) == nil && len(result.Data) > 0 {
		var completions []Completion
		for _, m := range result.Data {
			name := m.Name
			if name == "" {
				name = m.ID
			}
			completions = append(completions, Completion{Value: name, Description: "Model: " + m.ID})
		}
		return completions, nil
	}
	// Try alternate format (array of models directly)
	var models []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if json.Unmarshal(body, &models) == nil {
		var completions []Completion
		for _, m := range models {
			name := m.Name
			if name == "" {
				name = m.ID
			}
			completions = append(completions, Completion{Value: name, Description: "Model: " + m.ID})
		}
		return completions, nil
	}
	return nil, fmt.Errorf("failed to parse models response")
}

// --- Background completion refresh ---

// RefreshAppCompletions fetches completions for all links with app_type and api_key set,
// and updates the completions_content table.
func RefreshAppCompletions(store *Store) {
	links, err := store.All()
	if err != nil {
		log.Printf("refresh completions: failed to list links: %v", err)
		return
	}
	for _, link := range links {
		if link.AppType == "" || link.AppAPIKey == "" {
			continue
		}
		completions, err := FetchAppCompletions(link)
		if err != nil {
			log.Printf("refresh completions for %s (%s): %v", link.Name, link.AppType, err)
			continue
		}
		if completions == nil {
			continue
		}
		if err := store.SetCompletions(link.Name, completions); err != nil {
			log.Printf("refresh completions for %s: failed to save: %v", link.Name, err)
		} else {
			log.Printf("refresh completions for %s (%s): %d items", link.Name, link.AppType, len(completions))
		}
	}
}

// StartCompletionRefresher starts a background goroutine that refreshes app completions periodically.
func StartCompletionRefresher(store *Store, interval time.Duration) {
	// Initial refresh after a short delay
	go func() {
		time.Sleep(5 * time.Second)
		RefreshAppCompletions(store)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			RefreshAppCompletions(store)
		}
	}()
}
