package listaliasstatepollution

import (
	"coldchain/internal/domain"
	"coldchain/internal/storage"
	"testing"
	"time"
)

func TestListResultCannotMutatePersistedCase(t *testing.T) {
	newStore := func(t *testing.T, id string) *storage.Store {
		t.Helper()
		store, err := storage.New(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		now := time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)
		c, err := domain.NewCase(id, "LIST-ALIAS", "发送方", "接收方", now, now.Add(time.Hour), now)
		if err != nil {
			t.Fatal(err)
		}
		c.Status = domain.StatusReleased
		c.RecordAudit("release", "审核员", "credential issued")
		if err := store.Save(c, "create"); err != nil {
			t.Fatal(err)
		}
		return store
	}

	t.Run("Get", func(t *testing.T) {
		store := newStore(t, "case-get-alias")
		view, err := store.Get("case-get-alias")
		if err != nil {
			t.Fatal(err)
		}
		view.Audit[0].Action = "forged"
		view.Audit[0].Detail = "未持久化的审计修改"
		got, err := store.Get("case-get-alias")
		if err != nil {
			t.Fatal(err)
		}
		if got.Audit[0].Action == "forged" || got.Audit[0].Detail == "未持久化的审计修改" {
			t.Fatalf("点查返回值污染了存储中的审计记录: action=%q detail=%q", got.Audit[0].Action, got.Audit[0].Detail)
		}
	})

	t.Run("List", func(t *testing.T) {
		store := newStore(t, "case-list-alias")
		listed := store.List()
		if len(listed) != 1 {
			t.Fatalf("expected one case, got %d", len(listed))
		}
		listed[0].Audit[0].Action = "forged"
		listed[0].Audit[0].Detail = "未持久化的审计修改"
		got, err := store.Get("case-list-alias")
		if err != nil {
			t.Fatal(err)
		}
		if got.Audit[0].Action == "forged" || got.Audit[0].Detail == "未持久化的审计修改" {
			t.Fatalf("列表返回值污染了存储中的审计记录: action=%q detail=%q", got.Audit[0].Action, got.Audit[0].Detail)
		}
	})
}
