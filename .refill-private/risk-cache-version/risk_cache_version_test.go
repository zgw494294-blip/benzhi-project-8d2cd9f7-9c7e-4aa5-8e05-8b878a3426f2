package riskcacheversion

import (
	"coldchain/internal/application"
	"coldchain/internal/domain"
	"coldchain/internal/httpapi"
	"coldchain/internal/storage"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRiskCacheRefreshesAfterEvidenceVersionChanges(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := application.New(store)
	server := httptest.NewServer(httpapi.New(app).Handler())
	defer server.Close()

	now := time.Now().UTC().Truncate(time.Second)
	caseRecord, err := app.Create("RISK-CACHE", "发方", "收方", now, now.Add(6*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	caseRecord, err = app.RegisterBasics(caseRecord.ID, caseRecord.Version,
		[]domain.SampleContainer{{ID: "box", ContainerCode: "BOX", SealCode: "SEAL", MinTemperatureC: 2, MaxTemperatureC: 8}},
		[]domain.TemperatureProbe{{ID: "probe", SerialNumber: "PROBE", CertificateRef: "CERT", CalibratedAt: now.Add(-time.Hour), CalibrationExpiresAt: now.Add(7 * time.Hour)}})
	if err != nil {
		t.Fatal(err)
	}
	caseRecord, err = app.AddRevision(caseRecord.ID, caseRecord.Version, revision(now, now.Add(3*time.Hour), "SEAL"))
	if err != nil {
		t.Fatal(err)
	}
	if got := riskCount(t, server.URL+"/api/cases/"+caseRecord.ID+"/risk"); got != 0 {
		t.Fatalf("初次风险数量 = %d，期望 0", got)
	}
	caseRecord, err = app.AddRevision(caseRecord.ID, caseRecord.Version, revision(now.Add(3*time.Hour), now.Add(6*time.Hour), "BROKEN-SEAL"))
	if err != nil {
		t.Fatal(err)
	}
	if got := riskCount(t, server.URL+"/api/cases/"+caseRecord.ID+"/risk"); got != 1 {
		t.Fatalf("追加封签变化证据后的风险数量 = %d，期望 1", got)
	}
}

func revision(start, end time.Time, seal string) domain.TransportEvidenceRevision {
	middle := start.Add(end.Sub(start) / 2)
	return domain.TransportEvidenceRevision{
		ProbeID: "probe", SegmentStart: start, SegmentEnd: end, SealObservation: seal,
		Readings: []domain.TemperatureReading{{At: start, TemperatureC: 4}, {At: middle, TemperatureC: 4}, {At: end, TemperatureC: 4}},
	}
}

func riskCount(t *testing.T, url string) int {
	t.Helper()
	response, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var body struct {
		Count int `json:"count"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body.Count
}
