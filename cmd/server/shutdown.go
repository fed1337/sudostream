package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sudoStream/internal/db"
	"sudoStream/internal/dlna"
	"sudoStream/internal/httpapi"
	"sudoStream/internal/maintenance"
	"sudoStream/internal/metadata"
	"syscall"
	"time"
)

const (
	shutdownTimeout   = 10 * time.Second
	readHeaderTimeout = 10 * time.Second
)

func listenUntilSignal(srv *http.Server) error {
	errCh := make(chan error, 1)

	go func() {
		err := srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return err
	case sig := <-quit:
		log.Printf("shutdown signal received: %s", sig)

		return nil
	}
}

func shutdownServer(srv *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	err := srv.Shutdown(ctx)
	if err != nil {
		log.Printf("server shutdown: %v", err)
	}
}

func shutdownBackground( //nolint:cyclop // sequential optional shutdown steps
	ctx context.Context,
	workers *httpapi.Workers,
	metadataService *metadata.Service,
	maintenanceService *maintenance.Service,
	database *db.Database,
	dlnaController *dlna.Controller,
) {
	if dlnaController != nil {
		err := dlnaController.Shutdown(ctx)
		if err != nil {
			log.Printf("dlna shutdown: %v", err)
		}
	}

	if metadataService != nil {
		indexer := metadataService.Indexer()
		if indexer != nil {
			indexer.Shutdown()
		}
	}

	if maintenanceService != nil {
		maintenanceService.Shutdown()
	}
	if workers != nil && workers.Maintenance != nil {
		workers.Maintenance.Shutdown()
	}

	if workers != nil && workers.Transcode != nil {
		err := workers.Transcode.Shutdown(ctx)
		if err != nil {
			log.Printf("transcode shutdown: %v", err)
		}
	}

	if database != nil {
		database.Close()
	}
}
