package atomic_basics_rollback_test

import (
	"bytes"
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

func TestFailedBasicsRequestDoesNotCommitContainer(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	app := application.New(store)
	now := time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)
	c, err := app.Create("REFILL-ATOMIC-1", "发送方", "接收方", now, now.Add(6*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	beforeVersion, beforeAudit := c.Version, len(c.Audit)

	body, err := json.Marshal(map[string]any{
		"expectedVersion": c.Version,
		"containers": []domain.SampleContainer{{
			ID: "box-1", ContainerCode: "BOX-1", SealCode: "SEAL-1",
			SampleCategory: "冷藏", MinTemperatureC: 2, MaxTemperatureC: 8,
		}},
		"probes": []domain.TemperatureProbe{{
			ID: "probe-1", SerialNumber: "PROBE-1", CertificateRef: "CERT-1",
			CalibratedAt: now.Add(-time.Hour), CalibrationExpiresAt: now.Add(5 * time.Hour), AccuracyC: 0.2,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/cases/"+c.ID+"/basics", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	httpapi.New(app).Handler().ServeHTTP(recorder, req)
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("校准未覆盖交接窗口应返回 422，实际为 %d: %s", recorder.Code, recorder.Body.String())
	}

	after, err := app.Get(c.ID)
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := storage.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := restarted.Get(c.ID)
	if err != nil {
		t.Fatal(err)
	}
	currentChanged := after.Version != beforeVersion || len(after.Containers) != 0 || len(after.Probes) != 0 || len(after.Audit) != beforeAudit
	restartChanged := recovered.Version != beforeVersion || len(recovered.Containers) != 0 || len(recovered.Probes) != 0 || len(recovered.Audit) != beforeAudit
	if currentChanged || restartChanged {
		t.Fatalf("TestFailedBasicsRequestDoesNotCommitContainer: 失败请求产生并持久化部分提交: currentVersion=%d currentContainers=%d recoveredVersion=%d recoveredContainers=%d", after.Version, len(after.Containers), recovered.Version, len(recovered.Containers))
	}
}
