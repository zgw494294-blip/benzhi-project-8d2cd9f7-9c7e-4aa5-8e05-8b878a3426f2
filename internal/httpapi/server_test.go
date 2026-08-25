package httpapi

import (
	"bytes"
	"coldchain/internal/application"
	"coldchain/internal/domain"
	"coldchain/internal/storage"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func testServer(t *testing.T) (*httptest.Server, *application.Service) {
	t.Helper()
	st, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := application.New(st)
	return httptest.NewServer(New(app).Handler()), app
}
func requestJSON(t *testing.T, method, url string, body any, out any) int {
	t.Helper()
	var b bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&b).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req, err := http.NewRequest(method, url, &b)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if out != nil {
		_ = json.NewDecoder(res.Body).Decode(out)
	}
	return res.StatusCode
}

func TestBatchBasicsPublicFlowIsAtomic(t *testing.T) {
	ts, app := testServer(t)
	defer ts.Close()
	now := time.Now().UTC()
	c, err := app.Create("HTTP-1", "发送方", "接收方", now, now.Add(6*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	body := map[string]any{"expectedVersion": c.Version, "containers": []domain.SampleContainer{{ID: "b1", ContainerCode: "B1", SealCode: "S1", MinTemperatureC: 2, MaxTemperatureC: 8}, {ID: "b2", ContainerCode: "B2", SealCode: "S2", MinTemperatureC: 2, MaxTemperatureC: 8}}, "probes": []domain.TemperatureProbe{{ID: "p1", SerialNumber: "P1", CalibratedAt: now.Add(-time.Hour), CalibrationExpiresAt: now.Add(7 * time.Hour)}}}
	var got domain.ColdChainCase
	if code := requestJSON(t, "POST", ts.URL+"/api/cases/"+c.ID+"/basics", body, &got); code != 200 {
		t.Fatalf("状态码 %d", code)
	}
	if got.Version != c.Version+1 || len(got.Containers) != 2 || len(got.Probes) != 1 {
		t.Fatalf("批量响应异常: %#v", got)
	}
	version, audits := got.Version, len(got.Audit)
	body["expectedVersion"] = version
	body["containers"] = []domain.SampleContainer{{ID: "b3", ContainerCode: "B3", SealCode: "S2", MinTemperatureC: 2, MaxTemperatureC: 8}}
	if code := requestJSON(t, "POST", ts.URL+"/api/cases/"+c.ID+"/basics", body, nil); code != 422 {
		t.Fatalf("状态码 %d", code)
	}
	var after domain.ColdChainCase
	requestJSON(t, "GET", ts.URL+"/api/cases/"+c.ID, nil, &after)
	if after.Version != version || len(after.Containers) != 2 || len(after.Audit) != audits {
		t.Fatal("失败请求或详情查询改变了案卷")
	}
}

func TestListFilterValidationAndStatistics(t *testing.T) {
	ts, app := testServer(t)
	defer ts.Close()
	now := time.Now().UTC()
	_, _ = app.Create("HTTP-2", "发送方", "接收方", now, now.Add(time.Hour))
	var out application.ListResult
	if code := requestJSON(t, "GET", ts.URL+"/api/cases?status="+string(domain.StatusDraft), nil, &out); code != 200 || out.Total != 1 || out.Counts[string(domain.StatusDraft)] != 1 {
		t.Fatalf("筛选响应异常: %d %#v", code, out)
	}
	var invalid map[string]any
	url := ts.URL + "/api/cases?handoffWindowStart=" + now.Add(time.Hour).Format(time.RFC3339) + "&handoffWindowEnd=" + now.Format(time.RFC3339)
	if code := requestJSON(t, "GET", url, nil, &invalid); code != 422 || invalid["code"] != "INVALID_FILTER" {
		t.Fatalf("非法筛选响应异常: %d %#v", code, invalid)
	}
}

func TestWindowConflictAndCredentialBatchEntryPoints(t *testing.T) {
	ts, app := testServer(t)
	defer ts.Close()
	now := time.Now().UTC()
	first, err := app.Create("HTTP-CONFLICT-2", "同一发送方", "同一接收方", now.Add(time.Hour), now.Add(3*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	second, err := app.Create("HTTP-CONFLICT-1", "同一发送方", "同一接收方", now.Add(time.Hour), now.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	var list application.ListResult
	if code := requestJSON(t, "GET", ts.URL+"/api/cases?windowPhase=未开始&dueWithinMinutes=90", nil, &list); code != 200 || list.Total != 2 || len(list.Items[0].WindowConflicts) != 1 {
		t.Fatalf("窗口冲突响应异常: %d %#v", code, list)
	}
	first, err = app.RegisterBasics(first.ID, first.Version, []domain.SampleContainer{{ID: "box", ContainerCode: "BOX", SealCode: "SEAL", SampleCategory: "冷藏", MinTemperatureC: 2, MaxTemperatureC: 8}}, []domain.TemperatureProbe{{ID: "probe", SerialNumber: "PROBE", CertificateRef: "CERT", CalibratedAt: now, CalibrationExpiresAt: now.Add(4 * time.Hour), AccuracyC: .2}})
	if err != nil {
		t.Fatal(err)
	}
	segmentStart, segmentEnd := now.Add(time.Hour), now.Add(3*time.Hour)
	first, err = app.AddRevision(first.ID, first.Version, domain.TransportEvidenceRevision{ProbeID: "probe", SegmentStart: segmentStart, SegmentEnd: segmentEnd, SealObservation: "SEAL", Readings: []domain.TemperatureReading{{At: segmentStart, TemperatureC: 4}, {At: segmentStart.Add(time.Hour), TemperatureC: 4}, {At: segmentEnd, TemperatureC: 4}}})
	if err != nil {
		t.Fatal(err)
	}
	first, err = app.Submit(first.ID, first.Version)
	if err != nil {
		t.Fatal(err)
	}
	first, err = app.Approve(first.ID, first.Version, "审核员")
	if err != nil {
		t.Fatal(err)
	}
	credential, err := app.Issue(first.ID, first.Version, "审核员")
	if err != nil {
		t.Fatal(err)
	}
	var batch map[string]any
	code := requestJSON(t, "GET", ts.URL+"/api/credentials?credentialNumber="+credential.CredentialNumber+",missing-credential&caseId="+second.ID, nil, &batch)
	if code != 200 {
		t.Fatalf("批量凭据状态码异常: %d %#v", code, batch)
	}
	if summary, ok := batch["summary"].(map[string]any); !ok || summary["valid"] != float64(1) || summary["notReleased"] != float64(1) || summary["notFound"] != float64(1) {
		t.Fatalf("批量凭据汇总异常: %#v", batch)
	}
}
