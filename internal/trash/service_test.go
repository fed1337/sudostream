package trash

import (
	"context"
	"os"
	"path/filepath"
	"sudoStream/internal/mediafs"
	"sync"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

type memoryStore struct {
	mu    sync.Mutex
	items map[string]Item
}

func newMemoryStore() *memoryStore {
	return &memoryStore{items: map[string]Item{}}
}

func (m *memoryStore) Insert(_ context.Context, item Item) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[item.ID] = item

	return nil
}

func (m *memoryStore) Get(_ context.Context, id string) (Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.items[id]
	if !ok {
		return Item{}, ErrNotFound
	}

	return item, nil
}

func (m *memoryStore) List(_ context.Context) ([]Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Item, 0, len(m.items))
	for _, item := range m.items {
		out = append(out, item)
	}

	return out, nil
}

func (m *memoryStore) Delete(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.items, id)

	return nil
}

func (m *memoryStore) OriginalPrefixes(_ context.Context) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.items))
	for _, item := range m.items {
		out = append(out, item.OriginalRelPath)
	}

	return out, nil
}

type memoryKV struct {
	raw []byte
}

func (m *memoryKV) GetSettingValue(_ context.Context, _ string) ([]byte, error) {
	if len(m.raw) == 0 {
		return nil, os.ErrNotExist
	}

	return m.raw, nil
}

func (m *memoryKV) SaveSettingValue(_ context.Context, _ string, value []byte) error {
	m.raw = append([]byte(nil), value...)

	return nil
}

func TestService_MoveRestoreDeleteForever(t *testing.T) { //nolint:cyclop
	t.Parallel()

	allure.Test(
		t,
		"trash move hides a file; restore puts it back; forever removes it",
		func(a *allure.Context) {
			t := a.T()
			root := t.TempDir()
			media, err := mediafs.New(root)
			if err != nil {
				t.Fatalf("media: %v", err)
			}

			clip := filepath.Join(root, "movies", "clip.mp4")
			err = os.MkdirAll(filepath.Dir(clip), 0o750)
			if err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			err = os.WriteFile(clip, []byte("video"), 0o600)
			if err != nil {
				t.Fatalf("write: %v", err)
			}

			svc := NewService(media, newMemoryStore(), nil, &memoryKV{})
			ctx := context.Background()
			item, err := svc.Move(ctx, "movies/clip.mp4", "")
			if err != nil {
				t.Fatalf("move: %v", err)
			}
			_, err = os.Stat(clip)
			if !os.IsNotExist(err) {
				t.Fatal("expected source gone")
			}

			prefixes, err := svc.OriginalPrefixes(ctx)
			if err != nil || len(prefixes) != 1 || prefixes[0] != "movies/clip.mp4" {
				t.Fatalf("prefixes: %v %v", prefixes, err)
			}

			_, err = svc.Restore(ctx, item.ID)
			if err != nil {
				t.Fatalf("restore: %v", err)
			}
			_, err = os.Stat(clip)
			if err != nil {
				t.Fatalf("restored: %v", err)
			}

			item, err = svc.Move(ctx, "movies/clip.mp4", "")
			if err != nil {
				t.Fatalf("move 2: %v", err)
			}
			err = svc.DeleteForever(ctx, item.ID)
			if err != nil {
				t.Fatalf("forever: %v", err)
			}
			_, err = os.Stat(clip)
			if !os.IsNotExist(err) {
				t.Fatal("expected gone forever")
			}
			_, err = os.Stat(filepath.Join(root, mediafs.TrashDirName, item.ID))
			if !os.IsNotExist(err) {
				t.Fatal("expected trash dir gone")
			}
		},
	)
}

func TestService_PurgeExpiredNoopWhenRetentionOff(t *testing.T) {
	t.Parallel()

	allure.Test(t, "purge is a no-op when retentionDays is 0", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		media, err := mediafs.New(root)
		if err != nil {
			t.Fatalf("media: %v", err)
		}
		err = os.MkdirAll(filepath.Join(root, "movies"), 0o750)
		if err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		err = os.WriteFile(filepath.Join(root, "movies", "clip.mp4"), []byte("x"), 0o600)
		if err != nil {
			t.Fatalf("write: %v", err)
		}

		store := newMemoryStore()
		settingsKV := &memoryKV{}
		svc := NewService(media, store, nil, settingsKV)
		ctx := context.Background()
		item, err := svc.Move(ctx, "movies/clip.mp4", "")
		if err != nil {
			t.Fatalf("move: %v", err)
		}

		purged, err := svc.PurgeExpired(ctx)
		if err != nil || purged != 0 {
			t.Fatalf("purge default: n=%d err=%v", purged, err)
		}

		err = settingsKV.SaveSettingValue(ctx, settingsKey, []byte(`{"retentionDays":1}`))
		if err != nil {
			t.Fatalf("save settings: %v", err)
		}
		item.DeletedAt = time.Now().UTC().Add(-48 * time.Hour)
		store.items[item.ID] = item

		purged, err = svc.PurgeExpired(ctx)
		if err != nil || purged != 1 {
			t.Fatalf("purge expired: n=%d err=%v", purged, err)
		}
	})
}

func TestNormalizeSettings(t *testing.T) {
	t.Parallel()

	allure.Test(t, "retentionDays 0 is valid; negatives are rejected", func(a *allure.Context) {
		t := a.T()
		got, err := NormalizeSettings(Settings{RetentionDays: 0})
		if err != nil || got.RetentionDays != 0 {
			t.Fatalf("zero: %v %+v", err, got)
		}
		_, err = NormalizeSettings(Settings{RetentionDays: -1})
		if err == nil {
			t.Fatal("expected invalid")
		}
	})
}

type recordingCleaner struct {
	last Item
}

func (r *recordingCleaner) OnPermanentDelete(_ context.Context, item Item) error {
	r.last = item

	return nil
}

func TestService_EmptyAllSaveSettingsAndReconcile(t *testing.T) { //nolint:cyclop
	t.Parallel()

	allure.Test(
		t,
		"empty all, save settings, recover lost rows from info.json, run cleaner",
		func(a *allure.Context) {
			t := a.T()
			root := t.TempDir()
			media, err := mediafs.New(root)
			if err != nil {
				t.Fatalf("media: %v", err)
			}
			err = os.MkdirAll(filepath.Join(root, "movies"), 0o750)
			if err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			err = os.WriteFile(filepath.Join(root, "movies", "a.mp4"), []byte("a"), 0o600)
			if err != nil {
				t.Fatalf("write a: %v", err)
			}
			err = os.WriteFile(filepath.Join(root, "movies", "b.mp4"), []byte("b"), 0o600)
			if err != nil {
				t.Fatalf("write b: %v", err)
			}

			store := newMemoryStore()
			settingsKV := &memoryKV{}
			svc := NewService(media, store, nil, settingsKV)
			cleaner := &recordingCleaner{}
			svc.SetCleaner(cleaner)
			ctx := context.Background()

			saved, err := svc.SaveSettings(ctx, Settings{RetentionDays: 7})
			if err != nil || saved.RetentionDays != 7 {
				t.Fatalf("save settings: %+v %v", saved, err)
			}
			loaded, err := svc.GetSettings(ctx)
			if err != nil || loaded.RetentionDays != 7 {
				t.Fatalf("get settings: %+v %v", loaded, err)
			}

			itemA, err := svc.Move(ctx, "movies/a.mp4", "user-1")
			if err != nil {
				t.Fatalf("move a: %v", err)
			}
			itemB, err := svc.Move(ctx, "movies/b.mp4", "")
			if err != nil {
				t.Fatalf("move b: %v", err)
			}

			store.mu.Lock()
			delete(store.items, itemB.ID)
			store.mu.Unlock()

			listed, err := svc.List(ctx)
			if err != nil || len(listed) != 2 {
				t.Fatalf("reconcile list: n=%d err=%v", len(listed), err)
			}

			n, err := svc.EmptyAll(ctx)
			if err != nil || n != 2 {
				t.Fatalf("empty: n=%d err=%v", n, err)
			}
			if cleaner.last.ID != itemA.ID && cleaner.last.ID != itemB.ID {
				t.Fatalf("cleaner last=%q", cleaner.last.ID)
			}

			listed, err = svc.List(ctx)
			if err != nil || len(listed) != 0 {
				t.Fatalf("after empty: n=%d err=%v", len(listed), err)
			}
		},
	)
}

func TestService_NilGuardsAndCorruptSettings(t *testing.T) {
	t.Parallel()

	allure.Test(t, "nil service and corrupt KV fall back safely", func(a *allure.Context) {
		t := a.T()
		ctx := context.Background()

		var svc *Service
		svc.SetCleaner(nil)
		prefixes, err := svc.OriginalPrefixes(ctx)
		if err != nil || prefixes != nil {
			t.Fatalf("nil prefixes: %v %v", prefixes, err)
		}
		_, err = svc.SaveSettings(ctx, Settings{})
		if err == nil {
			t.Fatal("expected settings unavailable")
		}

		settingsKV := &memoryKV{raw: []byte(`not-json`)}
		wired := NewService(nil, newMemoryStore(), nil, settingsKV)
		got, err := wired.GetSettings(ctx)
		if err != nil || got.RetentionDays != 0 {
			t.Fatalf("corrupt kv: %+v %v", got, err)
		}

		item := itemFromInfo(mediafs.TrashInfo{
			ID:              "11111111-1111-1111-1111-111111111111",
			OriginalRelPath: "movies/clip.mp4",
			TrashRelPath:    ".trash/11111111-1111-1111-1111-111111111111/movies/clip.mp4",
		})
		if item.DeletedAt.IsZero() {
			t.Fatal("expected deletedAt fallback")
		}
	})
}
