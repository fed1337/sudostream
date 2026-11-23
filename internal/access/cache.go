package access

import (
	"context"
	"fmt"
	"maps"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// DefaultACLCacheTTL is the maximum age for in-process library/grant snapshots.
const DefaultACLCacheTTL = 15 * time.Minute

type librariesEntry struct {
	libraries []Library
	expires   time.Time
}

type grantsEntry struct {
	grants  map[string]LibraryPermissions
	expires time.Time
}

// aclCache holds short-lived library and per-user grant snapshots.
// Invalidated on library/grant admin writes; TTL is a safety net.
type aclCache struct {
	mu         sync.RWMutex
	ttl        time.Duration
	now        func() time.Time
	libraries  *librariesEntry
	grants     map[string]grantsEntry
	libGroup   singleflight.Group
	grantGroup singleflight.Group
}

func newACLCache(ttl time.Duration) *aclCache {
	if ttl <= 0 {
		ttl = DefaultACLCacheTTL
	}

	return &aclCache{
		ttl:    ttl,
		now:    time.Now,
		grants: make(map[string]grantsEntry),
	}
}

func (c *aclCache) getLibraries() ([]Library, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.libraries == nil || c.now().After(c.libraries.expires) {
		return nil, false
	}

	return cloneLibraries(c.libraries.libraries), true
}

func (c *aclCache) setLibraries(libraries []Library) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.libraries = &librariesEntry{
		libraries: cloneLibraries(libraries),
		expires:   c.now().Add(c.ttl),
	}
}

func (c *aclCache) getGrants(userID string) (map[string]LibraryPermissions, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.grants[userID]
	if !ok || c.now().After(entry.expires) {
		return nil, false
	}

	return cloneGrantMap(entry.grants), true
}

func (c *aclCache) setGrants(userID string, grants map[string]LibraryPermissions) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.grants[userID] = grantsEntry{
		grants:  cloneGrantMap(grants),
		expires: c.now().Add(c.ttl),
	}
}

func (c *aclCache) invalidateLibraries() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.libraries = nil
	// Library root changes affect MatchLibrary for every user; drop grant maps too
	// so stale library IDs cannot linger after delete/merge.
	c.grants = make(map[string]grantsEntry)
}

func (c *aclCache) invalidateUserGrants(userID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.grants, userID)
}

func (c *aclCache) invalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.libraries = nil
	c.grants = make(map[string]grantsEntry)
}

func cloneLibraries(libraries []Library) []Library {
	if libraries == nil {
		return nil
	}

	out := make([]Library, len(libraries))
	for i, library := range libraries {
		out[i] = library
		if library.Roots != nil {
			out[i].Roots = append([]string(nil), library.Roots...)
		}
	}

	return out
}

func cloneGrantMap(grants map[string]LibraryPermissions) map[string]LibraryPermissions {
	if grants == nil {
		return map[string]LibraryPermissions{}
	}

	out := make(map[string]LibraryPermissions, len(grants))
	maps.Copy(out, grants)

	return out
}

func (s *Service) loadLibraries(ctx context.Context) ([]Library, error) {
	if cached, ok := s.cache.getLibraries(); ok {
		return cached, nil
	}

	value, err, _ := s.cache.libGroup.Do("libraries", func() (any, error) {
		if cached, ok := s.cache.getLibraries(); ok {
			return cached, nil
		}

		libraries, listErr := s.store.ListLibraries(ctx)
		if listErr != nil {
			return nil, fmt.Errorf("list libraries: %w", listErr)
		}
		s.cache.setLibraries(libraries)

		return cloneLibraries(libraries), nil
	})
	if err != nil {
		return nil, fmt.Errorf("libraries cache: %w", err)
	}

	libraries, _ := value.([]Library)

	return libraries, nil
}

func (s *Service) loadGrants(ctx context.Context, userID string) (map[string]LibraryPermissions, error) {
	if cached, ok := s.cache.getGrants(userID); ok {
		return cached, nil
	}

	value, err, _ := s.cache.grantGroup.Do(userID, func() (any, error) {
		if cached, ok := s.cache.getGrants(userID); ok {
			return cached, nil
		}

		grants, grantErr := s.store.GetUserGrantMap(ctx, userID)
		if grantErr != nil {
			return nil, fmt.Errorf("load grants: %w", grantErr)
		}
		if grants == nil {
			grants = map[string]LibraryPermissions{}
		}
		s.cache.setGrants(userID, grants)

		return cloneGrantMap(grants), nil
	})
	if err != nil {
		return nil, fmt.Errorf("grants cache: %w", err)
	}

	grants, _ := value.(map[string]LibraryPermissions)

	return grants, nil
}
