package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

type CaseStatus string

const (
	StatusDraft      CaseStatus = "草拟"
	StatusCollecting CaseStatus = "证据采集中"
	StatusReview     CaseStatus = "待审核"
	StatusReturned   CaseStatus = "已退回"
	StatusResubmit   CaseStatus = "待复审"
	StatusApproved   CaseStatus = "已批准"
	StatusReleased   CaseStatus = "已放行"
)

type ColdChainCase struct {
	ID                     string                      `json:"id"`
	CaseNumber             string                      `json:"caseNumber"`
	SenderName             string                      `json:"senderName"`
	ReceiverName           string                      `json:"receiverName"`
	HandoffWindowStart     time.Time                   `json:"handoffWindowStart"`
	HandoffWindowEnd       time.Time                   `json:"handoffWindowEnd"`
	Status                 CaseStatus                  `json:"status"`
	Version                int                         `json:"version"`
	CreatedAt              time.Time                   `json:"createdAt"`
	UpdatedAt              time.Time                   `json:"updatedAt"`
	Containers             []SampleContainer           `json:"containers"`
	Probes                 []TemperatureProbe          `json:"probes"`
	Revisions              []TransportEvidenceRevision `json:"revisions"`
	Findings               []RiskFinding               `json:"findings"`
	Reviews                []ReviewRecord              `json:"reviews"`
	Credential             *ReleaseCredential          `json:"credential,omitempty"`
	Audit                  []AuditEvent                `json:"audit"`
	ReviewBaseline         []RiskFinding               `json:"reviewBaseline,omitempty"`
	ReviewFingerprint      string                      `json:"reviewFingerprint,omitempty"`
	ReviewBaselineRevision int                         `json:"reviewBaselineRevision,omitempty"`
	WindowPhase            string                      `json:"windowPhase,omitempty"`
	RemainingMinutes       int64                       `json:"remainingMinutes,omitempty"`
	WindowConflicts        []WindowConflict            `json:"windowConflicts,omitempty"`
	WindowConflictDigest   string                      `json:"windowConflictDigest,omitempty"`
	ProbeCoverageLedger    []ProbeCalibrationCoverage  `json:"probeCoverageLedger,omitempty"`
	ConflictReviews        []ConflictReview            `json:"conflictReviews,omitempty"`
	RemediationTasks       []RemediationTask           `json:"remediationTasks,omitempty"`
}

type WindowConflict struct {
	CaseID      string     `json:"caseId"`
	CaseNumber  string     `json:"caseNumber"`
	WindowStart time.Time  `json:"windowStart"`
	WindowEnd   time.Time  `json:"windowEnd"`
	Status      CaseStatus `json:"status"`
}

type ConflictReview struct {
	ConflictCaseIDs []string  `json:"conflictCaseIds"`
	ConflictDigest  string    `json:"conflictDigest"`
	Decision        string    `json:"decision"`
	Reviewer        string    `json:"reviewer"`
	ReviewedAt      time.Time `json:"reviewedAt"`
	Version         int       `json:"version"`
}

type ProbeCalibrationCoverage struct {
	ProbeID            string    `json:"probeId"`
	SerialNumber       string    `json:"serialNumber"`
	CertificateRef     string    `json:"certificateRef"`
	CoverageStart      time.Time `json:"coverageStart"`
	CoverageEnd        time.Time `json:"coverageEnd"`
	HandoffWindowStart time.Time `json:"handoffWindowStart"`
	HandoffWindowEnd   time.Time `json:"handoffWindowEnd"`
	RemainingHours     float64   `json:"remainingHours"`
	Status             string    `json:"status"`
	MissingBoundaries  []string  `json:"missingBoundaries"`
}

type SampleContainer struct {
	ID              string    `json:"id"`
	CaseID          string    `json:"caseId"`
	ContainerCode   string    `json:"containerCode"`
	SealCode        string    `json:"sealCode"`
	SampleCategory  string    `json:"sampleCategory"`
	MinTemperatureC float64   `json:"minTemperatureC"`
	MaxTemperatureC float64   `json:"maxTemperatureC"`
	RegisteredAt    time.Time `json:"registeredAt"`
}
type TemperatureProbe struct {
	ID                   string     `json:"id"`
	CaseID               string     `json:"caseId"`
	SerialNumber         string     `json:"serialNumber"`
	CertificateRef       string     `json:"certificateRef"`
	CalibratedAt         time.Time  `json:"calibratedAt"`
	CalibrationExpiresAt time.Time  `json:"calibrationExpiresAt"`
	AccuracyC            float64    `json:"accuracyC"`
	ReplacesProbeID      string     `json:"replacesProbeId,omitempty"`
	ReplacementAt        *time.Time `json:"replacementAt,omitempty"`
}
type TemperatureReading struct {
	At           time.Time `json:"at"`
	TemperatureC float64   `json:"temperatureC"`
}
type TransportEvidenceRevision struct {
	ID                   string               `json:"id"`
	CaseID               string               `json:"caseId"`
	RevisionNumber       int                  `json:"revisionNumber"`
	SegmentStart         time.Time            `json:"segmentStart"`
	SegmentEnd           time.Time            `json:"segmentEnd"`
	ProbeID              string               `json:"probeId"`
	Readings             []TemperatureReading `json:"readings"`
	SealObservation      string               `json:"sealObservation"`
	RemediationNote      string               `json:"remediationNote"`
	RemediatesFindingIDs []string             `json:"remediatesFindingIDs,omitempty"`
	SubmittedAt          time.Time            `json:"submittedAt"`
}
type RiskFinding struct {
	ID              string              `json:"id"`
	CaseID          string              `json:"caseId"`
	RevisionNumber  int                 `json:"revisionNumber"`
	Kind            string              `json:"kind"`
	Severity        string              `json:"severity"`
	EvidenceRefs    []string            `json:"evidenceRefs"`
	DerivedReason   string              `json:"derivedReason"`
	Decision        string              `json:"decision"`
	DecisionNote    string              `json:"decisionNote"`
	DecidedBy       string              `json:"decidedBy"`
	DecidedAt       *time.Time          `json:"decidedAt,omitempty"`
	StartAt         *time.Time          `json:"startAt"`
	EndAt           *time.Time          `json:"endAt"`
	DurationSeconds int64               `json:"durationSeconds"`
	DeviationC      float64             `json:"deviationC"`
	MaxDeviationC   float64             `json:"maxDeviationC,omitempty"`
	ContextID       string              `json:"contextId,omitempty"`
	Context         *RiskReadingContext `json:"context,omitempty"`
}

type RiskReadingContext struct {
	TriggerReading  *TemperatureReading `json:"triggerReading,omitempty"`
	PreviousReading *TemperatureReading `json:"previousReading,omitempty"`
	NextReading     *TemperatureReading `json:"nextReading,omitempty"`
	ThresholdMinC   *float64            `json:"thresholdMinC,omitempty"`
	ThresholdMaxC   *float64            `json:"thresholdMaxC,omitempty"`
	DurationSeconds int64               `json:"durationSeconds"`
	MaxDeviationC   float64             `json:"maxDeviationC"`
}
type ReviewRecord struct {
	Action   string    `json:"action"`
	Note     string    `json:"note"`
	Reviewer string    `json:"reviewer"`
	At       time.Time `json:"at"`
	Version  int       `json:"version"`
}
type ReleaseCredential struct {
	ID               string    `json:"id"`
	CaseID           string    `json:"caseId"`
	CredentialNumber string    `json:"credentialNumber"`
	ManifestDigest   string    `json:"manifestDigest"`
	ApprovedBy       string    `json:"approvedBy"`
	ManifestEntries  []string  `json:"manifestEntries"`
	IssuedAt         time.Time `json:"issuedAt"`
	IntegrityVersion string    `json:"integrityVersion"`
}
type CredentialVerificationRecord struct {
	Sequence         int64     `json:"sequence"`
	PreviousHash     string    `json:"previousHash"`
	Hash             string    `json:"hash"`
	ID               string    `json:"id"`
	BatchKey         string    `json:"batchKey"`
	CredentialInput  string    `json:"credentialInput"`
	CredentialNumber string    `json:"credentialNumber,omitempty"`
	FailureCode      string    `json:"failureCode"`
	CheckDigest      string    `json:"checkDigest"`
	Operator         string    `json:"operator"`
	Conclusion       string    `json:"conclusion"`
	Note             string    `json:"note"`
	CreatedAt        time.Time `json:"createdAt"`
}
type AuditEvent struct {
	Sequence     int64     `json:"sequence"`
	Action       string    `json:"action"`
	Actor        string    `json:"actor"`
	Detail       string    `json:"detail"`
	Hash         string    `json:"hash"`
	PreviousHash string    `json:"previousHash"`
	At           time.Time `json:"at"`
}

type CaseFilter struct {
	Status, CaseNumber, Sender, Receiver string
	HandoffStart, HandoffEnd             *time.Time
	WindowPhase                          string
	DueWithinMinutes                     *int
	Now                                  time.Time
	Page                                 int
	PageSize                             int
}

func (c *ColdChainCase) RegisterBasics(containers []SampleContainer, probes []TemperatureProbe) error {
	if len(containers) == 0 && len(probes) == 0 {
		return ErrInvalid
	}
	clone := *c
	clone.Containers = append([]SampleContainer(nil), c.Containers...)
	clone.Probes = append([]TemperatureProbe(nil), c.Probes...)
	ids := map[string]bool{}
	for _, old := range clone.Containers {
		if strings.TrimSpace(old.ID) != "" {
			ids[strings.ToLower(strings.TrimSpace(old.ID))] = true
		}
	}
	for _, old := range clone.Probes {
		if strings.TrimSpace(old.ID) != "" {
			ids[strings.ToLower(strings.TrimSpace(old.ID))] = true
		}
	}
	for i, v := range containers {
		key := strings.ToLower(strings.TrimSpace(v.ID))
		if key == "" || ids[key] {
			return fmt.Errorf("%w:容器[%d].id重复或为空", ErrInvalid, i)
		}
		ids[key] = true
	}
	for i, v := range probes {
		key := strings.ToLower(strings.TrimSpace(v.ID))
		if key == "" || ids[key] {
			return fmt.Errorf("%w:探头[%d].id重复或为空", ErrInvalid, i)
		}
		ids[key] = true
	}
	// Validate the complete batch before mutating the clone. Categories use one
	// range per case so a partial registration can never enter the workflow.
	ranges := map[string][2]float64{}
	for i, v := range containers {
		if v.MinTemperatureC < -80 || v.MaxTemperatureC > 40 || v.MinTemperatureC >= v.MaxTemperatureC {
			return fmt.Errorf("%w:容器[%d].temperatureRange无效", ErrInvalid, i)
		}
		key := strings.ToLower(strings.TrimSpace(v.SampleCategory))
		if old, ok := ranges[key]; ok && (old[0] != v.MinTemperatureC || old[1] != v.MaxTemperatureC) {
			return fmt.Errorf("%w:容器[%d].temperatureRange与同类样本不一致", ErrInvalid, i)
		}
		ranges[key] = [2]float64{v.MinTemperatureC, v.MaxTemperatureC}
	}
	for _, v := range clone.Containers {
		key := strings.ToLower(strings.TrimSpace(v.SampleCategory))
		if old, ok := ranges[key]; ok && (old[0] != v.MinTemperatureC || old[1] != v.MaxTemperatureC) {
			return fmt.Errorf("%w:同类样本温度范围不一致", ErrInvalid)
		}
		ranges[key] = [2]float64{v.MinTemperatureC, v.MaxTemperatureC}
	}
	strictCertificate := false
	for _, v := range append(append([]SampleContainer(nil), clone.Containers...), containers...) {
		if strings.TrimSpace(v.SampleCategory) != "" {
			strictCertificate = true
		}
	}
	for i, v := range probes {
		if strictCertificate && strings.TrimSpace(v.CertificateRef) == "" {
			return fmt.Errorf("%w:探头[%d].certificateRef不能为空", ErrInvalid, i)
		}
		width := -1.0
		for _, co := range append(append([]SampleContainer(nil), clone.Containers...), containers...) {
			if width < 0 || co.MaxTemperatureC-co.MinTemperatureC < width {
				width = co.MaxTemperatureC - co.MinTemperatureC
			}
		}
		if width >= 0 && v.AccuracyC > width/2 {
			return fmt.Errorf("%w:探头[%d].accuracyC超过温度范围宽度一半", ErrInvalid, i)
		}
	}
	for i, v := range containers {
		if err := clone.RegisterContainer(v); err != nil {
			return fmt.Errorf("%w:第%d项:%v", ErrInvalid, i+1, err)
		}
	}
	for i, v := range probes {
		if err := clone.RegisterProbe(v); err != nil {
			return fmt.Errorf("%w:第%d项:%v", ErrInvalid, i+1, err)
		}
	}
	if clone.Status == StatusDraft {
		clone.Status = StatusCollecting
	}
	clone.Version = c.Version + 1
	clone.UpdatedAt = time.Now().UTC()
	*c = clone
	return nil
}

func (c *ColdChainCase) AddRevisions(revisions []TransportEvidenceRevision) error {
	if len(revisions) == 0 {
		return ErrInvalid
	}
	revisions = append([]TransportEvidenceRevision(nil), revisions...)
	sort.SliceStable(revisions, func(i, j int) bool { return revisions[i].SegmentStart.Before(revisions[j].SegmentStart) })
	clone := *c
	clone.Revisions = append([]TransportEvidenceRevision(nil), c.Revisions...)
	for i, v := range revisions {
		if err := clone.AddRevision(v); err != nil {
			return fmt.Errorf("%w:第%d项:%v", ErrInvalid, i+1, err)
		}
	}
	clone.Version = c.Version + 1
	clone.UpdatedAt = time.Now().UTC()
	*c = clone
	return nil
}

func NewCase(id, number, sender, receiver string, start, end, now time.Time) (*ColdChainCase, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(number) == "" || strings.TrimSpace(sender) == "" || strings.TrimSpace(receiver) == "" || !end.After(start) {
		return nil, ErrInvalid
	}
	return &ColdChainCase{ID: id, CaseNumber: number, SenderName: sender, ReceiverName: receiver, HandoffWindowStart: start, HandoffWindowEnd: end, Status: StatusDraft, Version: 1, CreatedAt: now, UpdatedAt: now}, nil
}
func (c *ColdChainCase) touch() { c.Version++; c.UpdatedAt = time.Now().UTC() }
func (c *ColdChainCase) RegisterContainer(v SampleContainer) error {
	if c.Status == StatusReleased || c.Status == StatusApproved {
		return ErrImmutable
	}
	if strings.TrimSpace(v.ID) == "" || strings.TrimSpace(v.ContainerCode) == "" || strings.TrimSpace(v.SealCode) == "" || v.MinTemperatureC < -80 || v.MaxTemperatureC > 40 || v.MinTemperatureC >= v.MaxTemperatureC {
		return ErrInvalid
	}
	for _, old := range c.Containers {
		if strings.EqualFold(strings.TrimSpace(old.SampleCategory), strings.TrimSpace(v.SampleCategory)) && (old.MinTemperatureC != v.MinTemperatureC || old.MaxTemperatureC != v.MaxTemperatureC) {
			return fmt.Errorf("%w:同类样本温度范围不一致", ErrInvalid)
		}
	}
	for _, x := range c.Containers {
		if strings.EqualFold(x.ID, v.ID) || strings.EqualFold(x.ContainerCode, v.ContainerCode) || strings.EqualFold(x.SealCode, v.SealCode) {
			return fmt.Errorf("%w:容器或封签重复", ErrInvalid)
		}
	}
	for _, x := range c.Probes {
		if strings.EqualFold(x.ID, v.ID) {
			return fmt.Errorf("%w:实体ID重复", ErrInvalid)
		}
	}
	v.CaseID = c.ID
	if v.RegisteredAt.IsZero() {
		v.RegisteredAt = time.Now().UTC()
	}
	c.Containers = append(c.Containers, v)
	if c.Status == StatusDraft {
		c.Status = StatusCollecting
	}
	c.touch()
	return nil
}
func (c *ColdChainCase) RegisterProbe(v TemperatureProbe) error {
	if c.Status == StatusReleased || c.Status == StatusApproved {
		return ErrImmutable
	}
	if strings.TrimSpace(v.ID) == "" || strings.TrimSpace(v.SerialNumber) == "" || v.AccuracyC < 0 || !v.CalibrationExpiresAt.After(v.CalibratedAt) {
		return ErrInvalid
	}
	// Keep the historical single-probe API usable while batch registration
	// enforces the certificate reference as a hard requirement.
	if strings.TrimSpace(v.CertificateRef) == "" {
		v.CertificateRef = "legacy:auto"
	}
	if len(c.Containers) > 0 {
		width := c.Containers[0].MaxTemperatureC - c.Containers[0].MinTemperatureC
		for _, co := range c.Containers[1:] {
			if w := co.MaxTemperatureC - co.MinTemperatureC; w < width {
				width = w
			}
		}
		if v.AccuracyC > width/2 {
			return fmt.Errorf("%w:accuracyC超过容器温度范围宽度一半", ErrInvalid)
		}
	}
	if v.CalibratedAt.After(c.HandoffWindowStart) || v.CalibrationExpiresAt.Before(c.HandoffWindowEnd) {
		return fmt.Errorf("%w:校准未覆盖交接窗口", ErrInvalid)
	}
	for _, x := range c.Probes {
		if strings.EqualFold(x.ID, v.ID) || strings.EqualFold(x.SerialNumber, v.SerialNumber) {
			return fmt.Errorf("%w:探头重复", ErrInvalid)
		}
	}
	for _, x := range c.Containers {
		if strings.EqualFold(x.ID, v.ID) {
			return fmt.Errorf("%w:实体ID重复", ErrInvalid)
		}
	}
	v.CaseID = c.ID
	c.Probes = append(c.Probes, v)
	c.touch()
	return nil
}
func (c *ColdChainCase) AddRevision(v TransportEvidenceRevision) error {
	if c.Status != StatusCollecting && c.Status != StatusReturned && c.Status != StatusResubmit {
		return ErrInvalidState
	}
	if !v.SegmentEnd.After(v.SegmentStart) || len(v.Readings) == 0 {
		return ErrInvalid
	}
	if (c.Status == StatusReturned || c.Status == StatusResubmit) && len(c.Reviews) > 0 && strings.TrimSpace(v.RemediationNote) == "" {
		return fmt.Errorf("%w:退回补证必须填写整改说明", ErrInvalid)
	}
	if len(v.RemediatesFindingIDs) > 0 {
		baseline := make(map[string]bool, len(c.ReviewBaseline))
		for _, finding := range c.ReviewBaseline {
			baseline[finding.ID] = true
		}
		seen := map[string]bool{}
		for _, findingID := range v.RemediatesFindingIDs {
			if seen[findingID] || !baseline[findingID] {
				return fmt.Errorf("%w:remediatesFindingIDs包含重复或不存在的发现", ErrInvalid)
			}
			seen[findingID] = true
		}
	}
	var probe *TemperatureProbe
	for i := range c.Probes {
		if c.Probes[i].ID == v.ProbeID {
			probe = &c.Probes[i]
		}
	}
	if probe == nil {
		return fmt.Errorf("%w:探头未登记", ErrInvalid)
	}
	if probe.ReplacementAt != nil && v.SegmentStart.Before(*probe.ReplacementAt) {
		return fmt.Errorf("%w:新探头不能引用替换前证据", ErrInvalid)
	}
	for _, candidate := range c.Probes {
		if candidate.ReplacesProbeID == probe.ID && candidate.ReplacementAt != nil && v.SegmentEnd.After(*candidate.ReplacementAt) {
			return fmt.Errorf("%w:原探头不能引用替换后证据", ErrInvalid)
		}
	}
	if missing := missingCalibrationBoundaries(*probe, v.SegmentStart, v.SegmentEnd); len(missing) > 0 {
		return &CalibrationCoverageError{ProbeID: probe.ID, CertificateRef: probe.CertificateRef, MissingBoundary: missing}
	}
	if v.SegmentStart.Before(c.HandoffWindowStart) || v.SegmentEnd.After(c.HandoffWindowEnd) {
		return fmt.Errorf("%w:分段超出交接窗口", ErrInvalid)
	}
	seenReadingAt := map[int64]bool{}
	for _, reading := range v.Readings {
		key := reading.At.UnixNano()
		if seenReadingAt[key] {
			return fmt.Errorf("%w:读数时间重复", ErrInvalid)
		}
		seenReadingAt[key] = true
	}
	for _, x := range c.Revisions {
		if v.SegmentStart.Before(x.SegmentEnd) && x.SegmentStart.Before(v.SegmentEnd) {
			return fmt.Errorf("%w:运输分段重叠", ErrInvalid)
		}
	}
	for _, x := range c.Revisions {
		for _, old := range x.Readings {
			for _, cur := range v.Readings {
				if x.ProbeID == v.ProbeID && old.At.Equal(cur.At) {
					sharedBoundary := old.At.Equal(x.SegmentEnd) && cur.At.Equal(v.SegmentStart) && x.SegmentEnd.Equal(v.SegmentStart) || old.At.Equal(x.SegmentStart) && cur.At.Equal(v.SegmentEnd) && x.SegmentStart.Equal(v.SegmentEnd)
					if sharedBoundary {
						continue
					}
					return &EvidenceDuplicateError{RevisionNumber: x.RevisionNumber, EvidenceRef: fmt.Sprintf("%s@%s", x.ID, old.At.UTC().Format(time.RFC3339Nano))}
				}
			}
		}
	}
	for i := 1; i < len(v.Readings); i++ {
		if !v.Readings[i].At.After(v.Readings[i-1].At) {
			return fmt.Errorf("%w:读数未按时间排序", ErrInvalid)
		}
	}
	for _, r := range v.Readings {
		if r.At.Before(v.SegmentStart) || r.At.After(v.SegmentEnd) {
			return fmt.Errorf("%w:读数超出分段", ErrInvalid)
		}
	}
	if v.ID == "" {
		v.ID = fmt.Sprintf("%s-r%d", c.ID, len(c.Revisions)+1)
	}
	v.CaseID = c.ID
	v.RevisionNumber = len(c.Revisions) + 1
	v.SubmittedAt = time.Now().UTC()
	c.Revisions = append(c.Revisions, v)
	c.mergeFindings(FindingsForCase(c))
	c.UpdateRemediationTasks(time.Now().UTC())
	if c.Status == StatusReturned {
		c.Status = StatusResubmit
	}
	c.touch()
	return nil
}
func (c *ColdChainCase) SubmitReview() error {
	if c.Status != StatusCollecting && c.Status != StatusResubmit && c.Status != StatusReturned {
		return ErrInvalidState
	}
	if !c.EvidenceComplete() || !c.CoverageComplete() {
		return fmt.Errorf("%w:证据不完整", ErrInvalid)
	}
	if (c.Status == StatusReturned || c.Status == StatusResubmit) && c.ReviewBaselineRevision > 0 && len(c.Revisions) <= c.ReviewBaselineRevision {
		return fmt.Errorf("%w:退回后必须补充新证据", ErrInvalid)
	}
	if (c.Status == StatusReturned || c.Status == StatusResubmit) && len(RemediationClosureForCase(c).Uncovered) > 0 {
		return fmt.Errorf("%w:仍有未覆盖整改项", ErrInvalid)
	}
	if (c.Status == StatusReturned || c.Status == StatusResubmit) && len(c.RemediationTasks) > 0 {
		if err := c.ValidateRemediationGate(time.Now().UTC()); err != nil {
			return err
		}
	}
	if c.Status == StatusReturned || c.Status == StatusResubmit {
		c.Status = StatusResubmit
	} else {
		c.Status = StatusReview
	}
	c.mergeFindings(FindingsForCase(c))
	c.touch()
	return nil
}
func (c *ColdChainCase) DecideFinding(id, decision, note, by string) error {
	if c.Status != StatusReview && c.Status != StatusResubmit {
		return ErrInvalidState
	}
	if decision != "接受" && decision != "整改" || strings.TrimSpace(by) == "" || strings.TrimSpace(note) == "" {
		return ErrInvalid
	}
	for i := range c.Findings {
		if c.Findings[i].ID == id {
			now := time.Now().UTC()
			c.Findings[i].Decision = decision
			c.Findings[i].DecisionNote = note
			c.Findings[i].DecidedBy = by
			c.Findings[i].DecidedAt = &now
			c.touch()
			return nil
		}
	}
	return ErrNotFound
}
func (c *ColdChainCase) ReturnForCorrection(note, by string) error {
	if c.Status != StatusReview && c.Status != StatusResubmit {
		return ErrInvalidState
	}
	if strings.TrimSpace(note) == "" || strings.TrimSpace(by) == "" {
		return ErrInvalid
	}
	c.Status = StatusReturned
	c.ReviewBaseline = append([]RiskFinding(nil), c.Findings...)
	c.ReviewFingerprint = FindingsFingerprint(c.Findings)
	c.ReviewBaselineRevision = len(c.Revisions)
	c.RemediationTasks = nil
	c.Reviews = append(c.Reviews, ReviewRecord{"退回", note, by, time.Now().UTC(), c.Version})
	c.touch()
	return nil
}

func (c *ColdChainCase) ReturnForCorrectionWithTasks(note, by string, tasks []RemediationTask) error {
	if strings.TrimSpace(note) == "" || strings.TrimSpace(by) == "" {
		return ErrInvalid
	}
	clone := Clone(c)
	if err := clone.SetRemediationTasks(tasks, time.Now().UTC()); err != nil {
		return err
	}
	clone.Status = StatusReturned
	clone.Reviews = append(clone.Reviews, ReviewRecord{"退回", note, by, time.Now().UTC(), c.Version})
	// SetRemediationTasks already advances the version once for the atomic return.
	*c = *clone
	return nil
}
func (c *ColdChainCase) Approve(by string) error { return c.ApproveWithFingerprint(by, "") }
func (c *ColdChainCase) ApproveWithFingerprint(by, fingerprint string) error {
	if c.Status != StatusReview && c.Status != StatusResubmit {
		return ErrInvalidState
	}
	if strings.TrimSpace(by) == "" || !c.EvidenceComplete() || !c.CoverageComplete() || !c.AllFindingsDecided() {
		return ErrRiskUnresolved
	}
	if c.Status == StatusResubmit {
		if strings.TrimSpace(fingerprint) == "" || fingerprint != FindingsFingerprint(c.Findings) {
			return &RiskFingerprintConflictError{ProvidedFingerprint: fingerprint, CurrentFingerprint: FindingsFingerprint(c.Findings), CurrentVersion: c.Version}
		}
	}
	for _, f := range c.Findings {
		if f.Decision != "接受" {
			return ErrRiskUnresolved
		}
	}
	c.Status = StatusApproved
	c.Reviews = append(c.Reviews, ReviewRecord{"批准", "", by, time.Now().UTC(), c.Version})
	c.touch()
	return nil
}

func (c *ColdChainCase) mergeFindings(next []RiskFinding) {
	old := map[string]RiskFinding{}
	for _, f := range c.Findings {
		old[f.ID] = f
	}
	for i := range next {
		if prev, ok := old[next[i].ID]; ok && prev.Severity == next[i].Severity && prev.DerivedReason == next[i].DerivedReason && equalStrings(prev.EvidenceRefs, next[i].EvidenceRefs) {
			next[i].Decision, next[i].DecisionNote, next[i].DecidedBy, next[i].DecidedAt = prev.Decision, prev.DecisionNote, prev.DecidedBy, prev.DecidedAt
		}
	}
	c.Findings = next
}
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func FindingsFingerprint(fs []RiskFinding) string {
	parts := make([]string, 0, len(fs))
	for _, f := range fs {
		parts = append(parts, fmt.Sprintf("%s|%d|%s|%s|%s|%s|%d|%.3f|%s", f.ID, f.RevisionNumber, f.Kind, f.Severity, strings.Join(f.EvidenceRefs, ","), f.DerivedReason, f.DurationSeconds, f.DeviationC, f.ContextID))
	}
	sort.Strings(parts)
	return Digest(parts)
}
func (c *ColdChainCase) IssueCredential(by string, now time.Time) (*ReleaseCredential, error) {
	if c.Status == StatusReleased && c.Credential != nil {
		return c.Credential, nil
	}
	if c.Status != StatusApproved {
		return nil, ErrInvalidState
	}
	entries := Manifest(c)
	digest := Digest(entries)
	cr := &ReleaseCredential{ID: c.ID + "-credential", CaseID: c.ID, CredentialNumber: "REL-" + c.CaseNumber, ManifestDigest: digest, ManifestEntries: entries, ApprovedBy: by, IssuedAt: now, IntegrityVersion: "v1"}
	c.Credential = cr
	c.Status = StatusReleased
	c.touch()
	return cr, nil
}

func (c *ColdChainCase) ManifestPreview() ([]string, string) {
	entries := Manifest(c)
	return entries, Digest(entries)
}
func Manifest(c *ColdChainCase) []string {
	var out []string
	for _, v := range c.Containers {
		out = append(out, fmt.Sprintf("container:%s:%s:%s:%s:%.2f:%.2f", v.ID, v.ContainerCode, v.SealCode, v.SampleCategory, v.MinTemperatureC, v.MaxTemperatureC))
	}
	for _, p := range c.Probes {
		out = append(out, fmt.Sprintf("probe:%s:%s:%s:%s:%s:%s:%.3f", p.ID, p.SerialNumber, p.CertificateRef, p.CalibratedAt.UTC().Format(time.RFC3339Nano), p.CalibrationExpiresAt.UTC().Format(time.RFC3339Nano), c.ID, p.AccuracyC))
	}
	for _, r := range c.Revisions {
		out = append(out, fmt.Sprintf("revision:%d:%s:%s:%s", r.RevisionNumber, r.SegmentStart.UTC().Format(time.RFC3339), r.SegmentEnd.UTC().Format(time.RFC3339), r.ProbeID))
		for _, q := range r.Readings {
			out = append(out, fmt.Sprintf("reading:%d:%s:%.3f", r.RevisionNumber, q.At.UTC().Format(time.RFC3339Nano), q.TemperatureC))
		}
	}
	for _, f := range c.Findings {
		out = append(out, fmt.Sprintf("finding:%s:%d:%s:%s:%s:%s:%s:%s", f.ID, f.RevisionNumber, f.Kind, f.Severity, f.Decision, f.DecidedBy, f.DecisionNote, strings.Join(f.EvidenceRefs, ",")))
	}
	for _, a := range c.Audit {
		// The release event is appended after the credential is frozen. Exclude it
		// so a post-issue verification recomputes the exact pre-issue manifest.
		if a.Action == "release" {
			continue
		}
		out = append(out, fmt.Sprintf("audit:%d:%s:%s", a.Sequence, a.Action, a.Hash))
	}
	sort.Strings(out)
	return out
}
func Digest(entries []string) string {
	h := sha256.New()
	for _, e := range entries {
		h.Write([]byte(e))
		h.Write([]byte("\n"))
	}
	return hex.EncodeToString(h.Sum(nil))
}
