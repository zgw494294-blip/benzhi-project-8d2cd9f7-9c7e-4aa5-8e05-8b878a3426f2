package application

import (
	"coldchain/internal/domain"
	"coldchain/internal/storage"
	"testing"
	"time"
)

func TestWorkflow(t *testing.T) {
	st, _ := storage.New(t.TempDir())
	s := New(st)
	now := time.Now().UTC()
	c, e := s.Create("CC-1", "发方", "收方", now, now.Add(6*time.Hour))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.RegisterContainer(c.ID, c.Version, domain.SampleContainer{ID: "box", ContainerCode: "BOX", SealCode: "SEAL", MinTemperatureC: 2, MaxTemperatureC: 8}); e != nil {
		t.Fatal(e)
	}
	c, _ = s.Get(c.ID)
	if _, e = s.RegisterProbe(c.ID, c.Version, domain.TemperatureProbe{ID: "p1", SerialNumber: "P1", CalibratedAt: now.Add(-time.Hour), CalibrationExpiresAt: now.Add(7 * time.Hour), AccuracyC: .2}); e != nil {
		t.Fatal(e)
	}
	c, _ = s.Get(c.ID)
	_, e = s.AddRevision(c.ID, c.Version, domain.TransportEvidenceRevision{ProbeID: "p1", SegmentStart: now, SegmentEnd: now.Add(2 * time.Hour), Readings: []domain.TemperatureReading{{At: now.Add(time.Hour), TemperatureC: 4}}, SealObservation: "SEAL"})
	if e != nil {
		t.Fatal(e)
	}
}

func TestBatchBasicsAtomicAndSingleVersion(t *testing.T) {
	st, _ := storage.New(t.TempDir())
	s := New(st)
	now := time.Now().UTC()
	c, _ := s.Create("CC-BATCH", "甲", "乙", now, now.Add(6*time.Hour))
	startVersion := c.Version
	c, err := s.RegisterBasics(c.ID, c.Version, []domain.SampleContainer{
		{ID: "b1", ContainerCode: "B1", SealCode: "S1", MinTemperatureC: 2, MaxTemperatureC: 8},
		{ID: "b2", ContainerCode: "B2", SealCode: "S2", MinTemperatureC: 2, MaxTemperatureC: 8},
	}, []domain.TemperatureProbe{{ID: "p1", SerialNumber: "P1", CalibratedAt: now.Add(-time.Hour), CalibrationExpiresAt: now.Add(7 * time.Hour)}})
	if err != nil {
		t.Fatal(err)
	}
	if c.Version != startVersion+1 || c.Status != domain.StatusCollecting || len(c.Containers) != 2 || len(c.Probes) != 1 {
		t.Fatalf("批量登记结果异常: %#v", c)
	}
	version, audits := c.Version, len(c.Audit)
	_, err = s.RegisterBasics(c.ID, c.Version, []domain.SampleContainer{{ID: "b3", ContainerCode: "B3", SealCode: "S2", MinTemperatureC: 2, MaxTemperatureC: 8}}, nil)
	if err == nil {
		t.Fatal("重复封签应失败")
	}
	c, _ = s.Get(c.ID)
	if c.Version != version || len(c.Containers) != 2 || len(c.Audit) != audits {
		t.Fatal("失败批次修改了案卷")
	}
}

func TestQueryStableAndFilteredCounts(t *testing.T) {
	st, _ := storage.New(t.TempDir())
	s := New(st)
	now := time.Now().UTC()
	a, _ := s.Create("B-2", "发送甲", "接收甲", now.Add(time.Hour), now.Add(2*time.Hour))
	b, _ := s.Create("A-1", "发送乙", "接收乙", now.Add(time.Hour), now.Add(2*time.Hour))
	_, _ = s.RegisterContainer(b.ID, b.Version, domain.SampleContainer{ID: "x", ContainerCode: "X", SealCode: "SX", MinTemperatureC: 2, MaxTemperatureC: 8})
	r, err := s.Query(domain.CaseFilter{Status: string(domain.StatusDraft)})
	if err != nil {
		t.Fatal(err)
	}
	if r.Total != 1 || r.Items[0].ID != a.ID || r.Counts[string(domain.StatusDraft)] != 1 {
		t.Fatalf("筛选统计异常: %#v", r)
	}
	r, _ = s.Query(domain.CaseFilter{})
	if r.Items[0].CaseNumber != "A-1" || r.Items[1].CaseNumber != "B-2" {
		t.Fatal("同一窗口未按案卷编号稳定排序")
	}
	end := now
	start := now.Add(time.Hour)
	if _, err = s.Query(domain.CaseFilter{HandoffStart: &start, HandoffEnd: &end}); err == nil {
		t.Fatal("非法窗口应失败")
	}
}

func TestBatchEvidenceRollback(t *testing.T) {
	st, _ := storage.New(t.TempDir())
	s := New(st)
	now := time.Now().UTC()
	c, _ := s.Create("CC-E", "甲", "乙", now, now.Add(6*time.Hour))
	c, _ = s.RegisterBasics(c.ID, c.Version, []domain.SampleContainer{{ID: "b", ContainerCode: "B", SealCode: "S", MinTemperatureC: 2, MaxTemperatureC: 8}}, []domain.TemperatureProbe{{ID: "p", SerialNumber: "P", CalibratedAt: now.Add(-time.Hour), CalibrationExpiresAt: now.Add(7 * time.Hour)}})
	v := c.Version
	rs := []domain.TransportEvidenceRevision{{ProbeID: "p", SegmentStart: now, SegmentEnd: now.Add(4 * time.Hour), Readings: []domain.TemperatureReading{{At: now.Add(time.Hour), TemperatureC: 4}}}, {ProbeID: "p", SegmentStart: now.Add(3 * time.Hour), SegmentEnd: now.Add(6 * time.Hour), Readings: []domain.TemperatureReading{{At: now.Add(5 * time.Hour), TemperatureC: 4}}}}
	if _, err := s.AddRevisions(c.ID, v, rs); err == nil {
		t.Fatal("重叠批次应失败")
	}
	c, _ = s.Get(c.ID)
	if c.Version != v || len(c.Revisions) != 0 {
		t.Fatal("失败证据批次修改了案卷")
	}
}
