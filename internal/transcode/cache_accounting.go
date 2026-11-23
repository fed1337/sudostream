package transcode

import (
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// defaultUnusedGrace is how long a package must be idle (and not in jobs/sessions)
// before playback.cache.purge may delete it.
const defaultUnusedGrace = 15 * time.Minute

// touchCache records that cacheKey was used now (memory + top-level dir mtime).
func (s *Service) touchCache(cacheKey string) {
	if s == nil || cacheKey == "" {
		return
	}

	now := time.Now()
	s.mu.Lock()
	if s.lastUsed == nil {
		s.lastUsed = make(map[string]time.Time)
	}
	s.lastUsed[cacheKey] = now
	s.mu.Unlock()

	outDir := s.CacheOutDir(cacheKey)
	_ = os.Chtimes(outDir, now, now)
}

// recordPackageSize walks a package once and updates the running cache byte total.
func (s *Service) recordPackageSize(cacheKey string) {
	if s == nil || cacheKey == "" {
		return
	}

	size := dirSize(s.CacheOutDir(cacheKey))
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.packageBytes == nil {
		s.packageBytes = make(map[string]int64)
	}
	prev := s.packageBytes[cacheKey]
	s.packageBytes[cacheKey] = size
	s.cacheBytes += size - prev
}

func (s *Service) forgetPackageAccounting(cacheKey string) {
	if s == nil || cacheKey == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.packageBytes != nil {
		s.cacheBytes -= s.packageBytes[cacheKey]
		if s.cacheBytes < 0 {
			s.cacheBytes = 0
		}
		delete(s.packageBytes, cacheKey)
	}
	if s.lastUsed != nil {
		delete(s.lastUsed, cacheKey)
	}
}

func (s *Service) packageSizeLocked(cacheKey string) int64 {
	if s.packageBytes == nil {
		return 0
	}

	return s.packageBytes[cacheKey]
}

func (s *Service) lastUsedOf(cacheKey string, fallback time.Time) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.lastUsed != nil {
		if used, ok := s.lastUsed[cacheKey]; ok {
			return used
		}
	}

	return fallback
}

func (s *Service) inUseCacheKeys() map[string]struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()

	inUse := make(map[string]struct{}, len(s.jobs)+len(s.sessions))
	for cacheKey := range s.jobs {
		inUse[cacheKey] = struct{}{}
	}
	for cacheKey := range s.sessions {
		inUse[cacheKey] = struct{}{}
	}

	return inUse
}

// hydrateCacheAccounting measures each existing top-level package once at startup.
func (s *Service) hydrateCacheAccounting() {
	if s == nil || s.cacheDir == "" {
		return
	}

	entries, err := os.ReadDir(s.cacheDir)
	if err != nil {
		return
	}

	s.mu.Lock()
	s.packageBytes = make(map[string]int64)
	s.lastUsed = make(map[string]time.Time)
	s.cacheBytes = 0
	s.mu.Unlock()

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			continue
		}
		key := entry.Name()
		size := dirSize(filepath.Join(s.cacheDir, key))
		s.mu.Lock()
		s.packageBytes[key] = size
		s.cacheBytes += size
		s.lastUsed[key] = info.ModTime()
		s.mu.Unlock()
	}
}

func dirSize(root string) int64 {
	var total int64
	_ = filepath.WalkDir(root, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return nil //nolint:nilerr // skip unreadable entries while summing size
		}
		total += info.Size()

		return nil
	})

	return total
}
