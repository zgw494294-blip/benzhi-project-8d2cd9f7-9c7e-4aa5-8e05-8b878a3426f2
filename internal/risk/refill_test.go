package risk

import (
	"coldchain/internal/domain"
	"reflect"
	"testing"
	"time"
)

func TestCoverageGapSuggestionAndCompletion(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	c, _ := domain.NewCase("case-gap", "GAP-1", "甲", "乙", now, now.Add(6*time.Hour), now)
	c.Revisions = []domain.TransportEvidenceRevision{
		{RevisionNumber: 1, SegmentStart: now, SegmentEnd: now.Add(2 * time.Hour)},
		{RevisionNumber: 2, SegmentStart: now.Add(4 * time.Hour), SegmentEnd: now.Add(6 * time.Hour)},
	}
	coverage := EvidenceCoverage(c)
	if coverage.Covered || len(coverage.Gaps) != 1 || coverage.Gaps[0].DurationSeconds != 7200 || coverage.Gaps[0].PreviousRevisionNumber != 1 || coverage.Gaps[0].NextRevisionNumber != 2 || coverage.NextSegment == nil {
		t.Fatalf("缺口投影异常: %#v", coverage)
	}
	c.Revisions = append(c.Revisions, domain.TransportEvidenceRevision{RevisionNumber: 3, SegmentStart: now.Add(2 * time.Hour), SegmentEnd: now.Add(4 * time.Hour)})
	coverage = EvidenceCoverage(c)
	if !coverage.Covered || len(coverage.Gaps) != 0 || coverage.NextSegment != nil {
		t.Fatalf("补齐后覆盖异常: %#v", coverage)
	}
}

func TestRiskReadingContextIsStable(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	c, _ := domain.NewCase("case-risk", "RISK-1", "甲", "乙", now, now.Add(6*time.Hour), now)
	c.Containers = []domain.SampleContainer{{MinTemperatureC: 2, MaxTemperatureC: 8}}
	c.Probes = []domain.TemperatureProbe{{ID: "probe", CertificateRef: "CERT", CalibratedAt: now.Add(-time.Hour), CalibrationExpiresAt: now.Add(7 * time.Hour)}}
	c.Revisions = []domain.TransportEvidenceRevision{{ID: "revision-1", RevisionNumber: 1, ProbeID: "probe", SegmentStart: now, SegmentEnd: now.Add(6 * time.Hour), Readings: []domain.TemperatureReading{{At: now, TemperatureC: 4}, {At: now.Add(time.Hour), TemperatureC: 12}, {At: now.Add(3 * time.Hour), TemperatureC: 11}, {At: now.Add(6 * time.Hour), TemperatureC: 4}}}}
	first := New().Evaluate(c)
	second := New().Evaluate(c)
	if !reflect.DeepEqual(first, second) || domain.FindingsFingerprint(first) != domain.FindingsFingerprint(second) {
		t.Fatal("重复风险计算不稳定")
	}
	seenOvertemp, seenGap := false, false
	for _, finding := range first {
		if finding.Kind == "超温" {
			seenOvertemp = finding.Context != nil && finding.Context.TriggerReading != nil && finding.Context.PreviousReading != nil && finding.Context.MaxDeviationC == 4 && finding.ContextID != ""
		}
		if finding.Kind == "读数断档" {
			seenGap = seenGap || finding.Context != nil && finding.Context.TriggerReading != nil && finding.Context.PreviousReading != nil && finding.DurationSeconds == 7200 && finding.ContextID != ""
		}
	}
	if !seenOvertemp || !seenGap {
		t.Fatalf("风险上下文缺失: %#v", first)
	}
}
