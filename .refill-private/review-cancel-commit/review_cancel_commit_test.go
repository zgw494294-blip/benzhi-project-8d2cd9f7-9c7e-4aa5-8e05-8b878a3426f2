package reviewcancelcommit_test

import (
	"bytes"
	"coldchain/internal/application"
	"coldchain/internal/httpapi"
	"coldchain/internal/storage"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type cancelAfterFirstCheck struct {
	checks int
	done   chan struct{}
	once   sync.Once
}

func (c *cancelAfterFirstCheck) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *cancelAfterFirstCheck) Done() <-chan struct{}       { return c.done }
func (c *cancelAfterFirstCheck) Value(any) any               { return nil }
func (c *cancelAfterFirstCheck) Err() error {
	c.checks++
	if c.checks > 1 {
		c.once.Do(func() { close(c.done) })
		return context.Canceled
	}
	return nil
}

func TestCanceledCredentialReviewDoesNotCommit(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := application.New(store)
	check := app.VerifyCredentials([]string{"missing-credential"})
	if len(check.Items) != 1 {
		t.Fatal("未找到确定性凭据核验结果")
	}
	body, err := json.Marshal(map[string]string{
		"input":       "missing-credential",
		"checkDigest": check.Items[0].CheckDigest,
		"operator":    "核验员",
		"conclusion":  "确认凭据不存在",
		"note":        "等待补证",
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := &cancelAfterFirstCheck{done: make(chan struct{})}
	req := httptest.NewRequest(http.MethodPost, "/api/credentials/reviews", bytes.NewReader(body)).WithContext(ctx)
	recorder := httptest.NewRecorder()
	httpapi.New(app).Handler().ServeHTTP(recorder, req)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("取消请求状态码=%d, body=%s", recorder.Code, recorder.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response["error"] != context.Canceled.Error() {
		t.Fatalf("取消错误未传播: %#v", response)
	}
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatal("测试 context 未进入取消状态")
	}
	if got := app.CredentialReviews("", "CREDENTIAL_NOT_FOUND"); len(got) != 0 {
		t.Fatalf("已取消请求仍提交了 %d 条凭据复核记录", len(got))
	}
}
