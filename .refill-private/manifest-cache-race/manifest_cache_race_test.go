package manifest_cache_race_test

import (
	"coldchain/internal/application"
	"coldchain/internal/domain"
	"coldchain/internal/storage"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestConcurrentManifestCacheIsRaceFree(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	const workers = 24
	ids := make([]string, workers)
	for i := range ids {
		id := fmt.Sprintf("case-manifest-%02d", i)
		caseFile, createErr := domain.NewCase(id, fmt.Sprintf("CC-MANIFEST-%02d", i), "发送方", "接收方", now, now.Add(6*time.Hour), now)
		if createErr != nil {
			t.Fatal(createErr)
		}
		caseFile.Status = domain.StatusApproved
		caseFile.Version = 7
		caseFile.Containers = []domain.SampleContainer{{ID: "box", CaseID: id, ContainerCode: "BOX", SealCode: "SEAL", MinTemperatureC: 2, MaxTemperatureC: 8}}
		if saveErr := store.Save(caseFile, "approved-fixture"); saveErr != nil {
			t.Fatal(saveErr)
		}
		ids[i] = id
	}

	service := application.New(store)
	start := make(chan struct{})
	var ready sync.WaitGroup
	var done sync.WaitGroup
	ready.Add(workers)
	done.Add(workers)
	for _, id := range ids {
		go func(caseID string) {
			defer done.Done()
			ready.Done()
			<-start
			entries, digest, manifestErr := service.Manifest(caseID)
			if manifestErr != nil {
				t.Errorf("查询 %s 的清单失败: %v", caseID, manifestErr)
				return
			}
			if len(entries) == 0 || digest == "" {
				t.Errorf("查询 %s 返回空清单", caseID)
			}
		}(id)
	}
	ready.Wait()
	close(start)
	done.Wait()
}
