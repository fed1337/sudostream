package metadata

import (
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestIndexer_Shutdown_NoActiveJobs(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"shutdown returns immediately when no indexing is running",
		func(a *allure.Context) {
			t := a.T()
			indexer := NewIndexer(nil, nil)

			done := make(chan struct{})

			go func() {
				indexer.Shutdown()
				close(done)
			}()

			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("shutdown did not return")
			}
		},
	)
}

func TestIndexer_Shutdown_WaitsForWorkers(t *testing.T) {
	t.Parallel()

	allure.Test(t, "shutdown waits for active indexing goroutines", func(a *allure.Context) {
		t := a.T()
		indexer := NewIndexer(nil, nil)
		ctx, cancel := indexer.backgroundContext()
		defer cancel()

		released := make(chan struct{})

		indexer.wg.Go(func() {
			<-ctx.Done()
			close(released)
		})

		done := make(chan struct{})

		go func() {
			indexer.Shutdown()
			close(done)
		}()

		select {
		case <-released:
		case <-time.After(time.Second):
			t.Fatal("worker was not cancelled")
		}

		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("shutdown did not return")
		}
	})
}
