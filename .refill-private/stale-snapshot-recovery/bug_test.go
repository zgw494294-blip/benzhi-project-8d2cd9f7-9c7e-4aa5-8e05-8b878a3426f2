package stalesnapshotrecovery

import (
	"coldchain/internal/application"
	"coldchain/internal/domain"
	"coldchain/internal/storage"
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestStaleSnapshotCannotRollBackCommittedEvents(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	service := application.New(store)
	now := time.Now().UTC().Truncate(time.Second)
	c, err := service.Create("RECOVERY-1", "发送方", "接收方", now, now.Add(6*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	stale, err := store.Get(c.ID)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := service.RegisterContainer(c.ID, c.Version, domain.SampleContainer{
		ID: "box-1", ContainerCode: "BOX-1", SealCode: "SEAL-1", MinTemperatureC: 2, MaxTemperatureC: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version <= stale.Version || len(updated.Containers) != 1 {
		t.Fatalf("测试前提未建立: stale=%d updated=%d containers=%d", stale.Version, updated.Version, len(updated.Containers))
	}
	b, err := json.Marshal(stale)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.SnapshotPath(), b, 0644); err != nil {
		t.Fatal(err)
	}

	recovered, err := storage.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := recovered.Get(c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != updated.Version || len(got.Containers) != 1 {
		t.Fatalf("重启恢复回退到旧快照: got version=%d containers=%d, want version=%d containers=1", got.Version, len(got.Containers), updated.Version)
	}
}
