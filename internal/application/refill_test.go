package application

import (
	"coldchain/internal/domain"
	"coldchain/internal/storage"
	"errors"
	"testing"
	"time"
)

func TestUrgentQueueIncludesWindowConflictsWithoutMutation(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := New(store)
	now := time.Now().UTC().Truncate(time.Second)
	first, err := service.Create("Q-002", "发送方", "接收方", now.Add(20*time.Minute), now.Add(80*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Create("Q-001", "发送方", "接收方", now.Add(20*time.Minute), now.Add(50*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	beforeVersion, beforeAudit := first.Version, len(first.Audit)
	due := 30
	result, err := service.Query(domain.CaseFilter{WindowPhase: domain.WindowPhasePending, DueWithinMinutes: &due, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 2 || result.Items[0].ID != second.ID || result.Items[1].ID != first.ID {
		t.Fatalf("临期队列顺序异常: %#v", result.Items)
	}
	for _, item := range result.Items {
		if item.WindowPhase != domain.WindowPhasePending || item.RemainingMinutes != 20 || len(item.WindowConflicts) != 1 {
			t.Fatalf("窗口投影异常: %#v", item)
		}
	}
	after, _ := service.Get(first.ID)
	if after.Version != beforeVersion || len(after.Audit) != beforeAudit {
		t.Fatal("只读临期查询改变了案卷")
	}
}

func TestInvalidUrgencyFilterAndCalibrationFailureAreAtomic(t *testing.T) {
	store, _ := storage.New(t.TempDir())
	service := New(store)
	now := time.Now().UTC().Truncate(time.Second)
	negative := -1
	if _, err := service.Query(domain.CaseFilter{DueWithinMinutes: &negative}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("负临期分钟应失败: %v", err)
	}
	c, _ := service.Create("CAL-1", "甲", "乙", now, now.Add(6*time.Hour))
	c, err := service.RegisterBasics(c.ID, c.Version,
		[]domain.SampleContainer{{ID: "box", ContainerCode: "BOX", SealCode: "SEAL", SampleCategory: "冷藏", MinTemperatureC: 2, MaxTemperatureC: 8}},
		[]domain.TemperatureProbe{{ID: "probe", SerialNumber: "P-1", CertificateRef: "CERT-1", CalibratedAt: now.Add(-time.Hour), CalibrationExpiresAt: now.Add(7 * time.Hour), AccuracyC: .2}},
	)
	if err != nil {
		t.Fatal(err)
	}
	ledger := domain.CalibrationLedger(c)
	if len(ledger) != 1 || ledger[0].Status != "临期" || ledger[0].CertificateRef != "CERT-1" || ledger[0].RemainingHours != 1 {
		t.Fatalf("校准台账异常: %#v", ledger)
	}
	version, audits := c.Version, len(c.Audit)
	_, err = service.AddRevision(c.ID, version, domain.TransportEvidenceRevision{ProbeID: "probe", SegmentStart: now.Add(6 * time.Hour), SegmentEnd: now.Add(8 * time.Hour), Readings: []domain.TemperatureReading{{At: now.Add(7 * time.Hour), TemperatureC: 4}}})
	var coverageErr *domain.CalibrationCoverageError
	if !errors.As(err, &coverageErr) || coverageErr.CertificateRef != "CERT-1" || len(coverageErr.MissingBoundary) != 1 || coverageErr.MissingBoundary[0] != "segmentEnd" {
		t.Fatalf("校准边界错误异常: %v", err)
	}
	after, _ := service.Get(c.ID)
	if after.Version != version || len(after.Revisions) != 0 || len(after.Audit) != audits {
		t.Fatal("失败的校准核对改变了案卷")
	}
}

func TestDecisionBatchRollbackAndStableUndecided(t *testing.T) {
	store, _ := storage.New(t.TempDir())
	service := New(store)
	now := time.Now().UTC()
	c, _ := domain.NewCase("case-decisions", "DEC-1", "甲", "乙", now, now.Add(time.Hour), now)
	c.Status = domain.StatusReview
	c.Findings = []domain.RiskFinding{{ID: "finding-b", Kind: "超温"}, {ID: "finding-a", Kind: "读数断档"}}
	c.RecordAudit("create", "system", "create")
	if err := store.Save(c, "fixture"); err != nil {
		t.Fatal(err)
	}
	version, audits := c.Version, len(c.Audit)
	_, err := service.DecideBatch(c.ID, version, []FindingDecision{{FindingID: "finding-a", Decision: "接受", Note: "确认", Reviewer: "审核员"}, {FindingID: "finding-a", Decision: "接受", Note: "重复", Reviewer: "审核员"}})
	if !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("重复发现应失败: %v", err)
	}
	after, _ := service.Get(c.ID)
	if after.Version != version || len(after.Audit) != audits || after.Findings[0].Decision != "" || after.Findings[1].Decision != "" {
		t.Fatal("失败批量裁决产生了部分提交")
	}
	decided, err := service.DecideBatch(c.ID, version, []FindingDecision{{FindingID: "finding-b", Decision: "接受", Note: "确认", Reviewer: "审核员"}, {FindingID: "finding-a", Decision: "接受", Note: "确认", Reviewer: "审核员"}})
	if err != nil || decided.Version != version+1 || !decided.AllFindingsDecided() {
		t.Fatalf("合法批量裁决异常: %#v %v", decided, err)
	}
}
