package application

import (
	"coldchain/internal/domain"
	"encoding/json"
)

func (s *Service) replay(key string, out any) bool {
	if key == "" {
		return false
	}
	b, ok := s.store.Idempotent(key)
	if !ok {
		return false
	}
	_ = json.Unmarshal(b, out)
	return true
}
func (s *Service) remember(key string, v any) {
	if key == "" {
		return
	}
	if b, e := json.Marshal(v); e == nil {
		s.store.SaveIdempotent(key, b)
	}
}
func (s *Service) Replay(key string, out any) bool { return s.replay(key, out) }
func (s *Service) Remember(key string, v any)      { s.remember(key, v) }
func (s *Service) RememberResult(key string, status int, v any) {
	if key == "" {
		return
	}
	b, e := json.Marshal(map[string]any{"_status": status, "body": v})
	if e == nil {
		s.store.SaveIdempotent(key, b)
	}
}
func (s *Service) ReplayResult(key string) (int, any, bool) {
	if key == "" {
		return 0, nil, false
	}
	b, ok := s.store.Idempotent(key)
	if !ok {
		return 0, nil, false
	}
	var envelope struct {
		Status int             `json:"_status"`
		Body   json.RawMessage `json:"body"`
	}
	if json.Unmarshal(b, &envelope) == nil && envelope.Status > 0 && envelope.Body != nil {
		var v any
		_ = json.Unmarshal(envelope.Body, &v)
		return envelope.Status, v, true
	}
	var v any
	if json.Unmarshal(b, &v) == nil {
		return 200, v, true
	}
	return 0, nil, false
}
func IsRetryable(e error) bool { return e == domain.ErrConflict || e == domain.ErrInvalidState }
