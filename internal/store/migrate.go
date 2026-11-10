package store

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
)

// 首次使用 data/clash.db 时迁移旧库，保留一份位于 data 中的备份。
func MigrateLegacy(root, target string) error {
	legacy := filepath.Join(root, "clash.db")
	if filepath.Clean(legacy) == filepath.Clean(target) {
		return nil
	}
	if _, err := os.Stat(target); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if _, err := os.Stat(legacy); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".migration-*.db")
	if err != nil {
		return err
	}
	path := tmp.Name()
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Remove(path); err != nil {
		return err
	}
	defer os.Remove(path)
	db, err := sql.Open("sqlite", legacy)
	if err != nil {
		return err
	}
	_, err = db.Exec("VACUUM INTO ?", path)
	closeErr := db.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Chmod(path, 0600); err != nil {
		return err
	}
	if err = os.Rename(path, target); err != nil {
		return err
	}
	return os.Rename(legacy, filepath.Join(filepath.Dir(target), "clash-legacy.backup.db"))
}
