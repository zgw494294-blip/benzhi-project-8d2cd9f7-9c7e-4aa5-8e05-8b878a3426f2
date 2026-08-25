package domain

import (
	"errors"
	"testing"
	"time"
)

func TestRemediationTasksGateResubmission(t *testing.T) {
	now := time.Now().UTC()
	c, _ := NewCase("case", "TASK-1", "甲", "乙", now, now.Add(2*time.Hour), now)
	_ = c.RegisterContainer(SampleContainer{ID: "box", ContainerCode: "BOX", SealCode: "SEAL", MinTemperatureC: 2, MaxTemperatureC: 8})
	_ = c.RegisterProbe(TemperatureProbe{ID: "probe", SerialNumber: "P", CertificateRef: "C", CalibratedAt: now.Add(-time.Hour), CalibrationExpiresAt: now.Add(3 * time.Hour), AccuracyC: .2})
	_ = c.AddRevision(taskSegment(now, now.Add(time.Hour), nil))
	c.Findings = []RiskFinding{{ID: "finding-1", CaseID: c.ID, RevisionNumber: 1, Kind: "采样稀疏", Severity: "中"}}
	c.Status = StatusReview
	if err := c.ReturnForCorrectionWithTasks("补齐后半程", "审核员", []RemediationTask{{FindingID: "finding-1", Owner: "整理员", DueAt: now.Add(time.Hour), RequiredType: "温度读数"}}); err != nil {
		t.Fatal(err)
	}
	if err := c.SubmitReview(); err == nil {
		t.Fatal("未覆盖任务不应允许复审")
	}
	if err := c.AddRevision(taskSegment(now.Add(time.Hour), now.Add(2*time.Hour), []string{"finding-1"})); err != nil {
		t.Fatal(err)
	}
	if c.Revisions[0].ProbeID != "probe" || c.RemediationTasks[0].Status != "已覆盖" {
		t.Fatalf("任务状态异常: %#v", c.RemediationTasks)
	}
	if err := c.SubmitReview(); err != nil {
		t.Fatal(err)
	}
	old := *c
	c.Status = StatusReview
	if err := c.ReturnForCorrectionWithTasks("再次整改", "审核员", []RemediationTask{{FindingID: "finding-1", Owner: "", DueAt: now.Add(time.Hour), RequiredType: "温度读数"}}); !errors.Is(err, ErrInvalid) {
		t.Fatal("缺少责任人应拒绝")
	}
	if c.Version != old.Version {
		t.Fatal("失败任务登记改变版本")
	}
}

func taskSegment(start, end time.Time, ids []string) TransportEvidenceRevision {
	middle := start.Add(end.Sub(start) / 2)
	return TransportEvidenceRevision{ProbeID: "probe", SegmentStart: start, SegmentEnd: end, SealObservation: "SEAL", RemediationNote: func() string {
		if len(ids) > 0 {
			return "补充证据"
		}
		return ""
	}(), RemediatesFindingIDs: ids, Readings: []TemperatureReading{{At: start, TemperatureC: 4}, {At: middle, TemperatureC: 4}, {At: end, TemperatureC: 4}}}
}
