package idempotency_scope_test

import (
	"bytes"
	"coldchain/internal/application"
	"coldchain/internal/httpapi"
	"coldchain/internal/storage"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCrossEndpointIdempotencyKeyDoesNotSuppressAction(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httpapi.New(application.New(store)).Handler())
	defer server.Close()
	now := time.Now().UTC()
	key := "shared-operation-key"
	createBody := map[string]any{
		"caseNumber":         "IDEMP-SCOPE-1",
		"senderName":         "发送方",
		"receiverName":       "接收方",
		"handoffWindowStart": now,
		"handoffWindowEnd":   now.Add(time.Hour),
	}
	status, created := postJSON(t, server.URL+"/api/cases", key, createBody)
	if status != http.StatusCreated {
		t.Fatalf("建案状态码异常: %d", status)
	}
	id, ok := created["id"].(string)
	if !ok || id == "" {
		t.Fatal("建案响应缺少案卷 ID")
	}
	version, ok := created["version"].(float64)
	if !ok {
		t.Fatal("建案响应缺少版本")
	}
	actionBody := map[string]any{
		"expectedVersion": int(version),
		"id":              "box-1",
		"containerCode":   "BOX-1",
		"sealCode":        "SEAL-1",
		"minTemperatureC": 2,
		"maxTemperatureC": 8,
	}
	status, _ = postJSON(t, server.URL+"/api/cases/"+id+"/containers", key, actionBody)
	if status != http.StatusOK {
		t.Fatalf("复用其他接口幂等键不应回放建案结果，动作状态码为 %d", status)
	}
}

func postJSON(t *testing.T, url, key string, body any) (int, map[string]any) {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, out
}
