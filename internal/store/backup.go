package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
)

// VACUUM INTO 生成一致性副本，无需关闭服务或直接复制正在写入的文件。
func (s *Store) Backup(path string) error {
	_, err := s.db.Exec("VACUUM INTO '" + strings.ReplaceAll(path, "'", "''") + "'")
	return err
}

// 上传边界只读检查，不修改用户上传的备份。
func ValidateBackup(path string) error {
	uri := (&url.URL{Scheme: "file", Path: path}).String() + "?mode=ro"
	db, err := sql.Open("sqlite", uri)
	if err != nil {
		return err
	}
	defer db.Close()
	var check string
	if err = db.QueryRow("PRAGMA integrity_check").Scan(&check); err != nil {
		return err
	}
	if check != "ok" {
		return fmt.Errorf("数据库完整性检查失败")
	}
	rows, err := db.Query("SELECT id,name,slug,enabled,revision,original,draft,published,published_at,history FROM profiles LIMIT 0")
	if err != nil {
		return fmt.Errorf("不是本系统的数据库备份")
	}
	return rows.Close()
}

// 在当前连接内事务替换业务数据，保持运行中的数据库句柄不变。
func (s *Store) Restore(path string) error {
	conn, err := s.db.Conn(context.Background())
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err = conn.ExecContext(context.Background(), "ATTACH DATABASE ? AS restore_data", path); err != nil {
		return err
	}
	defer conn.ExecContext(context.Background(), "DETACH DATABASE restore_data")
	tx, err := conn.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	columns := "id,name,slug,enabled,revision,original,draft,published,published_at,history,kind,source_url,update_minutes,update_at,next_update,last_update,update_error,subscription_sync,client_update_hours,resource_proxy_prefix"
	if _, err = tx.Exec("DELETE FROM profiles"); err != nil {
		return err
	}
	if _, err = tx.Exec("INSERT INTO profiles (" + columns + ") SELECT " + columns + " FROM restore_data.profiles"); err != nil {
		return err
	}
	// 不恢复历史登录凭据，恢复完成后重新登录。
	if _, err = tx.Exec("DELETE FROM sessions"); err != nil {
		return err
	}
	return tx.Commit()
}
