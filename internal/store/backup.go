package store

import "strings"

// VACUUM INTO 生成一致性副本，无需关闭服务或直接复制正在写入的文件。
func (s *Store) Backup(path string) error {
	_, err := s.db.Exec("VACUUM INTO '" + strings.ReplaceAll(path, "'", "''") + "'")
	return err
}
