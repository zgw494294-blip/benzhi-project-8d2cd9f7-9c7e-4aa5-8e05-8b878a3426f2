package storage

import "sync"

type CaseLock struct{ mu sync.Mutex }

func (l *CaseLock) With(fn func() error) error { l.mu.Lock(); defer l.mu.Unlock(); return fn() }

type LockTable struct {
	mu    sync.Mutex
	items map[string]*CaseLock
}

func NewLockTable() *LockTable { return &LockTable{items: map[string]*CaseLock{}} }
func (t *LockTable) Get(id string) *CaseLock {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.items[id] == nil {
		t.items[id] = &CaseLock{}
	}
	return t.items[id]
}
