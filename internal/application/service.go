package application

import (
	"coldchain/internal/domain"
	"coldchain/internal/storage"
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type Service struct {
	store        *storage.Store
	locks        sync.Map
	createMu     sync.Mutex
	idemCreateMu sync.Mutex
	credentialMu sync.Mutex
}

func New(store *storage.Store) *Service { return &Service{store: store} }
func (s *Service) lock(id string) *sync.Mutex {
	v, _ := s.locks.LoadOrStore(id, &sync.Mutex{})
	return v.(*sync.Mutex)
}
func (s *Service) with(id string, version int, action string, fn func(*domain.ColdChainCase) error) (*domain.ColdChainCase, error) {
	m := s.lock(id)
	m.Lock()
	defer m.Unlock()
	c, e := s.store.Get(id)
	if e != nil {
		return nil, e
	}
	if version > 0 && c.Version != version {
		return nil, domain.ErrConflict
	}
	if e = fn(c); e != nil {
		return nil, e
	}
	c.RecordAudit(action, "system", action)
	if e = s.store.Save(c, action); e != nil {
		return nil, e
	}
	return c, nil
}
func (s *Service) Create(number, sender, receiver string, start, end time.Time) (*domain.ColdChainCase, error) {
	s.createMu.Lock()
	defer s.createMu.Unlock()
	for _, existing := range s.store.List() {
		if strings.EqualFold(strings.TrimSpace(existing.CaseNumber), strings.TrimSpace(number)) {
			return nil, &domain.CaseConflictError{Code: "DUPLICATE_CASE_NUMBER", CaseNumber: existing.CaseNumber, ConflictingCase: existing.ID}
		}
	}
	id := fmt.Sprintf("case-%d", time.Now().UnixNano())
	c, e := domain.NewCase(id, number, sender, receiver, start, end, time.Now().UTC())
	if e != nil {
		return nil, e
	}
	c.RecordAudit("create", "system", "create")
	if e = s.store.Save(c, "create"); e != nil {
		return nil, e
	}
	return c, nil
}
func (s *Service) CreateWithIdempotency(key, number, sender, receiver string, start, end time.Time) (*domain.ColdChainCase, int, any, error) {
	if key == "" {
		c, e := s.Create(number, sender, receiver, start, end)
		if e != nil {
			return nil, statusFor(e), errorValue(e), e
		}
		return c, 201, c, nil
	}
	s.idemCreateMu.Lock()
	defer s.idemCreateMu.Unlock()
	if code, body, ok := s.ReplayResult(key); ok {
		return nil, code, body, nil
	}
	c, e := s.Create(number, sender, receiver, start, end)
	if e != nil {
		code := statusFor(e)
		body := errorValue(e)
		s.RememberResult(key, code, body)
		return nil, code, body, e
	}
	s.RememberResult(key, 201, c)
	return c, 201, c, nil
}
func statusFor(e error) int {
	if errors.Is(e, domain.ErrConflict) {
		return 409
	}
	if errors.Is(e, domain.ErrInvalid) {
		return 422
	}
	return 400
}
func errorValue(e error) map[string]string {
	out := map[string]string{"error": e.Error()}
	var ce *domain.CaseConflictError
	if errors.As(e, &ce) {
		out["code"] = ce.Code
		out["conflictingCase"] = ce.ConflictingCase
		out["windowStart"] = ce.WindowStart
		out["windowEnd"] = ce.WindowEnd
	}
	return out
}
func (s *Service) Get(id string) (*domain.ColdChainCase, error) {
	c, e := s.store.Get(id)
	if e == nil {
		s.Enrich(c, time.Now().UTC())
	}
	return c, e
}
func (s *Service) List() []*domain.ColdChainCase { return s.store.List() }

func (s *Service) Enrich(c *domain.ColdChainCase, now time.Time) {
	if c == nil {
		return
	}
	all := s.store.List()
	c.WindowConflicts = make([]domain.WindowConflict, 0)
	for _, other := range all {
		if other.ID == c.ID || !strings.EqualFold(strings.TrimSpace(other.SenderName), strings.TrimSpace(c.SenderName)) || !strings.EqualFold(strings.TrimSpace(other.ReceiverName), strings.TrimSpace(c.ReceiverName)) {
			continue
		}
		if c.HandoffWindowStart.Before(other.HandoffWindowEnd) && other.HandoffWindowStart.Before(c.HandoffWindowEnd) {
			c.WindowConflicts = append(c.WindowConflicts, domain.WindowConflict{CaseID: other.ID, CaseNumber: other.CaseNumber, WindowStart: other.HandoffWindowStart, WindowEnd: other.HandoffWindowEnd, Status: other.Status})
		}
	}
	sort.Slice(c.WindowConflicts, func(i, j int) bool {
		if c.WindowConflicts[i].WindowStart.Equal(c.WindowConflicts[j].WindowStart) {
			return c.WindowConflicts[i].CaseNumber < c.WindowConflicts[j].CaseNumber
		}
		return c.WindowConflicts[i].WindowStart.Before(c.WindowConflicts[j].WindowStart)
	})
	c.WindowConflictDigest = domain.WindowConflictDigest(c.WindowConflicts)
	c.WindowPhase, c.RemainingMinutes = domain.WindowTiming(c, now)
	c.ProbeCoverageLedger = domain.CalibrationLedger(c)
	c.UpdateRemediationTasks(now)
}

type ListResult struct {
	Items           []*domain.ColdChainCase `json:"items"`
	Counts          map[string]int          `json:"counts"`
	Total           int                     `json:"total"`
	LatestUpdatedAt time.Time               `json:"latestUpdatedAt"`
	Page            int                     `json:"page"`
	PageSize        int                     `json:"pageSize"`
	HasNext         bool                    `json:"hasNext"`
	NextPage        int                     `json:"nextPage,omitempty"`
}

func (s *Service) Query(f domain.CaseFilter) (ListResult, error) {
	if f.HandoffStart != nil && f.HandoffEnd != nil && f.HandoffEnd.Before(*f.HandoffStart) {
		return ListResult{}, domain.ErrInvalid
	}
	page, pageSize := f.Page, f.PageSize
	if page == 0 {
		page = 1
	}
	if pageSize == 0 {
		pageSize = 50
	}
	if page < 1 || pageSize < 1 || pageSize > 100 || page > int(^uint(0)>>1)/pageSize {
		return ListResult{}, domain.ErrInvalid
	}
	if f.WindowPhase != "" && f.WindowPhase != domain.WindowPhasePending && f.WindowPhase != domain.WindowPhaseActive && f.WindowPhase != domain.WindowPhaseEnded || f.DueWithinMinutes != nil && *f.DueWithinMinutes < 0 {
		return ListResult{}, domain.ErrInvalid
	}
	if f.Now.IsZero() {
		f.Now = time.Now().UTC()
	}
	all := s.store.List()
	out := make([]*domain.ColdChainCase, 0, len(all))
	counts := map[string]int{}
	for _, c := range all {
		phase, remaining := domain.WindowTiming(c, f.Now)
		if f.Status != "" && string(c.Status) != f.Status || f.CaseNumber != "" && !strings.Contains(c.CaseNumber, f.CaseNumber) || f.Sender != "" && !strings.Contains(c.SenderName, f.Sender) || f.Receiver != "" && !strings.Contains(c.ReceiverName, f.Receiver) {
			continue
		}
		if f.HandoffStart != nil && c.HandoffWindowStart.Before(*f.HandoffStart) {
			continue
		}
		if f.HandoffEnd != nil && c.HandoffWindowEnd.After(*f.HandoffEnd) {
			continue
		}
		if f.WindowPhase != "" && phase != f.WindowPhase {
			continue
		}
		if f.DueWithinMinutes != nil && (phase == domain.WindowPhaseEnded || remaining > int64(*f.DueWithinMinutes)) {
			continue
		}
		c.WindowPhase, c.RemainingMinutes = phase, remaining
		c.ProbeCoverageLedger = domain.CalibrationLedger(c)
		c.WindowConflicts = make([]domain.WindowConflict, 0)
		for _, other := range all {
			if other.ID == c.ID || !strings.EqualFold(strings.TrimSpace(other.SenderName), strings.TrimSpace(c.SenderName)) || !strings.EqualFold(strings.TrimSpace(other.ReceiverName), strings.TrimSpace(c.ReceiverName)) {
				continue
			}
			if c.HandoffWindowStart.Before(other.HandoffWindowEnd) && other.HandoffWindowStart.Before(c.HandoffWindowEnd) {
				c.WindowConflicts = append(c.WindowConflicts, domain.WindowConflict{CaseID: other.ID, CaseNumber: other.CaseNumber, WindowStart: other.HandoffWindowStart, WindowEnd: other.HandoffWindowEnd, Status: other.Status})
			}
		}
		sort.Slice(c.WindowConflicts, func(i, j int) bool {
			if c.WindowConflicts[i].WindowStart.Equal(c.WindowConflicts[j].WindowStart) {
				if c.WindowConflicts[i].CaseNumber == c.WindowConflicts[j].CaseNumber {
					return c.WindowConflicts[i].CaseID < c.WindowConflicts[j].CaseID
				}
				return c.WindowConflicts[i].CaseNumber < c.WindowConflicts[j].CaseNumber
			}
			return c.WindowConflicts[i].WindowStart.Before(c.WindowConflicts[j].WindowStart)
		})
		c.WindowConflictDigest = domain.WindowConflictDigest(c.WindowConflicts)
		out = append(out, c)
		counts[string(c.Status)]++
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].HandoffWindowStart.Equal(out[j].HandoffWindowStart) {
			if out[i].CaseNumber == out[j].CaseNumber {
				return out[i].ID < out[j].ID
			}
			return out[i].CaseNumber < out[j].CaseNumber
		}
		return out[i].HandoffWindowStart.Before(out[j].HandoffWindowStart)
	})
	var latest time.Time
	for _, c := range out {
		if c.UpdatedAt.After(latest) {
			latest = c.UpdatedAt
		}
	}
	total := len(out)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	items := out[start:end]
	hasNext := end < total
	next := 0
	if hasNext {
		next = page + 1
	}
	return ListResult{Items: items, Counts: counts, Total: total, LatestUpdatedAt: latest, Page: page, PageSize: pageSize, HasNext: hasNext, NextPage: next}, nil
}
func (s *Service) RegisterContainer(id string, v int, x domain.SampleContainer) (*domain.ColdChainCase, error) {
	return s.with(id, v, "container", func(c *domain.ColdChainCase) error { return c.RegisterContainer(x) })
}
func (s *Service) RegisterProbe(id string, v int, x domain.TemperatureProbe) (*domain.ColdChainCase, error) {
	return s.with(id, v, "probe", func(c *domain.ColdChainCase) error { return c.RegisterProbe(x) })
}
func (s *Service) RegisterBasics(id string, v int, cs []domain.SampleContainer, ps []domain.TemperatureProbe) (*domain.ColdChainCase, error) {
	return s.with(id, v, "basics", func(c *domain.ColdChainCase) error { return c.RegisterBasics(cs, ps) })
}
func (s *Service) AddRevision(id string, v int, x domain.TransportEvidenceRevision) (*domain.ColdChainCase, error) {
	return s.with(id, v, "evidence", func(c *domain.ColdChainCase) error { return c.AddRevision(x) })
}
func (s *Service) AddRevisionWithFingerprint(id string, v int, x domain.TransportEvidenceRevision, fingerprint string) (*domain.ColdChainCase, error) {
	m := s.lock(id)
	m.Lock()
	defer m.Unlock()
	c, err := s.store.Get(id)
	if err != nil {
		return nil, err
	}
	if fingerprint != "" {
		current := domain.EvidenceFingerprint(c.Version, []domain.TransportEvidenceRevision{x})
		if current != fingerprint {
			return nil, &domain.EvidencePrecheckConflictError{ProvidedFingerprint: fingerprint, CurrentFingerprint: current, CurrentVersion: c.Version}
		}
	}
	if v > 0 && c.Version != v {
		return nil, domain.ErrConflict
	}
	if err = c.AddRevision(x); err != nil {
		return nil, err
	}
	c.RecordAudit("evidence", "system", "evidence")
	if err = s.store.Save(c, "evidence"); err != nil {
		return nil, err
	}
	return c, nil
}
func (s *Service) AddRevisions(id string, v int, xs []domain.TransportEvidenceRevision) (*domain.ColdChainCase, error) {
	for i, x := range xs {
		if len(x.Readings) < 3 || !x.Readings[0].At.Equal(x.SegmentStart) || !x.Readings[len(x.Readings)-1].At.Equal(x.SegmentEnd) {
			return nil, fmt.Errorf("%w:证据[%d]必须包含起止和中间读数", domain.ErrInvalid, i)
		}
	}
	return s.with(id, v, "evidence-batch", func(c *domain.ColdChainCase) error { return c.AddRevisions(xs) })
}
func (s *Service) Submit(id string, v int) (*domain.ColdChainCase, error) {
	return s.with(id, v, "submit", func(c *domain.ColdChainCase) error { return c.SubmitReview() })
}
func (s *Service) Decide(id string, v int, finding, decision, note, by string) (*domain.ColdChainCase, error) {
	return s.with(id, v, "decision", func(c *domain.ColdChainCase) error { return c.DecideFinding(finding, decision, note, by) })
}

type FindingDecision struct{ FindingID, Decision, Note, Reviewer string }

func (s *Service) DecideBatch(id string, v int, ds []FindingDecision) (*domain.ColdChainCase, error) {
	return s.with(id, v, "decisions", func(c *domain.ColdChainCase) error {
		if len(ds) == 0 {
			return domain.ErrInvalid
		}
		clone := domain.Clone(c)
		seen := map[string]bool{}
		for i, d := range ds {
			if seen[d.FindingID] {
				return fmt.Errorf("%w:第%d项重复发现", domain.ErrInvalid, i+1)
			}
			seen[d.FindingID] = true
			if err := clone.DecideFinding(d.FindingID, d.Decision, d.Note, d.Reviewer); err != nil {
				return fmt.Errorf("%w:第%d项:%v", domain.ErrInvalid, i+1, err)
			}
		}
		clone.Version = c.Version + 1
		clone.UpdatedAt = time.Now().UTC()
		*c = *clone
		return nil
	})
}
func (s *Service) Return(id string, v int, note, by string) (*domain.ColdChainCase, error) {
	return s.with(id, v, "return", func(c *domain.ColdChainCase) error { return c.ReturnForCorrection(note, by) })
}
func (s *Service) ReturnWithTasks(id string, v int, note, by string, tasks []domain.RemediationTask) (*domain.ColdChainCase, error) {
	return s.with(id, v, "return", func(c *domain.ColdChainCase) error { return c.ReturnForCorrectionWithTasks(note, by, tasks) })
}
func (s *Service) ReviewConflicts(id string, v int, items []domain.WindowConflict, digest, decision, reviewer string) (*domain.ColdChainCase, error) {
	s.createMu.Lock()
	defer s.createMu.Unlock()
	m := s.lock(id)
	m.Lock()
	defer m.Unlock()
	c, err := s.store.Get(id)
	if err != nil {
		return nil, err
	}
	if v > 0 && c.Version != v {
		return nil, domain.ErrConflict
	}
	s.Enrich(c, time.Now().UTC())
	if digest == "" && len(items) > 0 {
		digest = domain.WindowConflictDigest(items)
	}
	if digest == "" {
		digest = c.WindowConflictDigest
	}
	if err = c.ReviewWindowConflicts(c.WindowConflicts, digest, decision, reviewer, time.Now().UTC()); err != nil {
		return nil, err
	}
	c.RecordAudit("conflict-review", reviewer, decision+":"+c.WindowConflictDigest)
	if err = s.store.Save(c, "conflict-review"); err != nil {
		return nil, err
	}
	return c, nil
}
func (s *Service) AddRevisionsWithFingerprint(id string, v int, xs []domain.TransportEvidenceRevision, fingerprint string) (*domain.ColdChainCase, error) {
	if fingerprint == "" {
		return s.AddRevisions(id, v, xs)
	}
	m := s.lock(id)
	m.Lock()
	defer m.Unlock()
	c, err := s.store.Get(id)
	if err != nil {
		return nil, err
	}
	current := domain.EvidenceFingerprint(c.Version, xs)
	if current != fingerprint {
		return nil, &domain.EvidencePrecheckConflictError{ProvidedFingerprint: fingerprint, CurrentFingerprint: current, CurrentVersion: c.Version}
	}
	if v > 0 && c.Version != v {
		return nil, domain.ErrConflict
	}
	for i, x := range xs {
		if len(x.Readings) < 3 || !x.Readings[0].At.Equal(x.SegmentStart) || !x.Readings[len(x.Readings)-1].At.Equal(x.SegmentEnd) {
			return nil, fmt.Errorf("%w:证据[%d]必须包含起止和中间读数", domain.ErrInvalid, i)
		}
	}
	if err = c.AddRevisions(xs); err != nil {
		return nil, err
	}
	c.RecordAudit("evidence-batch", "system", "evidence-batch")
	if err = s.store.Save(c, "evidence-batch"); err != nil {
		return nil, err
	}
	return c, nil
}
func (s *Service) ReplaceProbe(id string, v int, oldID string, probe domain.TemperatureProbe, at time.Time) (*domain.ColdChainCase, error) {
	return s.with(id, v, "probe-replace", func(c *domain.ColdChainCase) error { return c.ReplaceProbe(oldID, probe, at) })
}
func (s *Service) PrecheckEvidence(id string, v int, revisions []domain.TransportEvidenceRevision) (domain.PrecheckResult, error) {
	m := s.lock(id)
	m.Lock()
	defer m.Unlock()
	c, err := s.store.Get(id)
	if err != nil {
		return domain.PrecheckResult{}, err
	}
	if v > 0 && c.Version != v {
		return domain.PrecheckResult{}, domain.ErrConflict
	}
	return domain.PrecheckEvidence(c, revisions)
}
func (s *Service) Approve(id string, v int, by string) (*domain.ColdChainCase, error) {
	return s.with(id, v, "approve", func(c *domain.ColdChainCase) error { return c.Approve(by) })
}
func (s *Service) ApproveWithFingerprint(id string, v int, by, fingerprint string) (*domain.ColdChainCase, error) {
	return s.with(id, v, "approve", func(c *domain.ColdChainCase) error { return c.ApproveWithFingerprint(by, fingerprint) })
}
func (s *Service) Issue(id string, v int, by string) (*domain.ReleaseCredential, error) {
	m := s.lock(id)
	m.Lock()
	defer m.Unlock()
	c, e := s.store.Get(id)
	if e != nil {
		return nil, e
	}
	if v > 0 && c.Version != v {
		return nil, domain.ErrConflict
	}
	wasReleased := c.Status == domain.StatusReleased && c.Credential != nil
	cr, e := c.IssueCredential(by, time.Now().UTC())
	if e != nil {
		return nil, e
	}
	if wasReleased {
		return cr, nil
	}
	c.RecordAudit("release", by, "credential issued")
	if e = s.store.Save(c, "release"); e != nil {
		return nil, e
	}
	return cr, nil
}
func (s *Service) VerifyCredential(id string) (bool, *domain.ReleaseCredential, error) {
	result, e := s.VerifyCredentialDetailed(id)
	if e != nil {
		return false, nil, e
	}
	return result.Valid, result.Credential, nil
}

type CredentialCheck struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Code   string `json:"code,omitempty"`
}
type CredentialVerification struct {
	Valid      bool                      `json:"valid"`
	Credential *domain.ReleaseCredential `json:"credential"`
	Checks     []CredentialCheck         `json:"checks"`
	CaseStatus domain.CaseStatus         `json:"caseStatus"`
}

type CredentialBatchItem struct {
	Input       string                    `json:"input"`
	Valid       bool                      `json:"valid"`
	Code        string                    `json:"code,omitempty"`
	Checks      []CredentialCheck         `json:"checks"`
	Credential  *domain.ReleaseCredential `json:"credential"`
	CaseStatus  domain.CaseStatus         `json:"caseStatus,omitempty"`
	CheckDigest string                    `json:"checkDigest"`
}

type CredentialBatchSummary struct {
	Valid       int `json:"valid"`
	Invalid     int `json:"invalid"`
	NotReleased int `json:"notReleased"`
	NotFound    int `json:"notFound"`
}

type CredentialBatchResult struct {
	Items       []CredentialBatchItem  `json:"items"`
	Summary     CredentialBatchSummary `json:"summary"`
	BatchID     string                 `json:"batchId,omitempty"`
	InputDigest string                 `json:"inputDigest,omitempty"`
}

func (s *Service) VerifyCredentials(inputs []string) CredentialBatchResult {
	out := CredentialBatchResult{Items: make([]CredentialBatchItem, 0, len(inputs))}
	for _, input := range inputs {
		item := CredentialBatchItem{Input: input}
		result, err := s.VerifyCredentialDetailed(input)
		if errors.Is(err, domain.ErrNotFound) {
			item.Code = "CREDENTIAL_NOT_FOUND"
			item.Checks = missingCredentialChecks("CREDENTIAL_NOT_FOUND")
			out.Summary.NotFound++
		} else if err != nil {
			item.Code = "CREDENTIAL_INVALID"
			out.Summary.Invalid++
		} else {
			item.Valid, item.Checks, item.Credential, item.CaseStatus = result.Valid, result.Checks, result.Credential, result.CaseStatus
			item.Code = firstFailedCheck(result.Checks)
			switch {
			case result.Valid:
				out.Summary.Valid++
			case result.Credential == nil || result.CaseStatus != domain.StatusReleased:
				item.Code = "CASE_NOT_RELEASED"
				out.Summary.NotReleased++
			default:
				out.Summary.Invalid++
			}
		}
		item.CheckDigest = credentialCheckDigest(item)
		out.Items = append(out.Items, item)
	}
	return out
}

func credentialInputsDigest(inputs []string) string { return domain.Digest(inputs) }
func credentialCheckDigest(item CredentialBatchItem) string {
	parts := []string{item.Input, item.Code, fmt.Sprintf("%t", item.Valid), string(item.CaseStatus)}
	for _, check := range item.Checks {
		parts = append(parts, check.Name+"|"+fmt.Sprintf("%t", check.Passed)+"|"+check.Code)
	}
	return domain.Digest(parts)
}
func (s *Service) VerifyCredentialsRecorded(inputs []string, key string) (CredentialBatchResult, error) {
	s.credentialMu.Lock()
	defer s.credentialMu.Unlock()
	for i := range inputs {
		inputs[i] = strings.TrimSpace(inputs[i])
	}
	if len(inputs) == 0 || len(inputs) > 50 {
		return CredentialBatchResult{}, domain.ErrInvalid
	}
	digest := credentialInputsDigest(inputs)
	if key != "" {
		var cached CredentialBatchResult
		if s.replay("credential-batch:"+key, &cached) {
			if cached.InputDigest != digest {
				return CredentialBatchResult{}, domain.ErrConflict
			}
			return cached, nil
		}
	}
	out := s.VerifyCredentials(inputs)
	out.InputDigest = digest
	out.BatchID = "verification-" + digest[:16]
	if key != "" {
		s.remember("credential-batch:"+key, out)
	}
	return out, nil
}

func (s *Service) RecordCredentialReview(input, batchKey, checkDigest, operator, conclusion, note string) (domain.CredentialVerificationRecord, error) {
	s.credentialMu.Lock()
	defer s.credentialMu.Unlock()
	if strings.TrimSpace(input) == "" || strings.TrimSpace(checkDigest) == "" || strings.TrimSpace(operator) == "" || strings.TrimSpace(conclusion) == "" || strings.TrimSpace(note) == "" {
		return domain.CredentialVerificationRecord{}, domain.ErrInvalid
	}
	batch := s.VerifyCredentials([]string{strings.TrimSpace(input)})
	item := batch.Items[0]
	current := credentialCheckDigest(item)
	if item.Valid {
		return domain.CredentialVerificationRecord{}, fmt.Errorf("%w:有效凭据无需异常复核", domain.ErrInvalidState)
	}
	if checkDigest != "" && checkDigest != current {
		return domain.CredentialVerificationRecord{}, domain.ErrConflict
	}
	record := domain.CredentialVerificationRecord{ID: fmt.Sprintf("verification-review-%d", time.Now().UnixNano()), BatchKey: batchKey, CredentialInput: item.Input, FailureCode: item.Code, CheckDigest: current, Operator: strings.TrimSpace(operator), Conclusion: strings.TrimSpace(conclusion), Note: strings.TrimSpace(note), CreatedAt: time.Now().UTC()}
	if item.Credential != nil {
		record.CredentialNumber = item.Credential.CredentialNumber
	}
	var err error
	if record, err = s.store.SaveVerification(record, "credential-review"); err != nil {
		return domain.CredentialVerificationRecord{}, err
	}
	return record, nil
}

func (s *Service) RecordCredentialReviewContext(ctx context.Context, input, batchKey, checkDigest, operator, conclusion, note string) (domain.CredentialVerificationRecord, error) {
	if err := ctx.Err(); err != nil {
		return domain.CredentialVerificationRecord{}, err
	}
	record, err := s.RecordCredentialReview(input, batchKey, checkDigest, operator, conclusion, note)
	if err != nil {
		return domain.CredentialVerificationRecord{}, err
	}
	if err = ctx.Err(); err != nil {
		return domain.CredentialVerificationRecord{}, err
	}
	return record, nil
}
func (s *Service) CredentialReviews(credential, failureCode string) []domain.CredentialVerificationRecord {
	all := s.store.VerificationRecords()
	out := make([]domain.CredentialVerificationRecord, 0)
	for _, item := range all {
		if credential != "" && item.CredentialInput != credential && item.CredentialNumber != credential || failureCode != "" && item.FailureCode != failureCode {
			continue
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

func missingCredentialChecks(code string) []CredentialCheck {
	return []CredentialCheck{{Name: "credentialNumber", Code: code}, {Name: "manifestDigest", Code: code}, {Name: "manifestEntries", Code: code}, {Name: "integrityVersion", Code: code}, {Name: "caseReleased", Code: code}, {Name: "auditChain", Code: code}}
}

func firstFailedCheck(checks []CredentialCheck) string {
	for _, check := range checks {
		if !check.Passed && check.Code != "" {
			return check.Code
		}
	}
	return ""
}

func (s *Service) VerifyCredentialDetailed(id string) (CredentialVerification, error) {
	c, e := s.store.Get(id)
	if e != nil {
		c, e = s.store.FindCredential(id)
	}
	if e != nil {
		return CredentialVerification{}, e
	}
	if c.Credential == nil {
		checks := []CredentialCheck{
			{Name: "credentialNumber", Code: "CREDENTIAL_NOT_FOUND"},
			{Name: "manifestDigest", Code: "CREDENTIAL_NOT_FOUND"},
			{Name: "manifestEntries", Code: "CREDENTIAL_NOT_FOUND"},
			{Name: "integrityVersion", Code: "CREDENTIAL_NOT_FOUND"},
			{Name: "caseReleased", Passed: false, Code: "CASE_NOT_RELEASED"},
			{Name: "auditChain", Passed: c.AuditValid(), Code: "AUDIT_CHAIN_INVALID"},
		}
		if checks[len(checks)-1].Passed {
			checks[len(checks)-1].Code = ""
		}
		return CredentialVerification{Valid: false, Checks: checks, CaseStatus: c.Status}, nil
	}
	cr := c.Credential
	current := domain.Manifest(c)
	checks := []CredentialCheck{
		{Name: "credentialNumber", Passed: cr.CredentialNumber != "" && cr.CredentialNumber == "REL-"+c.CaseNumber, Code: "CREDENTIAL_NUMBER_MISMATCH"},
		{Name: "manifestDigest", Passed: domain.Digest(current) == cr.ManifestDigest, Code: "MANIFEST_DIGEST_MISMATCH"},
		{Name: "manifestEntries", Passed: strings.Join(current, "\n") == strings.Join(cr.ManifestEntries, "\n"), Code: "MANIFEST_ENTRIES_MISMATCH"},
		{Name: "integrityVersion", Passed: cr.IntegrityVersion == "v1", Code: "INTEGRITY_VERSION_UNSUPPORTED"},
		{Name: "caseReleased", Passed: c.Status == domain.StatusReleased, Code: "CASE_NOT_RELEASED"},
		{Name: "auditChain", Passed: c.AuditValid(), Code: "AUDIT_CHAIN_INVALID"},
	}
	ok := true
	for i := range checks {
		if !checks[i].Passed {
			ok = false
		} else {
			checks[i].Code = ""
		}
	}
	return CredentialVerification{Valid: ok, Credential: cr, Checks: checks, CaseStatus: c.Status}, nil
}
func (s *Service) Manifest(id string) ([]string, string, error) {
	c, e := s.Get(id)
	if e != nil {
		return nil, "", e
	}
	if c.Status != domain.StatusApproved && c.Status != domain.StatusReleased {
		return nil, "", domain.ErrInvalidState
	}
	entries, d := c.ManifestPreview()
	return entries, d, nil
}
func (s *Service) Audit(id string, action string) ([]domain.AuditEvent, error) {
	c, e := s.Get(id)
	if e != nil {
		return nil, e
	}
	out := make([]domain.AuditEvent, 0, len(c.Audit))
	for _, a := range c.Audit {
		if action == "" || a.Action == action {
			a.Detail = ""
			out = append(out, a)
		}
	}
	return out, nil
}
