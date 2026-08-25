package application

import (
	"coldchain/internal/domain"
	"coldchain/internal/storage"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestConflictReviewRechecksDigestAndIsAtomic(t *testing.T) {
	s := newTestService(t)
	now := time.Now().UTC()
	first, _ := s.Create("CONFLICT-A", "发送方", "接收方", now.Add(-time.Minute), now.Add(time.Hour))
	_, _ = s.Create("CONFLICT-B", "发送方", "接收方", now, now.Add(2*time.Hour))
	view, _ := s.Get(first.ID)
	if len(view.WindowConflicts) != 1 || view.WindowConflictDigest == "" {
		t.Fatal("详情缺少冲突摘要")
	}
	version, audits := view.Version, len(view.Audit)
	_, _ = s.Create("CONFLICT-C", "发送方", "接收方", now, now.Add(30*time.Minute))
	if _, err := s.ReviewConflicts(first.ID, version, view.WindowConflicts, view.WindowConflictDigest, "接受", "复核员"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("变化的冲突集合应拒绝: %v", err)
	}
	after, _ := s.Get(first.ID)
	if after.Version != version || len(after.Audit) != audits {
		t.Fatal("失败复核修改了案卷")
	}
	accepted, err := s.ReviewConflicts(first.ID, version, after.WindowConflicts, after.WindowConflictDigest, "接受", "复核员")
	if err != nil || accepted.Version != version+1 || len(accepted.ConflictReviews) != 1 || accepted.Audit[len(accepted.Audit)-1].Action != "conflict-review" {
		t.Fatalf("冲突复核结果异常: %#v %v", accepted, err)
	}
}

func TestProbeReplacementAndEvidencePrecheckFingerprint(t *testing.T) {
	s := newTestService(t)
	now := time.Now().UTC()
	c, _ := s.Create("PROBE-PRECHECK", "甲", "乙", now, now.Add(4*time.Hour))
	c, _ = s.RegisterBasics(c.ID, c.Version, []domain.SampleContainer{{ID: "box", ContainerCode: "BOX", SealCode: "SEAL", MinTemperatureC: 2, MaxTemperatureC: 8}}, []domain.TemperatureProbe{{ID: "old", SerialNumber: "OLD", CertificateRef: "CERT-OLD", CalibratedAt: now.Add(-time.Hour), CalibrationExpiresAt: now.Add(5 * time.Hour), AccuracyC: .2}})
	first := segment("old", now, now.Add(2*time.Hour), 4)
	pre, err := s.PrecheckEvidence(c.ID, c.Version, []domain.TransportEvidenceRevision{first})
	if err != nil || !pre.Valid || pre.Fingerprint == "" {
		t.Fatalf("预检失败: %#v %v", pre, err)
	}
	c, err = s.AddRevisionWithFingerprint(c.ID, c.Version, first, pre.Fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	c, err = s.ReplaceProbe(c.ID, c.Version, "old", domain.TemperatureProbe{ID: "new", SerialNumber: "NEW", CertificateRef: "CERT-NEW", CalibratedAt: now, CalibrationExpiresAt: now.Add(5 * time.Hour), AccuracyC: .2}, now.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if c.Revisions[0].ProbeID != "old" || len(c.Probes) != 2 {
		t.Fatal("替换改写了历史证据")
	}
	second := segment("new", now.Add(2*time.Hour), now.Add(4*time.Hour), 4)
	pre, err = s.PrecheckEvidence(c.ID, c.Version, []domain.TransportEvidenceRevision{second})
	if err != nil || !pre.Valid {
		t.Fatal(err)
	}
	modified := second
	modified.Readings = append([]domain.TemperatureReading(nil), second.Readings...)
	modified.Readings[1].TemperatureC = 5
	version, audits := c.Version, len(c.Audit)
	if _, err = s.AddRevisionWithFingerprint(c.ID, c.Version, modified, pre.Fingerprint); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("修改输入应使指纹失效: %v", err)
	}
	after, _ := s.Get(c.ID)
	if after.Version != version || len(after.Audit) != audits || len(after.Revisions) != 1 {
		t.Fatal("指纹失败修改了案卷")
	}
}

func TestCredentialReviewPersistsWithIdempotentBatch(t *testing.T) {
	dir := t.TempDir()
	st, _ := storage.New(dir)
	s := New(st)
	now := time.Now().UTC()
	c, _ := s.Create("VERIFY-RECORD", "甲", "乙", now, now.Add(time.Hour))
	inputs := []string{c.ID, "missing"}
	first, err := s.VerifyCredentialsRecorded(inputs, "batch-key")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.VerifyCredentialsRecorded(inputs, "batch-key")
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatal("幂等批次结果不一致")
	}
	record, err := s.RecordCredentialReview(c.ID, first.BatchID, first.Items[0].CheckDigest, "操作员", "确认未放行", "等待审核")
	if err != nil {
		t.Fatal(err)
	}
	if record.Sequence != 1 || record.Hash == "" {
		t.Fatal("核验复核缺少哈希链")
	}
	recovered, err := storage.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	restored := New(recovered)
	if got := restored.CredentialReviews("", "CASE_NOT_RELEASED"); len(got) != 1 || got[0].Operator != "操作员" {
		t.Fatalf("恢复后记录缺失: %#v", got)
	}
	replayed, err := restored.VerifyCredentialsRecorded(inputs, "batch-key")
	if err != nil || !reflect.DeepEqual(first, replayed) {
		t.Fatal("恢复后幂等批次丢失")
	}
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	st, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return New(st)
}
func segment(probe string, start, end time.Time, value float64) domain.TransportEvidenceRevision {
	middle := start.Add(end.Sub(start) / 2)
	return domain.TransportEvidenceRevision{ProbeID: probe, SegmentStart: start, SegmentEnd: end, SealObservation: "SEAL", Readings: []domain.TemperatureReading{{At: start, TemperatureC: value}, {At: middle, TemperatureC: value}, {At: end, TemperatureC: value}}}
}
