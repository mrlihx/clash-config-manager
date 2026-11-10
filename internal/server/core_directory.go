package server

import (
	"errors"
	"os"
	"path/filepath"
)

// PrepareCoreDirectory 将旧的内核运行缓存迁入 data，不覆盖已经存在的新缓存。
func PrepareCoreDirectory(root string) error {
	directory := filepath.Join(root, "data")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	for _, name := range []string{"cache.db", "rules"} {
		source, target := filepath.Join(root, name), filepath.Join(directory, name)
		if _, err := os.Stat(target); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if _, err := os.Stat(source); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		if err := os.Rename(source, target); err != nil {
			return err
		}
	}
	return nil
}
