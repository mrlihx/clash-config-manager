package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareCoreDirectoryMovesCacheAndPreservesNewerCache(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "cache.db"), []byte("legacy"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "rules"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := PrepareCoreDirectory(root); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "data", "cache.db"))
	if err != nil || string(data) != "legacy" {
		t.Fatal("cache not migrated", err)
	}
	if _, err = os.Stat(filepath.Join(root, "cache.db")); !os.IsNotExist(err) {
		t.Fatal("old cache still in root")
	}
	if _, err = os.Stat(filepath.Join(root, "data", "rules")); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "cache.db"), []byte("other"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = PrepareCoreDirectory(root); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(filepath.Join(root, "data", "cache.db"))
	if string(data) != "legacy" {
		t.Fatal("existing cache overwritten")
	}
}
