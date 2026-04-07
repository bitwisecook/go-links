package main

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Link struct {
	Name          string
	URL           string
	Description   string
	Tags          string
	CIDRAllow     string
	JSSnippet     string
	CompletionsJS string
	AppType       string // detected app type: jellyfin, plex, sonarr, etc.
	AppAPIKey     string // API key for app completions
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type Completion struct {
	Value       string `json:"value"`
	Description string `json:"description"`
}

type Store struct {
	db *sql.DB
}

func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(wal)&_pragma=foreign_keys(on)")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS links (
			name          TEXT PRIMARY KEY COLLATE NOCASE,
			url           TEXT NOT NULL,
			description   TEXT DEFAULT '',
			tags          TEXT DEFAULT '',
			cidr_allow    TEXT DEFAULT '',
			js_snippet    TEXT DEFAULT '',
			completions_js TEXT DEFAULT '',
			app_type      TEXT DEFAULT '',
			app_api_key   TEXT DEFAULT '',
			created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at    DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS completions_content (
			rowid       INTEGER PRIMARY KEY AUTOINCREMENT,
			link_name   TEXT NOT NULL COLLATE NOCASE REFERENCES links(name) ON DELETE CASCADE,
			value       TEXT NOT NULL COLLATE NOCASE,
			description TEXT DEFAULT ''
		)`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS completions USING fts5(
			link_name,
			value,
			description,
			content='completions_content',
			content_rowid='rowid'
		)`,
		// Triggers to keep FTS in sync
		`CREATE TRIGGER IF NOT EXISTS completions_ai AFTER INSERT ON completions_content BEGIN
			INSERT INTO completions(rowid, link_name, value, description)
			VALUES (new.rowid, new.link_name, new.value, new.description);
		END`,
		`CREATE TRIGGER IF NOT EXISTS completions_ad AFTER DELETE ON completions_content BEGIN
			INSERT INTO completions(completions, rowid, link_name, value, description)
			VALUES ('delete', old.rowid, old.link_name, old.value, old.description);
		END`,
		`CREATE TRIGGER IF NOT EXISTS completions_au AFTER UPDATE ON completions_content BEGIN
			INSERT INTO completions(completions, rowid, link_name, value, description)
			VALUES ('delete', old.rowid, old.link_name, old.value, old.description);
			INSERT INTO completions(rowid, link_name, value, description)
			VALUES (new.rowid, new.link_name, new.value, new.description);
		END`,
		// FTS for link search (standalone, not external content)
		`CREATE VIRTUAL TABLE IF NOT EXISTS links_fts USING fts5(
			name,
			description,
			tags
		)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("exec %q: %w", stmt[:60], err)
		}
	}

	// Migrations for existing databases: add new columns if missing
	migrations := []string{
		`ALTER TABLE links ADD COLUMN app_type TEXT DEFAULT ''`,
		`ALTER TABLE links ADD COLUMN app_api_key TEXT DEFAULT ''`,
	}
	for _, m := range migrations {
		s.db.Exec(m) // ignore errors (column already exists)
	}

	return nil
}

// Get retrieves a link by name (case-insensitive).
func (s *Store) Get(name string) (*Link, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	link := &Link{}
	err := s.db.QueryRow(
		`SELECT name, url, description, tags, cidr_allow, js_snippet, completions_js, app_type, app_api_key, created_at, updated_at
		 FROM links WHERE name = ? COLLATE NOCASE`, name,
	).Scan(&link.Name, &link.URL, &link.Description, &link.Tags, &link.CIDRAllow,
		&link.JSSnippet, &link.CompletionsJS, &link.AppType, &link.AppAPIKey, &link.CreatedAt, &link.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return link, nil
}

func (s *Store) All() ([]*Link, error) {
	rows, err := s.db.Query(
		`SELECT name, url, description, tags, cidr_allow, js_snippet, completions_js, app_type, app_api_key, created_at, updated_at
		 FROM links ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var links []*Link
	for rows.Next() {
		link := &Link{}
		if err := rows.Scan(&link.Name, &link.URL, &link.Description, &link.Tags, &link.CIDRAllow,
			&link.JSSnippet, &link.CompletionsJS, &link.AppType, &link.AppAPIKey, &link.CreatedAt, &link.UpdatedAt); err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

func (s *Store) Save(link *Link) error {
	link.Name = strings.ToLower(strings.TrimSpace(link.Name))
	now := time.Now().UTC()
	link.UpdatedAt = now

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		`INSERT INTO links (name, url, description, tags, cidr_allow, js_snippet, completions_js, app_type, app_api_key, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(name) DO UPDATE SET
			url=excluded.url, description=excluded.description, tags=excluded.tags,
			cidr_allow=excluded.cidr_allow, js_snippet=excluded.js_snippet,
			completions_js=excluded.completions_js, app_type=excluded.app_type,
			app_api_key=excluded.app_api_key, updated_at=excluded.updated_at`,
		link.Name, link.URL, link.Description, link.Tags, link.CIDRAllow,
		link.JSSnippet, link.CompletionsJS, link.AppType, link.AppAPIKey, link.CreatedAt, now,
	)
	if err != nil {
		return err
	}

	// Update links_fts within the same transaction
	// Use lowercase for FTS to ensure case-insensitive matching
	if _, err := tx.Exec(`DELETE FROM links_fts WHERE name = ? COLLATE NOCASE`, link.Name); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO links_fts (name, description, tags) VALUES (?, ?, ?)`,
		strings.ToLower(link.Name), strings.ToLower(link.Description), strings.ToLower(link.Tags),
	); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Store) Delete(name string) error {
	name = strings.ToLower(strings.TrimSpace(name))

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM links_fts WHERE name = ? COLLATE NOCASE`, name); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM completions_content WHERE link_name = ? COLLATE NOCASE`, name); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM links WHERE name = ? COLLATE NOCASE`, name); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Store) SearchLinks(query string) ([]*Link, error) {
	if query == "" {
		return s.All()
	}
	query = strings.ToLower(query)
	// Use FTS5 prefix search (FTS5 is case-insensitive by default)
	ftsQuery := strings.ReplaceAll(query, `"`, `""`)
	ftsQuery = `"` + ftsQuery + `"*`

	rows, err := s.db.Query(
		`SELECT l.name, l.url, l.description, l.tags, l.cidr_allow, l.js_snippet, l.completions_js, l.created_at, l.updated_at
		 FROM links l
		 JOIN links_fts f ON l.name = f.name COLLATE NOCASE
		 WHERE links_fts MATCH ?
		 ORDER BY rank`, ftsQuery)
	if err != nil {
		// Fallback to LIKE search if FTS fails
		return s.searchLinksFallback(query)
	}
	defer rows.Close()
	var links []*Link
	for rows.Next() {
		link := &Link{}
		if err := rows.Scan(&link.Name, &link.URL, &link.Description, &link.Tags, &link.CIDRAllow,
			&link.JSSnippet, &link.CompletionsJS, &link.AppType, &link.AppAPIKey, &link.CreatedAt, &link.UpdatedAt); err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

func (s *Store) searchLinksFallback(query string) ([]*Link, error) {
	pattern := "%" + query + "%"
	rows, err := s.db.Query(
		`SELECT name, url, description, tags, cidr_allow, js_snippet, completions_js, app_type, app_api_key, created_at, updated_at
		 FROM links WHERE name LIKE ? COLLATE NOCASE OR description LIKE ? COLLATE NOCASE OR tags LIKE ? COLLATE NOCASE
		 ORDER BY name`, pattern, pattern, pattern)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var links []*Link
	for rows.Next() {
		link := &Link{}
		if err := rows.Scan(&link.Name, &link.URL, &link.Description, &link.Tags, &link.CIDRAllow,
			&link.JSSnippet, &link.CompletionsJS, &link.AppType, &link.AppAPIKey, &link.CreatedAt, &link.UpdatedAt); err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

// SearchCompletions searches for completions case-insensitively.
func (s *Store) SearchCompletions(linkName, prefix string) ([]Completion, error) {
	linkName = strings.ToLower(strings.TrimSpace(linkName))
	if prefix == "" {
		return s.allCompletions(linkName)
	}
	prefix = strings.ToLower(prefix)
	ftsQuery := `"` + strings.ReplaceAll(prefix, `"`, `""`) + `"*`
	rows, err := s.db.Query(
		`SELECT value, description FROM completions
		 WHERE link_name = ? COLLATE NOCASE AND completions MATCH ?
		 ORDER BY rank LIMIT 10`, linkName, ftsQuery)
	if err != nil {
		// Fallback to LIKE
		return s.searchCompletionsFallback(linkName, prefix)
	}
	defer rows.Close()
	var completions []Completion
	for rows.Next() {
		var c Completion
		if err := rows.Scan(&c.Value, &c.Description); err != nil {
			return nil, err
		}
		completions = append(completions, c)
	}
	return completions, rows.Err()
}

func (s *Store) searchCompletionsFallback(linkName, prefix string) ([]Completion, error) {
	rows, err := s.db.Query(
		`SELECT value, description FROM completions_content
		 WHERE link_name = ? COLLATE NOCASE AND value LIKE ? COLLATE NOCASE
		 ORDER BY value LIMIT 10`, linkName, prefix+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var completions []Completion
	for rows.Next() {
		var c Completion
		if err := rows.Scan(&c.Value, &c.Description); err != nil {
			return nil, err
		}
		completions = append(completions, c)
	}
	return completions, rows.Err()
}

func (s *Store) allCompletions(linkName string) ([]Completion, error) {
	linkName = strings.ToLower(strings.TrimSpace(linkName))
	rows, err := s.db.Query(
		`SELECT value, description FROM completions_content
		 WHERE link_name = ? COLLATE NOCASE ORDER BY value LIMIT 20`, linkName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var completions []Completion
	for rows.Next() {
		var c Completion
		if err := rows.Scan(&c.Value, &c.Description); err != nil {
			return nil, err
		}
		completions = append(completions, c)
	}
	return completions, rows.Err()
}

// SetCompletions replaces all completions for a link, deduplicating by lowercase value.
func (s *Store) SetCompletions(linkName string, completions []Completion) error {
	linkName = strings.ToLower(strings.TrimSpace(linkName))

	// Deduplicate by lowercase value, last occurrence wins
	seen := make(map[string]int) // lowercase value -> index in deduped
	var deduped []Completion
	for _, c := range completions {
		key := strings.ToLower(strings.TrimSpace(c.Value))
		if key == "" {
			continue
		}
		if idx, exists := seen[key]; exists {
			// Replace with later occurrence (keeps latest description)
			deduped[idx] = c
		} else {
			seen[key] = len(deduped)
			deduped = append(deduped, c)
		}
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Delete existing
	if _, err := tx.Exec(`DELETE FROM completions_content WHERE link_name = ? COLLATE NOCASE`, linkName); err != nil {
		return err
	}

	// Insert deduplicated completions
	for _, c := range deduped {
		if _, err := tx.Exec(
			`INSERT INTO completions_content (link_name, value, description) VALUES (?, ?, ?)`,
			linkName, c.Value, c.Description,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) Close() error {
	return s.db.Close()
}

// TagList returns tags as a slice.
func (l *Link) TagList() []string {
	if l.Tags == "" {
		return nil
	}
	parts := strings.Split(l.Tags, ",")
	var tags []string
	for _, t := range parts {
		t = strings.TrimSpace(t)
		if t != "" {
			tags = append(tags, t)
		}
	}
	return tags
}

// CIDRList returns CIDR entries as a slice.
func (l *Link) CIDRList() []string {
	if l.CIDRAllow == "" {
		return nil
	}
	parts := strings.Split(l.CIDRAllow, "\n")
	var cidrs []string
	for _, c := range parts {
		c = strings.TrimSpace(c)
		if c != "" {
			cidrs = append(cidrs, c)
		}
	}
	return cidrs
}
