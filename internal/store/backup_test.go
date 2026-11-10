package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestRestoreRollsBackOnIncompatibleBackup(t *testing.T) {
	live, err := Open(filepath.Join(t.TempDir(), "live.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	if err = live.Save(&Profile{ID: "kept", Name: "Kept", Slug: "kept", Draft: "original"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "incompatible.db")
	broken, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = broken.Exec("CREATE TABLE profiles (id TEXT)"); err != nil {
		t.Fatal(err)
	}
	broken.Close()
	if err = live.Restore(path); err == nil {
		t.Fatal("incompatible database accepted")
	}
	if p, err := live.Get("kept"); err != nil || p.Draft != "original" {
		t.Fatal("failed restore changed live data", err)
	}
}
