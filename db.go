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
			name          TEXT PRIMARY KEY,
			url           TEXT NOT NULL,
			description   TEXT DEFAULT '',
			tags          TEXT DEFAULT '',
			cidr_allow    TEXT DEFAULT '',
			js_snippet    TEXT DEFAULT '',
			completions_js TEXT DEFAULT '',
			created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at    DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS completions_content (
			rowid       INTEGER PRIMARY KEY AUTOINCREMENT,
			link_name   TEXT NOT NULL REFERENCES links(name) ON DELETE CASCADE,
			value       TEXT NOT NULL,
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
		// FTS for link search
		`CREATE VIRTUAL TABLE IF NOT EXISTS links_fts USING fts5(
			name,
			description,
			tags,
			content='links',
			content_rowid='rowid'
		)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("exec %q: %w", stmt[:60], err)
		}
	}
	return nil
}

func (s *Store) Get(name string) (*Link, error) {
	link := &Link{}
	err := s.db.QueryRow(
		`SELECT name, url, description, tags, cidr_allow, js_snippet, completions_js, created_at, updated_at
		 FROM links WHERE name = ?`, name,
	).Scan(&link.Name, &link.URL, &link.Description, &link.Tags, &link.CIDRAllow,
		&link.JSSnippet, &link.CompletionsJS, &link.CreatedAt, &link.UpdatedAt)
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
		`SELECT name, url, description, tags, cidr_allow, js_snippet, completions_js, created_at, updated_at
		 FROM links ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var links []*Link
	for rows.Next() {
		link := &Link{}
		if err := rows.Scan(&link.Name, &link.URL, &link.Description, &link.Tags, &link.CIDRAllow,
			&link.JSSnippet, &link.CompletionsJS, &link.CreatedAt, &link.UpdatedAt); err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

func (s *Store) Save(link *Link) error {
	now := time.Now().UTC()
	link.UpdatedAt = now

	_, err := s.db.Exec(
		`INSERT INTO links (name, url, description, tags, cidr_allow, js_snippet, completions_js, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(name) DO UPDATE SET
			url=excluded.url, description=excluded.description, tags=excluded.tags,
			cidr_allow=excluded.cidr_allow, js_snippet=excluded.js_snippet,
			completions_js=excluded.completions_js, updated_at=excluded.updated_at`,
		link.Name, link.URL, link.Description, link.Tags, link.CIDRAllow,
		link.JSSnippet, link.CompletionsJS, link.CreatedAt, now,
	)
	if err != nil {
		return err
	}

	// Update links_fts
	s.db.Exec(`DELETE FROM links_fts WHERE name = ?`, link.Name)
	_, err = s.db.Exec(
		`INSERT INTO links_fts (name, description, tags) VALUES (?, ?, ?)`,
		link.Name, link.Description, link.Tags,
	)
	return err
}

func (s *Store) Delete(name string) error {
	s.db.Exec(`DELETE FROM links_fts WHERE name = ?`, name)
	s.db.Exec(`DELETE FROM completions_content WHERE link_name = ?`, name)
	_, err := s.db.Exec(`DELETE FROM links WHERE name = ?`, name)
	return err
}

func (s *Store) SearchLinks(query string) ([]*Link, error) {
	if query == "" {
		return s.All()
	}
	// Use FTS5 prefix search
	ftsQuery := strings.ReplaceAll(query, `"`, `""`)
	ftsQuery = `"` + ftsQuery + `"*`

	rows, err := s.db.Query(
		`SELECT l.name, l.url, l.description, l.tags, l.cidr_allow, l.js_snippet, l.completions_js, l.created_at, l.updated_at
		 FROM links l
		 JOIN links_fts f ON l.name = f.name
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
			&link.JSSnippet, &link.CompletionsJS, &link.CreatedAt, &link.UpdatedAt); err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

func (s *Store) searchLinksFallback(query string) ([]*Link, error) {
	pattern := "%" + query + "%"
	rows, err := s.db.Query(
		`SELECT name, url, description, tags, cidr_allow, js_snippet, completions_js, created_at, updated_at
		 FROM links WHERE name LIKE ? OR description LIKE ? OR tags LIKE ?
		 ORDER BY name`, pattern, pattern, pattern)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var links []*Link
	for rows.Next() {
		link := &Link{}
		if err := rows.Scan(&link.Name, &link.URL, &link.Description, &link.Tags, &link.CIDRAllow,
			&link.JSSnippet, &link.CompletionsJS, &link.CreatedAt, &link.UpdatedAt); err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

func (s *Store) SearchCompletions(linkName, prefix string) ([]Completion, error) {
	if prefix == "" {
		return s.allCompletions(linkName)
	}
	ftsQuery := `"` + strings.ReplaceAll(prefix, `"`, `""`) + `"*`
	rows, err := s.db.Query(
		`SELECT value, description FROM completions
		 WHERE link_name = ? AND completions MATCH ?
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
		 WHERE link_name = ? AND value LIKE ?
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
	rows, err := s.db.Query(
		`SELECT value, description FROM completions_content
		 WHERE link_name = ? ORDER BY value LIMIT 20`, linkName)
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

func (s *Store) SetCompletions(linkName string, completions []Completion) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Delete existing
	if _, err := tx.Exec(`DELETE FROM completions_content WHERE link_name = ?`, linkName); err != nil {
		return err
	}

	// Insert new
	for _, c := range completions {
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
