package transcode

import (
	"context"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestService_Shutdown_NoActiveJobs(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"shutdown returns immediately when no jobs are running",
		func(a *allure.Context) {
			t := a.T()
			service, err := NewService(t.TempDir())
			if err != nil {
				t.Fatalf("new service: %v", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			err = service.Shutdown(ctx)
			if err != nil {
				t.Fatalf("shutdown: %v", err)
			}
		},
	)
}

func TestService_Shutdown_WaitsForWorkers(t *testing.T) {
	t.Parallel()

	allure.Test(t, "shutdown waits for active workers to exit", func(a *allure.Context) {
		t := a.T()
		service, err := NewService(t.TempDir())
		if err != nil {
			t.Fatalf("new service: %v", err)
		}

		ctx, cancel := service.workerContext()
		defer cancel()

		done := make(chan struct{})

		service.wg.Go(func() {
			<-ctx.Done()
			close(done)
		})

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
		defer shutdownCancel()

		err = service.Shutdown(shutdownCtx)
		if err != nil {
			t.Fatalf("shutdown: %v", err)
		}

		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("worker did not exit after shutdown")
		}
	})
}
