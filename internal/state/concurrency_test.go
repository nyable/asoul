package state_test

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"asoul/internal/state"
)

func TestConcurrentDeploymentUpdatesDoNotLoseRecords(t *testing.T) {
	manager, err := state.NewManager(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			if err := manager.RecordDeployment("target", fmt.Sprintf("skill-%d", i), "hash"); err != nil {
				t.Error(err)
			}
		}(i)
	}
	close(start)
	wg.Wait()

	st, err := manager.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := len(st.Targets["target"]); got != 32 {
		t.Fatalf("expected 32 records, got %d", got)
	}
}
