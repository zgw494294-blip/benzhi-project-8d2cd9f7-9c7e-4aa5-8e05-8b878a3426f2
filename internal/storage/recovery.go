package storage

import "path/filepath"

func (s *Store) SnapshotPath() string { return filepath.Join(s.dir, "snapshot.json") }
func (s *Store) EventPath() string    { return filepath.Join(s.dir, "events.jsonl") }
