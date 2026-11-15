package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestMigrationDropsSubscriptionTokensAndKeepsConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE profiles(id TEXT PRIMARY KEY,name TEXT,slug TEXT UNIQUE,token TEXT NOT NULL,enabled INTEGER,revision INTEGER,original TEXT,draft TEXT,published TEXT,published_at TEXT,history TEXT);
 INSERT INTO profiles VALUES('id','Legacy','legacy','legacy-token',1,1,'original','draft','published','0001-01-01T00:00:00Z','[]')`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	current, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer current.Close()
	p, err := current.Get("id")
	if err != nil || p.Draft != "draft" || p.Kind != "source" {
		t.Fatalf("legacy configuration changed: %v", err)
	}
	if _, err = current.db.Exec("SELECT token FROM profiles"); err == nil {
		t.Fatal("obsolete token column was retained")
	}
	p.Name = "Updated"
	if err = current.Save(p); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyDatabaseMovesToDataWithBackup(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "clash.db")
	db, err := Open(legacy)
	if err != nil {
		t.Fatal(err)
	}
	p := &Profile{ID: "id", Name: "Saved", Slug: "saved", Kind: "subscription", Revision: 1, Draft: "saved yaml", Published: "published yaml"}
	if err = db.Save(p); err != nil {
		t.Fatal(err)
	}
	db.Close()
	target := filepath.Join(root, "data", "clash.db")
	if err = MigrateLegacy(root, target); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatal("database remained at project root")
	}
	if _, err = os.Stat(filepath.Join(root, "data", "clash-legacy.backup.db")); err != nil {
		t.Fatal(err)
	}
	moved, err := Open(target)
	if err != nil {
		t.Fatal(err)
	}
	defer moved.Close()
	restored, err := moved.Get("id")
	if err != nil || restored.Published != p.Published {
		t.Fatal("migration changed subscription data")
	}
	if err = MigrateLegacy(root, target); err != nil {
		t.Fatal("migration is not idempotent")
	}
}
