package snapshot_commit_rollback

import (
	"coldchain/internal/domain"
	"coldchain/internal/storage"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFailedSnapshotDoesNotCommitEvent(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	base, err := domain.NewCase("case-1", "CC-1", "发送方", "接收方", now, now.Add(time.Hour), now)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Save(base, "create"); err != nil {
		t.Fatal(err)
	}

	updated, err := store.Get(base.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = updated.RegisterContainer(domain.SampleContainer{
		ID: "container-1", ContainerCode: "BOX-1", SealCode: "SEAL-1",
		MinTemperatureC: 2, MaxTemperatureC: 8,
	}); err != nil {
		t.Fatal(err)
	}
	snapshot := filepath.Join(dir, "snapshot.json")
	if err = os.Remove(snapshot); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(snapshot, 0755); err != nil {
		t.Fatal(err)
	}
	if err = store.Save(updated, "container"); err == nil {
		t.Fatal("快照提交失败时应返回错误")
	}

	inMemory, err := store.Get(base.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(inMemory.Containers) != 0 {
		t.Errorf("失败写入不应改变运行中投影，得到 %d 个容器", len(inMemory.Containers))
	}

	recovered, err := storage.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	afterRestart, err := recovered.Get(base.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterRestart.Containers) != 0 {
		t.Fatalf("失败写入不应在重启后重放，得到 %d 个容器", len(afterRestart.Containers))
	}
}
