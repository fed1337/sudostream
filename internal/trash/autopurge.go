package trash

import (
	"context"
	"log/slog"
	"time"
)

// StartAutoPurge runs PurgeExpired once every UTC midnight until Shutdown.
func (s *Service) StartAutoPurge() {
	if s == nil {
		return
	}

	s.autoOnce.Do(func() {
		go s.autoPurgeLoop()
	})
}

// Shutdown stops the UTC-midnight auto-purge loop.
func (s *Service) Shutdown() {
	if s == nil {
		return
	}

	s.autoStopOnce.Do(func() {
		close(s.autoStop)
	})
}

func (s *Service) autoPurgeLoop() {
	for {
		wait := time.Until(nextUTCMidnight(time.Now()))
		timer := time.NewTimer(wait)
		select {
		case <-s.autoStop:
			timer.Stop()

			return
		case <-timer.C:
			purged, err := s.PurgeExpired(context.Background())
			if err != nil {
				slog.Error("trash auto-purge failed",
					slog.String("action", "trash.auto_purge"),
					slog.String("err", err.Error()),
				)

				continue
			}
			if purged > 0 {
				slog.Info("trash auto-purge completed",
					slog.String("action", "trash.auto_purge"),
					slog.Int("purged", purged),
				)
			}
		}
	}
}

// nextUTCMidnight returns the next 00:00:00 UTC strictly after now.
func nextUTCMidnight(now time.Time) time.Time {
	now = now.UTC()

	return time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
}
