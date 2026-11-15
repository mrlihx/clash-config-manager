package store

import "time"

func (s *Store) DueSources(now time.Time) ([]string, error) {
	rows, err := s.db.Query("SELECT id FROM profiles WHERE kind='source' AND source_url!='' AND (update_minutes>0 OR update_at!='') AND next_update<=?", now.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) DeleteSource(p *Profile) error {
	if p.Published != "" {
		// 兼容早期直接从来源发布的链接，移出来源列表后仍可在订阅管理使用。
		_, err := s.db.Exec("UPDATE profiles SET kind='subscription',source_url='',update_minutes=0,update_at='',next_update=0 WHERE id=? AND kind='source'", p.ID)
		return err
	}
	_, err := s.db.Exec("DELETE FROM profiles WHERE id=? AND kind='source'", p.ID)
	return err
}
