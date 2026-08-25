package domain

import "errors"
import "fmt"

var (
	ErrNotFound       = errors.New("案卷不存在")
	ErrInvalid        = errors.New("输入不符合业务规则")
	ErrConflict       = errors.New("版本冲突")
	ErrInvalidState   = errors.New("当前状态不允许该操作")
	ErrImmutable      = errors.New("已冻结内容不可修改")
	ErrRiskUnresolved = errors.New("仍有未裁决风险")
)

// CaseConflictError carries a stable, machine-readable explanation for create conflicts.
type CaseConflictError struct {
	Code            string `json:"code"`
	CaseNumber      string `json:"caseNumber,omitempty"`
	ConflictingCase string `json:"conflictingCase,omitempty"`
	Sender          string `json:"sender,omitempty"`
	Receiver        string `json:"receiver,omitempty"`
	WindowStart     string `json:"windowStart,omitempty"`
	WindowEnd       string `json:"windowEnd,omitempty"`
}

func (e *CaseConflictError) Error() string {
	if e.Code == "DUPLICATE_CASE_NUMBER" {
		return fmt.Sprintf("案卷编号重复:%s", e.CaseNumber)
	}
	return fmt.Sprintf("交接窗口与案卷%s重叠:%s至%s", e.ConflictingCase, e.WindowStart, e.WindowEnd)
}
func (e *CaseConflictError) Unwrap() error { return ErrConflict }

type EvidenceDuplicateError struct {
	RevisionNumber int    `json:"revisionNumber"`
	EvidenceRef    string `json:"evidenceRef"`
}

func (e *EvidenceDuplicateError) Error() string {
	return fmt.Sprintf("重复证据已存在于修订%d:%s", e.RevisionNumber, e.EvidenceRef)
}
func (e *EvidenceDuplicateError) Unwrap() error { return ErrInvalid }

type CalibrationCoverageError struct {
	ProbeID         string   `json:"probeId"`
	CertificateRef  string   `json:"certificateRef"`
	MissingBoundary []string `json:"missingBoundaries"`
}

func (e *CalibrationCoverageError) Error() string {
	return fmt.Sprintf("探头%s的校准证书%s未覆盖分段边界:%v", e.ProbeID, e.CertificateRef, e.MissingBoundary)
}
func (e *CalibrationCoverageError) Unwrap() error { return ErrInvalid }

type RiskFingerprintConflictError struct {
	ProvidedFingerprint string `json:"providedFingerprint"`
	CurrentFingerprint  string `json:"currentFingerprint"`
	CurrentVersion      int    `json:"currentVersion"`
}

func (e *RiskFingerprintConflictError) Error() string {
	return "风险确认令牌与当前证据不匹配"
}
func (e *RiskFingerprintConflictError) Unwrap() error { return ErrConflict }

type EvidencePrecheckConflictError struct {
	ProvidedFingerprint string `json:"providedFingerprint"`
	CurrentFingerprint  string `json:"currentFingerprint"`
	CurrentVersion      int    `json:"currentVersion"`
}

func (e *EvidencePrecheckConflictError) Error() string {
	return "证据预检指纹与当前案卷或输入不匹配，请重新预检"
}
func (e *EvidencePrecheckConflictError) Unwrap() error { return ErrConflict }

type RemediationGateError struct {
	Reasons []RemediationTaskReason `json:"reasons"`
}

func (e *RemediationGateError) Error() string { return "整改任务尚未满足复审门禁" }
func (e *RemediationGateError) Unwrap() error { return ErrInvalid }

type RemediationTaskReason struct {
	FindingID string `json:"findingId"`
	Code      string `json:"code"`
	Message   string `json:"message"`
}
