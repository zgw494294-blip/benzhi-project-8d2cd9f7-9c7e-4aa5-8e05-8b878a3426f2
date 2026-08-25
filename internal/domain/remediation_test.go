package domain

import (
	"errors"
	"testing"
	"time"
)

func TestRemediationClosureRequiresReferencedNewEvidence(t *testing.T) {
	now := time.Now().UTC()
	c, _ := NewCase("case-remediation", "REM-1", "甲", "乙", now, now.Add(6*time.Hour), now)
	c.Status = StatusResubmit
	c.Containers = []SampleContainer{{ID: "box", ContainerCode: "BOX", SealCode: "SEAL", MinTemperatureC: 2, MaxTemperatureC: 8}}
	c.Probes = []TemperatureProbe{{ID: "probe", SerialNumber: "P", CertificateRef: "CERT", CalibratedAt: now.Add(-time.Hour), CalibrationExpiresAt: now.Add(7 * time.Hour)}}
	c.ReviewBaselineRevision = 1
	firstStart, firstEnd := now, now.Add(2*time.Hour)
	secondStart, secondEnd := now.Add(5*time.Hour), now.Add(6*time.Hour)
	c.ReviewBaseline = []RiskFinding{{ID: "finding-a", RevisionNumber: 1, StartAt: &firstStart, EndAt: &firstEnd}, {ID: "finding-b", RevisionNumber: 1, StartAt: &secondStart, EndAt: &secondEnd}}
	c.Findings = append([]RiskFinding(nil), c.ReviewBaseline...)
	c.Revisions = []TransportEvidenceRevision{{RevisionNumber: 1, SegmentStart: now, SegmentEnd: now.Add(2 * time.Hour), ProbeID: "probe", Readings: []TemperatureReading{{At: now, TemperatureC: 4}, {At: now.Add(time.Hour), TemperatureC: 4}, {At: now.Add(2 * time.Hour), TemperatureC: 4}}}, {RevisionNumber: 2, SegmentStart: now.Add(2 * time.Hour), SegmentEnd: now.Add(4 * time.Hour), ProbeID: "probe", RemediationNote: "补充记录", RemediatesFindingIDs: []string{"finding-a"}, Readings: []TemperatureReading{{At: now.Add(2 * time.Hour), TemperatureC: 4}, {At: now.Add(3 * time.Hour), TemperatureC: 4}, {At: now.Add(4 * time.Hour), TemperatureC: 4}}}}
	closure := RemediationClosureForCase(c)
	if len(closure.Covered) != 1 || closure.Covered[0].ID != "finding-a" || len(closure.Uncovered) != 1 || closure.Uncovered[0].ID != "finding-b" {
		t.Fatalf("整改闭环异常: %#v", closure)
	}
	if err := c.SubmitReview(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("未覆盖整改项应阻止复审: %v", err)
	}
	c.Revisions = append(c.Revisions, TransportEvidenceRevision{RevisionNumber: 3, SegmentStart: now.Add(4 * time.Hour), SegmentEnd: now.Add(6 * time.Hour), ProbeID: "probe", RemediationNote: "补充记录", RemediatesFindingIDs: []string{"finding-b"}, Readings: []TemperatureReading{{At: now.Add(4 * time.Hour), TemperatureC: 4}, {At: now.Add(5 * time.Hour), TemperatureC: 4}, {At: now.Add(6 * time.Hour), TemperatureC: 4}}})
	if err := c.SubmitReview(); err != nil {
		t.Fatalf("全部覆盖后应允许复审: %v", err)
	}
}
