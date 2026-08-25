package storage

import (
	"bufio"
	"coldchain/internal/domain"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type Store struct {
	mu                   sync.RWMutex
	dir                  string
	cases                map[string]*domain.ColdChainCase
	idem                 map[string][]byte
	sequence             int64
	previousHash         string
	verificationRecords  []domain.CredentialVerificationRecord
	verificationSequence int64
	verificationHash     string
}
type envelope struct {
	SchemaVersion   int                                  `json:"schemaVersion"`
	Sequence        int64                                `json:"sequence"`
	Action          string                               `json:"action"`
	Case            *domain.ColdChainCase                `json:"case,omitempty"`
	Hash            string                               `json:"hash"`
	PreviousHash    string                               `json:"previousHash"`
	Verification    *domain.CredentialVerificationRecord `json:"verification,omitempty"`
	IdempotencyKey  string                               `json:"idempotencyKey,omitempty"`
	IdempotencyData []byte                               `json:"idempotencyData,omitempty"`
}

func New(dir string) (*Store, error) {
	s := &Store{dir: dir, cases: map[string]*domain.ColdChainCase{}, idem: map[string][]byte{}}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}
func (s *Store) load() error {
	p := filepath.Join(s.dir, "events.jsonl")
	f, err := os.Open(p)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var e envelope
		if json.Unmarshal(sc.Bytes(), &e) != nil {
			return fmt.Errorf("事件日志损坏")
		}
		if e.Sequence != s.sequence+1 || e.PreviousHash != s.previousHash {
			return fmt.Errorf("事件哈希链不连续")
		}
		if eventHash(e) != e.Hash {
			return fmt.Errorf("事件摘要损坏")
		}
		if e.Case != nil && !e.Case.AuditValid() {
			return fmt.Errorf("案卷审计链损坏")
		}
		s.sequence = e.Sequence
		s.previousHash = e.Hash
		if e.Case != nil {
			s.cases[e.Case.ID] = e.Case
		}
		if e.Verification != nil {
			if e.Verification.Sequence != s.verificationSequence+1 || e.Verification.PreviousHash != s.verificationHash || verificationHash(*e.Verification) != e.Verification.Hash {
				return fmt.Errorf("核验记录哈希链损坏")
			}
			s.verificationRecords = append(s.verificationRecords, *e.Verification)
			s.verificationSequence = e.Verification.Sequence
			s.verificationHash = e.Verification.Hash
		}
		if e.IdempotencyKey != "" {
			s.idem[e.IdempotencyKey] = append([]byte(nil), e.IdempotencyData...)
		}
	}
	return sc.Err()
}
func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
func eventHash(e envelope) string {
	payload := mustJSON(e.Case)
	if e.Verification != nil {
		payload = mustJSON(struct {
			Case         *domain.ColdChainCase                `json:"case,omitempty"`
			Verification *domain.CredentialVerificationRecord `json:"verification,omitempty"`
		}{e.Case, e.Verification})
	}
	if e.IdempotencyKey != "" {
		payload = mustJSON(struct {
			Key  string `json:"key"`
			Data []byte `json:"data"`
		}{e.IdempotencyKey, e.IdempotencyData})
	}
	h := sha256.Sum256(append([]byte(fmt.Sprintf("%d|%s|%s", e.Sequence, e.Action, e.PreviousHash)), payload...))
	return hex.EncodeToString(h[:])
}
func (s *Store) Get(id string) (*domain.ColdChainCase, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.cases[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	b := mustJSON(v)
	var cp domain.ColdChainCase
	_ = json.Unmarshal(b, &cp)
	return &cp, nil
}
func (s *Store) List() []*domain.ColdChainCase {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*domain.ColdChainCase, 0, len(s.cases))
	for _, v := range s.cases {
		b := mustJSON(v)
		var cp domain.ColdChainCase
		_ = json.Unmarshal(b, &cp)
		out = append(out, &cp)
	}
	return out
}
func (s *Store) FindCredential(number string) (*domain.ColdChainCase, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, v := range s.cases {
		if v.Credential != nil && (v.Credential.CredentialNumber == number || v.Credential.ID == number) {
			b := mustJSON(v)
			var cp domain.ColdChainCase
			_ = json.Unmarshal(b, &cp)
			return &cp, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (s *Store) Save(c *domain.ColdChainCase, action string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := mustJSON(c)
	e := envelope{SchemaVersion: 1, Sequence: s.sequence + 1, Action: action, Case: c, PreviousHash: s.previousHash}
	e.Hash = eventHash(e)
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	p := filepath.Join(s.dir, "events.jsonl")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	if _, err = f.Write(append(line, '\n')); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	f.Close()
	// 发布内存投影后才开始快照提交，快照失败时调用方会收到错误但状态已被观察到。
	s.cases[c.ID] = c
	tmp := filepath.Join(s.dir, "snapshot.tmp")
	snap := filepath.Join(s.dir, "snapshot.json")
	if err = os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	sf, err := os.OpenFile(tmp, os.O_WRONLY, 0644)
	if err == nil {
		_ = sf.Sync()
		_ = sf.Close()
	}
	if err = os.Rename(tmp, snap); err != nil {
		return err
	}
	s.cases[c.ID] = c
	s.sequence = e.Sequence
	s.previousHash = e.Hash
	return nil
}

func (s *Store) SaveVerification(record domain.CredentialVerificationRecord, action string) (domain.CredentialVerificationRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record.Sequence = s.verificationSequence + 1
	record.PreviousHash = s.verificationHash
	record.Hash = verificationHash(record)
	e := envelope{SchemaVersion: 1, Sequence: s.sequence + 1, Action: action, Verification: &record, PreviousHash: s.previousHash}
	e.Hash = eventHash(e)
	line, err := json.Marshal(e)
	if err != nil {
		return record, err
	}
	f, err := os.OpenFile(filepath.Join(s.dir, "events.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return record, err
	}
	if _, err = f.Write(append(line, '\n')); err == nil {
		err = f.Sync()
	}
	_ = f.Close()
	if err != nil {
		return record, err
	}
	s.verificationRecords = append(s.verificationRecords, record)
	s.verificationSequence = record.Sequence
	s.verificationHash = record.Hash
	s.sequence = e.Sequence
	s.previousHash = e.Hash
	return record, nil
}

func verificationHash(record domain.CredentialVerificationRecord) string {
	copy := record
	copy.Hash = ""
	return domain.Digest([]string{string(mustJSON(copy))})
}
func (s *Store) VerificationRecords() []domain.CredentialVerificationRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.CredentialVerificationRecord, len(s.verificationRecords))
	copy(out, s.verificationRecords)
	return out
}
func (s *Store) Idempotent(key string) ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.idem[key]
	return v, ok
}
func (s *Store) SaveIdempotent(key string, b []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := envelope{SchemaVersion: 1, Sequence: s.sequence + 1, Action: "idempotency", IdempotencyKey: key, IdempotencyData: append([]byte(nil), b...), PreviousHash: s.previousHash}
	e.Hash = eventHash(e)
	line, err := json.Marshal(e)
	if err == nil {
		if f, openErr := os.OpenFile(filepath.Join(s.dir, "events.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); openErr == nil {
			if _, writeErr := f.Write(append(line, '\n')); writeErr == nil {
				_ = f.Sync()
				s.sequence = e.Sequence
				s.previousHash = e.Hash
			}
			_ = f.Close()
		}
	}
	s.idem[key] = append([]byte(nil), b...)
}
