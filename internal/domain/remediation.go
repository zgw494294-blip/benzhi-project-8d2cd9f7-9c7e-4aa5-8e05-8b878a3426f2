package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type RemediationClosure struct {
	Covered   []RiskFinding `json:"covered"`
	Uncovered []RiskFinding `json:"uncovered"`
	Added     []RiskFinding `json:"added"`
}

type RemediationTask struct {
	FindingID            string     `json:"findingId"`
	Owner                string     `json:"owner"`
	Assignee             string     `json:"assignee,omitempty"`
	ResponsiblePerson    string     `json:"responsiblePerson,omitempty"`
	DueAt                time.Time  `json:"dueAt"`
	Deadline             *time.Time `json:"deadline,omitempty"`
	RequiredType         string     `json:"requiredEvidenceType"`
	RequiredEvidenceType string     `json:"requiredEvidence,omitempty"`
	BaselineVersion      int        `json:"baselineVersion"`
	Status               string     `json:"status"`
	CoveredBy            []int      `json:"coveredBy,omitempty"`
}

func RemediationClosureForCase(c *ColdChainCase) RemediationClosure {
	out := RemediationClosure{}
	if c.ReviewBaselineRevision == 0 {
		return out
	}
	baseline := make(map[string]RiskFinding, len(c.ReviewBaseline))
	for _, finding := range c.ReviewBaseline {
		baseline[finding.ID] = finding
		covered := false
		start, end := findingBounds(c, finding)
		for _, revision := range c.Revisions {
			if revision.RevisionNumber <= c.ReviewBaselineRevision || revision.RemediationNote == "" {
				continue
			}
			explicit := false
			for _, findingID := range revision.RemediatesFindingIDs {
				if findingID == finding.ID {
					explicit = true
					break
				}
			}
			overlaps := !start.IsZero() && revision.SegmentStart.Before(end) && start.Before(revision.SegmentEnd) || !start.IsZero() && revision.SegmentStart.Equal(end) || !end.IsZero() && revision.SegmentEnd.Equal(start)
			if explicit || overlaps {
				covered = true
				break
			}
		}
		if covered {
			out.Covered = append(out.Covered, finding)
		} else {
			out.Uncovered = append(out.Uncovered, finding)
		}
	}
	for _, finding := range c.Findings {
		if _, ok := baseline[finding.ID]; !ok && finding.RevisionNumber > c.ReviewBaselineRevision {
			out.Added = append(out.Added, finding)
		}
	}
	sortFindings := func(items []RiskFinding) { sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID }) }
	sortFindings(out.Covered)
	sortFindings(out.Uncovered)
	sortFindings(out.Added)
	return out
}

func findingBounds(c *ColdChainCase, finding RiskFinding) (start, end time.Time) {
	if finding.StartAt != nil {
		start = *finding.StartAt
	}
	if finding.EndAt != nil {
		end = *finding.EndAt
	}
	if !start.IsZero() && !end.IsZero() {
		return start, end
	}
	for _, revision := range c.Revisions {
		if revision.RevisionNumber == finding.RevisionNumber {
			return revision.SegmentStart, revision.SegmentEnd
		}
	}
	return start, end
}

func (c *ColdChainCase) SetRemediationTasks(tasks []RemediationTask, now time.Time) error {
	if c.Status != StatusReview && c.Status != StatusResubmit {
		return ErrInvalidState
	}
	if len(tasks) == 0 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for i := range tasks {
		t := &tasks[i]
		if strings.TrimSpace(t.RequiredType) == "" {
			t.RequiredType = t.RequiredEvidenceType
		}
		if strings.TrimSpace(t.Owner) == "" {
			if strings.TrimSpace(t.Assignee) != "" {
				t.Owner = t.Assignee
			} else {
				t.Owner = t.ResponsiblePerson
			}
		}
		if t.DueAt.IsZero() && t.Deadline != nil {
			t.DueAt = *t.Deadline
		}
		if seen[t.FindingID] || strings.TrimSpace(t.FindingID) == "" || strings.TrimSpace(t.Owner) == "" || strings.TrimSpace(t.RequiredType) == "" || t.DueAt.IsZero() {
			return fmt.Errorf("%w:整改任务[%d]字段不完整或发现重复", ErrInvalid, i+1)
		}
		if !t.DueAt.After(now) {
			return fmt.Errorf("%w:整改任务[%d]截止时间必须在未来", ErrInvalid, i+1)
		}
		found := false
		for _, f := range c.Findings {
			if f.ID == t.FindingID {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%w:整改任务[%d]引用的发现不存在", ErrInvalid, i+1)
		}
		seen[t.FindingID] = true
		t.BaselineVersion = c.Version
		t.Status = "未覆盖"
		t.CoveredBy = nil
	}
	c.RemediationTasks = append([]RemediationTask(nil), tasks...)
	c.ReviewBaseline = append([]RiskFinding(nil), c.Findings...)
	c.ReviewFingerprint = FindingsFingerprint(c.Findings)
	c.ReviewBaselineRevision = len(c.Revisions)
	c.touch()
	return nil
}

func (c *ColdChainCase) UpdateRemediationTasks(now time.Time) {
	if len(c.RemediationTasks) == 0 {
		return
	}
	for i := range c.RemediationTasks {
		t := &c.RemediationTasks[i]
		t.Status = "未覆盖"
		t.CoveredBy = nil
		for _, r := range c.Revisions {
			if r.RevisionNumber <= c.ReviewBaselineRevision || r.RemediationNote == "" {
				continue
			}
			for _, id := range r.RemediatesFindingIDs {
				if id == t.FindingID {
					typeCovered := t.RequiredType != "温度读数" || len(r.Readings) > 0
					typeCovered = typeCovered && (t.RequiredType != "封签证据" || strings.TrimSpace(r.SealObservation) != "")
					typeCovered = typeCovered && (t.RequiredType != "整改说明" && t.RequiredType != "说明" || strings.TrimSpace(r.RemediationNote) != "")
					if typeCovered {
						t.Status = "已覆盖"
						t.CoveredBy = append(t.CoveredBy, r.RevisionNumber)
					}
				}
			}
		}
		if t.Status == "未覆盖" && !t.DueAt.After(now) {
			t.Status = "已过期"
		}
		if t.Status == "已覆盖" {
			baseline := map[string]bool{}
			for _, finding := range c.ReviewBaseline {
				baseline[finding.ID] = true
			}
			for _, finding := range c.Findings {
				if baseline[finding.ID] {
					continue
				}
				for _, revisionNumber := range t.CoveredBy {
					if finding.RevisionNumber == revisionNumber {
						t.Status = "新增风险"
					}
				}
			}
		}
	}
}

func (c *ColdChainCase) ValidateRemediationGate(now time.Time) error {
	c.UpdateRemediationTasks(now)
	reasons := make([]RemediationTaskReason, 0)
	for _, t := range c.RemediationTasks {
		if strings.TrimSpace(t.Owner) == "" {
			reasons = append(reasons, RemediationTaskReason{FindingID: t.FindingID, Code: "OWNER_REQUIRED", Message: "责任人不能为空"})
		}
		if t.DueAt.IsZero() {
			reasons = append(reasons, RemediationTaskReason{FindingID: t.FindingID, Code: "DUE_AT_REQUIRED", Message: "截止时间不能为空"})
		}
		if t.Status != "已覆盖" {
			code := "NOT_COVERED"
			if t.Status == "已过期" {
				code = "OVERDUE"
			}
			reasons = append(reasons, RemediationTaskReason{FindingID: t.FindingID, Code: code, Message: "整改任务尚未闭环"})
		}
	}
	if len(reasons) > 0 {
		return &RemediationGateError{Reasons: reasons}
	}
	return nil
}
