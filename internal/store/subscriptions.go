package store

func (s *Store) DeleteSubscription(p *Profile) error {
	if p.Kind == "source" {
		// 早期由来源直接发布的订阅只取消发布，保留来源节点。
		_, err := s.db.Exec("UPDATE profiles SET published='',published_at='0001-01-01T00:00:00Z',revision=revision+1 WHERE id=?", p.ID)
		return err
	}
	_, err := s.db.Exec("DELETE FROM profiles WHERE id=? AND kind='subscription'", p.ID)
	return err
}
