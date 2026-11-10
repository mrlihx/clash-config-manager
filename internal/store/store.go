package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type Profile struct {
	Sync          string
	SourceURL     string
	UpdateMinutes int
	UpdateAt      string
	NextUpdate    time.Time
	LastUpdate    time.Time
	UpdateError   string
	Kind          string
	ID            string
	Name          string
	Slug          string
	Enabled       bool
	Revision      int
	Original      string
	Draft         string
	Published     string
	PublishedAt   time.Time
}

type Store struct {
	sync.Mutex
	db *sql.DB
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = file.Close(); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS profiles (
 id TEXT PRIMARY KEY,
 name TEXT NOT NULL,
 slug TEXT NOT NULL UNIQUE,
 enabled INTEGER NOT NULL,
 revision INTEGER NOT NULL,
 original TEXT NOT NULL,
 draft TEXT NOT NULL,
 published TEXT NOT NULL,
 published_at TEXT NOT NULL,
 history TEXT NOT NULL
 )`)
	if err != nil {
		db.Close()
		return nil, err
	}
	rows, err := db.Query("PRAGMA table_info(profiles)")
	if err != nil {
		db.Close()
		return nil, err
	}
	columns := map[string]bool{}
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var defaultValue any
		if err = rows.Scan(&cid, &name, &typ, &notnull, &defaultValue, &pk); err != nil {
			rows.Close()
			db.Close()
			return nil, err
		}
		columns[name] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		db.Close()
		return nil, err
	}
	migrations := []struct{ name, definition string }{
		{"kind", "TEXT NOT NULL DEFAULT 'source'"},
		{"subscription_sync", "TEXT NOT NULL DEFAULT ''"},
		{"source_url", "TEXT NOT NULL DEFAULT ''"},
		{"update_minutes", "INTEGER NOT NULL DEFAULT 0"},
		{"update_at", "TEXT NOT NULL DEFAULT ''"},
		{"next_update", "INTEGER NOT NULL DEFAULT 0"},
		{"last_update", "INTEGER NOT NULL DEFAULT 0"},
		{"update_error", "TEXT NOT NULL DEFAULT ''"},
	}
	for _, column := range migrations {
		if columns[column.name] {
			continue
		}
		if _, err = db.Exec(fmt.Sprintf("ALTER TABLE profiles ADD COLUMN %s %s", column.name, column.definition)); err != nil {
			db.Close()
			return nil, err
		}
	}
	if columns["token"] {
		if _, err = db.Exec("ALTER TABLE profiles DROP COLUMN token"); err != nil {
			db.Close()
			return nil, err
		}
	}

	if _, err = db.Exec(`CREATE TABLE IF NOT EXISTS sessions (token_hash TEXT PRIMARY KEY, expires INTEGER NOT NULL)`); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

type Summary struct {
	RoutingPlan   string `json:"routingPlan"`
	FollowRouting bool   `json:"followRouting"`
	HasSource     bool   `json:"hasSource"`
	UpdateMinutes int    `json:"updateMinutes"`
	UpdateAt      string `json:"updateAt"`
	LastUpdate    int64  `json:"lastUpdate"`
	NextUpdate    int64  `json:"nextUpdate"`
	UpdateError   string `json:"updateError"`
	Kind          string `json:"kind"`
	ID            string `json:"id"`
	Name          string `json:"name"`
	Slug          string `json:"slug"`
	Enabled       bool   `json:"enabled"`
	Published     bool   `json:"published"`
	Dirty         bool   `json:"dirty"`
}

func (s *Store) List() ([]Summary, error) {
	rows, err := s.db.Query("SELECT kind,id,name,slug,enabled,published != '',draft != published,update_minutes,update_at,last_update,next_update,update_error,source_url!='',subscription_sync FROM profiles ORDER BY name,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Summary{}
	for rows.Next() {
		var p Summary
		var syncData string
		if err = rows.Scan(&p.Kind, &p.ID, &p.Name, &p.Slug, &p.Enabled, &p.Published, &p.Dirty, &p.UpdateMinutes, &p.UpdateAt, &p.LastUpdate, &p.NextUpdate, &p.UpdateError, &p.HasSource, &syncData); err != nil {
			return nil, err
		}
		var state struct {
			RoutingPlan   string
			FollowRouting *bool
		}
		if syncData != "" {
			if err = json.Unmarshal([]byte(syncData), &state); err != nil {
				return nil, err
			}
		}
		p.RoutingPlan = state.RoutingPlan
		p.FollowRouting = state.FollowRouting == nil || *state.FollowRouting
		result = append(result, p)
	}
	return result, rows.Err()
}

func (s *Store) Get(id string) (*Profile, error) {
	p := new(Profile)
	var stamp string
	var next, last int64
	err := s.db.QueryRow("SELECT kind,id,name,slug,enabled,revision,original,draft,published,published_at,source_url,update_minutes,update_at,next_update,last_update,update_error,subscription_sync FROM profiles WHERE id=?", id).
		Scan(&p.Kind, &p.ID, &p.Name, &p.Slug, &p.Enabled, &p.Revision, &p.Original, &p.Draft, &p.Published, &stamp, &p.SourceURL, &p.UpdateMinutes, &p.UpdateAt, &next, &last, &p.UpdateError, &p.Sync)
	if err != nil {
		return nil, err
	}
	if next > 0 {
		p.NextUpdate = time.Unix(next, 0)
	}
	if last > 0 {
		p.LastUpdate = time.Unix(last, 0)
	}
	p.PublishedAt, err = time.Parse(time.RFC3339Nano, stamp)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (s *Store) Save(p *Profile) error {
	if p.Kind == "" {
		p.Kind = "source"
	}
	// history 列保留以兼容旧数据库，新数据不再保存历史版本。
	_, err := s.db.Exec(`INSERT INTO profiles (id,name,slug,enabled,revision,original,draft,published,published_at,history,kind,source_url,update_minutes,update_at,next_update,last_update,update_error,subscription_sync)
 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET
 name=excluded.name,slug=excluded.slug,enabled=excluded.enabled,original=excluded.original,
 source_url=excluded.source_url,update_minutes=excluded.update_minutes,update_at=excluded.update_at,next_update=excluded.next_update,last_update=excluded.last_update,update_error=excluded.update_error,
 revision=excluded.revision,draft=excluded.draft,published=excluded.published,
 published_at=excluded.published_at,history=excluded.history,subscription_sync=excluded.subscription_sync`,
		p.ID, p.Name, p.Slug, p.Enabled, p.Revision, p.Original, p.Draft, p.Published, p.PublishedAt.Format(time.RFC3339Nano), "[]", p.Kind, p.SourceURL, p.UpdateMinutes, p.UpdateAt, p.NextUpdate.Unix(), p.LastUpdate.Unix(), p.UpdateError, p.Sync)
	return err
}

func (s *Store) SlugAvailable(slug, id string) (bool, error) {
	var available bool
	err := s.db.QueryRow("SELECT NOT EXISTS(SELECT 1 FROM profiles WHERE slug=? AND id!=?)", slug, id).Scan(&available)
	return available, err
}

func (s *Store) Published(slug string) (string, error) {
	var source string
	err := s.db.QueryRow("SELECT published FROM profiles WHERE slug=? AND enabled=1 AND published!=''", slug).Scan(&source)
	return source, err
}

func (s *Store) SaveSession(hash string, expires time.Time) error {
	_, err := s.db.Exec("INSERT OR REPLACE INTO sessions (token_hash,expires) VALUES (?,?)", hash, expires.Unix())
	return err
}
func (s *Store) HasSession(hash string) bool {
	var expires int64
	return s.db.QueryRow("SELECT expires FROM sessions WHERE token_hash=?", hash).Scan(&expires) == nil && expires > time.Now().Unix()
}
func (s *Store) DeleteSession(hash string) error {
	_, err := s.db.Exec("DELETE FROM sessions WHERE token_hash=?", hash)
	return err
}

func (s *Store) DeletePlan(id string) error {
	_, err := s.db.Exec("DELETE FROM profiles WHERE id=? AND kind='template'", id)
	return err
}
