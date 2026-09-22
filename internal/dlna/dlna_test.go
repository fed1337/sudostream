package dlna

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sudoStream/internal/access"
	"sudoStream/internal/auth"
	"sudoStream/internal/mediafs"
	"sudoStream/internal/playback"
	"sudoStream/internal/transcode"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

const (
	testTVID      = "tv-1"
	testFilmsName = "Films"
	testDLNABase  = "http://127.0.0.1:8200"
)

type memoryKV struct {
	data map[string][]byte
}

func (m *memoryKV) GetSettingValue(_ context.Context, key string) ([]byte, error) {
	raw, ok := m.data[key]
	if !ok {
		return nil, os.ErrNotExist
	}

	return raw, nil
}

func (m *memoryKV) SaveSettingValue(_ context.Context, key string, value []byte) error {
	if m.data == nil {
		m.data = map[string][]byte{}
	}
	m.data[key] = value

	return nil
}

type dlnaAccessStore struct {
	libraries []access.Library
	grants    map[string]map[string]access.LibraryPermissions
}

func (s *dlnaAccessStore) ListLibraries(_ context.Context) ([]access.Library, error) {
	return s.libraries, nil
}

func (s *dlnaAccessStore) GetLibrary(_ context.Context, libraryID string) (access.Library, error) {
	for _, library := range s.libraries {
		if library.ID == libraryID {
			return library, nil
		}
	}

	return access.Library{}, access.ErrLibraryNotFound
}

func (s *dlnaAccessStore) CreateLibrary(
	_ context.Context, library access.Library,
) (access.Library, error) {
	return library, nil
}

func (s *dlnaAccessStore) UpsertLibrary(
	_ context.Context, library access.Library,
) (access.Library, error) {
	return library, nil
}

func (s *dlnaAccessStore) AddRoot(
	_ context.Context, libraryID, relPath string,
) (access.Library, error) {
	return access.Library{ID: libraryID, Roots: []string{relPath}}, nil
}

func (s *dlnaAccessStore) RemoveRoot(
	_ context.Context, libraryID, _ string,
) (access.Library, error) {
	return access.Library{ID: libraryID}, nil
}

func (s *dlnaAccessStore) DeleteLibrary(_ context.Context, _ string) error {
	return nil
}

func (s *dlnaAccessStore) UpdateLibrary(
	_ context.Context, libraryID string, _ *string, _ *access.LibraryType,
) (access.Library, error) {
	return access.Library{ID: libraryID}, nil
}

func (s *dlnaAccessStore) GetUserGrantMap(
	_ context.Context, userID string,
) (map[string]access.LibraryPermissions, error) {
	if s.grants == nil {
		return map[string]access.LibraryPermissions{}, nil
	}

	return s.grants[userID], nil
}

func (s *dlnaAccessStore) ReplaceUserGrants(
	_ context.Context, _ string, _ []access.GrantInput,
) error {
	return nil
}

func TestKVSettingsStore_RoundTrip(t *testing.T) {
	t.Parallel()

	allure.Test(t, "DLNA settings KV round-trips", func(a *allure.Context) {
		t := a.T()
		store := NewKVSettingsStore(&memoryKV{})
		got, err := store.GetSettings(context.Background())
		if err != nil || got.Enabled {
			t.Fatalf("defaults: %+v %v", got, err)
		}
		err = store.SaveSettings(context.Background(), Settings{
			Enabled: true,
			UserID:  testTVID,
			UDN:     "uuid:test",
		})
		if err != nil {
			t.Fatal(err)
		}
		got, err = store.GetSettings(context.Background())
		if err != nil || !got.Enabled || got.UserID != testTVID {
			t.Fatalf("loaded: %+v %v", got, err)
		}
	})
}

func TestDLNAEnvDefaults(t *testing.T) {
	t.Parallel()

	allure.Test(t, "DLNA listen and SSDP defaults from env contract", func(a *allure.Context) {
		t := a.T()
		if DefaultHTTPPort() != 8200 {
			t.Fatalf("port %d", DefaultHTTPPort())
		}
		if DefaultSSDPMulticast() != "239.255.255.250:1900" {
			t.Fatalf("ssdp %s", DefaultSSDPMulticast())
		}
		if listenAddr("", 8200) != ":8200" || listenAddr("192.168.1.2", 8200) != "192.168.1.2:8200" {
			t.Fatal("listenAddr")
		}
		err := applySSDPMulticast("")
		if err != nil {
			t.Fatalf("ssdp multicast: %v", err)
		}
	})
}

func TestTVDeviceProfile_HasDirectPlay(t *testing.T) {
	t.Parallel()

	allure.Test(t, "TV profile includes mp4 h264", func(a *allure.Context) {
		t := a.T()
		profile := TVDeviceProfile()
		if len(profile.DirectPlay) == 0 || profile.PreferHLS {
			t.Fatalf("%+v", profile)
		}
		decision := playback.Decide(profile, playback.SourceCaps{
			Container:  "mp4",
			VideoCodec: "h264",
			AudioCodec: "aac",
			RemuxOK:    true,
		}, 0)
		if decision.Method != playback.MethodDirectPlay {
			t.Fatalf("method %s", decision.Method)
		}
	})
}

func TestBrowser_RootLibraries(t *testing.T) {
	t.Parallel()

	allure.Test(t, "root lists readable film libraries only", func(a *allure.Context) {
		t := a.T()
		store := &dlnaAccessStore{
			libraries: []access.Library{
				{ID: "film-1", Name: testFilmsName, Type: access.LibraryTypeFilm, Roots: []string{"movies"}},
				{ID: "music-1", Name: "Music", Type: access.LibraryTypeMusic, Roots: []string{"music"}},
			},
			grants: map[string]map[string]access.LibraryPermissions{
				testTVID: {"film-1": {Read: true}},
			},
		}
		browser := &Browser{
			Access:  access.NewService(store),
			SignKey: []byte("secret"),
			BaseURL: testDLNABase,
		}
		user := auth.PublicUser{ID: testTVID, Role: auth.RoleTV}
		didl, total, err := browser.BrowseDirectChildren(context.Background(), user, "0", 0, 50)
		if err != nil {
			t.Fatal(err)
		}
		if total != 1 || !strings.Contains(didl, testFilmsName) {
			t.Fatalf("total=%d didl=%s", total, didl)
		}
		if strings.Contains(didl, "Music") {
			t.Fatal("music should be hidden")
		}
	})
}

func TestBrowser_LibraryBranchesAndFiles(t *testing.T) {
	t.Parallel()

	allure.Test(t, "library branches and files tree list videos", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		movies := filepath.Join(root, "movies")
		mkdirErr := os.MkdirAll(movies, 0o750)
		if mkdirErr != nil {
			t.Fatal(mkdirErr)
		}
		videoPath := filepath.Join(movies, "clip.mp4")
		writeErr := os.WriteFile(videoPath, []byte("not-real"), 0o600)
		if writeErr != nil {
			t.Fatal(writeErr)
		}
		media, err := mediafs.New(root)
		if err != nil {
			t.Fatal(err)
		}

		libID := "lib-1"
		store := &dlnaAccessStore{
			libraries: []access.Library{
				{ID: libID, Name: testFilmsName, Type: access.LibraryTypeOther, Roots: []string{"movies"}},
			},
			grants: map[string]map[string]access.LibraryPermissions{
				testTVID: {libID: {Read: true}},
			},
		}
		browser := &Browser{
			Access: access.NewService(store),
			Media:  media,
			Probe: func(_ context.Context, _ string) (transcode.SourceInfo, error) {
				return transcode.SourceInfo{
					VideoCodec:       "h264",
					AudioCodec:       "aac",
					DurationSeconds:  12,
					VideoStreamCount: 1,
				}, nil
			},
			SignKey: []byte("secret-key-32-bytes-minimum!!!!!"),
			BaseURL: "http://192.168.1.2:8200",
			Now:     func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
		}
		user := auth.PublicUser{ID: testTVID, Role: auth.RoleTV}

		libOID := EncodeObjectID(partLib, libID)
		didl, total, err := browser.BrowseDirectChildren(context.Background(), user, libOID, 0, 50)
		if err != nil || total != 2 {
			t.Fatalf("branches total=%d err=%v didl=%s", total, err, didl)
		}

		filesOID := EncodeObjectID(partLib, libID, partFiles)
		didl, total, err = browser.BrowseDirectChildren(context.Background(), user, filesOID, 0, 50)
		if err != nil {
			t.Fatal(err)
		}
		if total != 1 || !strings.Contains(didl, "clip.mp4") {
			t.Fatalf("files total=%d didl=%s", total, didl)
		}
		if !strings.Contains(didl, "/dlna/stream/") {
			t.Fatalf("missing stream url: %s", didl)
		}
	})
}

func TestSOAP_ConnectionManagerAndBrowse(t *testing.T) {
	t.Parallel()

	allure.Test(t, "SOAP ConnectionManager and Browse root", func(a *allure.Context) {
		t := a.T()
		store := &dlnaAccessStore{
			libraries: []access.Library{
				{ID: "a", Name: "Lib", Type: access.LibraryTypeFilm, Roots: []string{"x"}},
			},
			grants: map[string]map[string]access.LibraryPermissions{
				"tv": {"a": {Read: true}},
			},
		}
		handler := &SOAPHandler{
			Browser: &Browser{
				Access:  access.NewService(store),
				SignKey: []byte("k"),
				BaseURL: testDLNABase,
			},
			TVUser: func() (auth.PublicUser, error) {
				return auth.PublicUser{ID: "tv", Role: auth.RoleTV}, nil
			},
		}

		cmBody := `<s:Envelope><s:Body><u:GetProtocolInfo xmlns:u="` + cmServiceType + `"/></s:Body></s:Envelope>`
		req := httptest.NewRequestWithContext(
			context.Background(), http.MethodPost, "/dlna/ConnectionManager/control", bytes.NewBufferString(cmBody),
		)
		req.Header.Set("SoapAction", `"urn:schemas-upnp-org:service:ConnectionManager:1#GetProtocolInfo"`)
		rec := httptest.NewRecorder()
		handler.ServeConnectionManager(rec, req)
		if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("video/mp4")) {
			t.Fatalf("cm: %d %s", rec.Code, rec.Body.String())
		}

		browseBody := `<s:Envelope><s:Body><u:Browse xmlns:u="urn:schemas-upnp-org:service:ContentDirectory:1">` +
			`<ObjectID>0</ObjectID><BrowseFlag>BrowseDirectChildren</BrowseFlag>` +
			`<StartingIndex>0</StartingIndex><RequestedCount>10</RequestedCount></u:Browse></s:Body></s:Envelope>`
		req = httptest.NewRequestWithContext(
			context.Background(), http.MethodPost, "/dlna/ContentDirectory/control", bytes.NewBufferString(browseBody),
		)
		req.Header.Set("SoapAction", `"urn:schemas-upnp-org:service:ContentDirectory:1#Browse"`)
		rec = httptest.NewRecorder()
		handler.ServeContentDirectory(rec, req)
		if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("Lib")) {
			t.Fatalf("browse: %d %s", rec.Code, rec.Body.String())
		}
	})
}

func TestStreamHandler_RejectsBadToken(t *testing.T) {
	t.Parallel()

	allure.Test(t, "stream handler rejects unsigned requests", func(a *allure.Context) {
		t := a.T()
		handler := &StreamHandler{SignKey: []byte("secret")}
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/dlna/stream/movies/a.mp4", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("code %d", rec.Code)
		}
	})
}

func TestDescriptionAndHelpers(t *testing.T) {
	t.Parallel()

	allure.Test(t, "description SCPD and helpers", func(a *allure.Context) {
		t := a.T()
		rec := httptest.NewRecorder()
		writeDescription(rec, "uuid:x")
		if !bytes.Contains(rec.Body.Bytes(), []byte("MediaServer:1")) {
			t.Fatalf("%s", rec.Body.String())
		}
		rec = httptest.NewRecorder()
		writeContentDirectorySCPD(rec)
		if !bytes.Contains(rec.Body.Bytes(), []byte("Browse")) {
			t.Fatal(rec.Body.String())
		}
		rec = httptest.NewRecorder()
		writeConnectionManagerSCPD(rec)
		if !bytes.Contains(rec.Body.Bytes(), []byte("GetProtocolInfo")) {
			t.Fatal(rec.Body.String())
		}
		if EscapeXML(`a&b<"x"`) == `a&b<"x"` {
			t.Fatal("escape failed")
		}
		if formatUPNPDuration(3661) != "1:01:01.000" {
			t.Fatalf("%q", formatUPNPDuration(3661))
		}
		if emptyDIDL() == "" {
			t.Fatal("empty didl")
		}
		raw, _ := json.Marshal(DefaultSettings())
		if string(raw) == "" {
			t.Fatal("defaults")
		}
	})
}

func TestDetectLANIPv4_ReturnsSomething(t *testing.T) {
	t.Parallel()

	allure.Test(t, "DetectLANIPv4 returns an address or ErrNoLANIPv4", func(a *allure.Context) {
		t := a.T()
		ip, err := DetectLANIPv4(context.Background())
		if err != nil && !errors.Is(err, ErrNoLANIPv4) {
			t.Fatalf("%v", err)
		}
		if err == nil && ip == "" {
			t.Fatal("empty ip")
		}
	})
}

func TestStreamHandler_ServeDirect(t *testing.T) {
	t.Parallel()

	allure.Test(t, "signed stream serves original file with Range", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		writeErr := os.WriteFile(filepath.Join(root, "a.mp4"), []byte("abcdef"), 0o600)
		if writeErr != nil {
			t.Fatal(writeErr)
		}
		media, err := mediafs.New(root)
		if err != nil {
			t.Fatal(err)
		}
		secret := []byte("stream-secret-key-32-bytes-min!!")
		now := time.Unix(1_700_000_000, 0).UTC()
		query := SignStream(secret, testTVID, "a.mp4", StreamDirect, now)

		handler := &StreamHandler{
			Access: fakeAccess{allow: true},
			Media:  media,
			Auth: fakeUsers{user: &auth.User{
				ID: testTVID, Role: auth.RoleTV, Enabled: true, Email: "tv@lan",
			}},
			SignKey: secret,
			Now:     func() time.Time { return now },
		}
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/dlna/stream/a.mp4?"+query, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || rec.Body.String() != "abcdef" {
			t.Fatalf("code=%d body=%q", rec.Code, rec.Body.String())
		}
	})
}

type fakeAccess struct{ allow bool }

func (f fakeAccess) CanRead(_ context.Context, _ auth.PublicUser, _ string) (bool, error) {
	return f.allow, nil
}

type fakeUsers struct{ user *auth.User }

func (f fakeUsers) GetUserByID(_ context.Context, _ string) (*auth.User, error) {
	return f.user, nil
}

func TestSOAP_MoreActionsAndFaults(t *testing.T) {
	t.Parallel()

	allure.Test(t, "SOAP helpers cover caps faults and connection info", func(a *allure.Context) {
		t := a.T()
		handler := &SOAPHandler{
			Browser: &Browser{Access: access.NewService(&dlnaAccessStore{}), SignKey: []byte("k")},
			TVUser: func() (auth.PublicUser, error) {
				return auth.PublicUser{ID: "tv", Role: auth.RoleTV}, nil
			},
		}
		for _, action := range []string{"GetSearchCapabilities", "GetSortCapabilities", "GetSystemUpdateID"} {
			body := `<s:Envelope><s:Body><u:` + action + ` xmlns:u="` + cdServiceType + `"/></s:Body></s:Envelope>`
			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", bytes.NewBufferString(body))
			req.Header.Set("SoapAction", `"`+cdServiceType+`#`+action+`"`)
			rec := httptest.NewRecorder()
			handler.ServeContentDirectory(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("%s -> %d %s", action, rec.Code, rec.Body.String())
			}
		}
		for _, action := range []string{"GetCurrentConnectionIDs", "GetCurrentConnectionInfo"} {
			body := `<s:Envelope><s:Body><u:` + action + ` xmlns:u="` + cmServiceType + `"/></s:Body></s:Envelope>`
			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", bytes.NewBufferString(body))
			req.Header.Set("SoapAction", `"`+cmServiceType+`#`+action+`"`)
			rec := httptest.NewRecorder()
			handler.ServeConnectionManager(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("%s -> %d", action, rec.Code)
			}
		}

		faultHandler := &SOAPHandler{
			Browser: &Browser{},
			TVUser:  func() (auth.PublicUser, error) { return auth.PublicUser{}, ErrNotEnabled },
		}
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", bytes.NewBufferString(`<x/>`))
		req.Header.Set("SoapAction", `"#Browse"`)
		rec := httptest.NewRecorder()
		faultHandler.ServeContentDirectory(rec, req)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("fault code %d", rec.Code)
		}

		if StreamModeForDecision(playback.MethodRemux) != StreamRemux {
			t.Fatal("remux")
		}
		if StreamModeForDecision(playback.MethodTranscode) != StreamTranscode {
			t.Fatal("transcode")
		}
		if pathEscapeKeepSlash("a/b c") == "" {
			t.Fatal("escape")
		}
		_, err := DecodeObjectID("!!!")
		if err == nil {
			t.Fatal("expected decode error")
		}
	})
}

func TestBrowser_CatalogNilBranch(t *testing.T) {
	t.Parallel()

	allure.Test(t, "catalog branch empty when catalog service unset", func(a *allure.Context) {
		t := a.T()
		libID := "lib"
		store := &dlnaAccessStore{
			libraries: []access.Library{
				{ID: libID, Name: testFilmsName, Type: access.LibraryTypeFilm, Roots: []string{"m"}},
			},
			grants: map[string]map[string]access.LibraryPermissions{
				testTVID: {libID: {Read: true}},
			},
		}
		browser := &Browser{
			Access:  access.NewService(store),
			SignKey: []byte("k"),
			BaseURL: testDLNABase,
		}
		user := auth.PublicUser{ID: testTVID, Role: auth.RoleTV}
		oid := EncodeObjectID(partLib, libID, partCatalog)
		_, total, err := browser.BrowseDirectChildren(context.Background(), user, oid, 0, 10)
		if err != nil || total != 0 {
			t.Fatalf("total=%d err=%v", total, err)
		}
	})
}

func TestSaveSettings_NilStore(t *testing.T) {
	t.Parallel()

	allure.Test(t, "nil KV store returns ErrSettingsUnavailable", func(a *allure.Context) {
		t := a.T()
		store := &KVSettingsStore{}
		err := store.SaveSettings(context.Background(), Settings{Enabled: false})
		if !errors.Is(err, ErrSettingsUnavailable) {
			t.Fatalf("%v", err)
		}
	})
}

func TestController_ReloadEnableDisable(t *testing.T) { //nolint:paralleltest // SSDP bind is process-global
	allure.Test(t, "controller can enable then disable DLNA listener", func(a *allure.Context) {
		t := a.T()
		settingsStore := NewKVSettingsStore(&memoryKV{})
		err := settingsStore.SaveSettings(context.Background(), Settings{
			Enabled: true,
			UserID:  testTVID,
			UDN:     "uuid:test-dlna",
		})
		if err != nil {
			t.Fatal(err)
		}
		ctrl := NewController(Deps{
			Settings: settingsStore,
			Access:   access.NewService(&dlnaAccessStore{}),
			Auth:     nil, // tvUser only needed on SOAP
			Port:     0,   // will become default then we override
			SignKey:  []byte("secret"),
		})
		ctrl.deps.Port = 18255 + (os.Getpid() % 100)
		err = ctrl.Reload(context.Background())
		if err != nil {
			t.Fatalf("enable: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
		descURL := ctrl.baseURL + "/dlna/description.xml"
		reqDesc, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, descURL, nil)
		resp, getErr := http.DefaultClient.Do(reqDesc)
		if getErr == nil {
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("description status %d", resp.StatusCode)
			}
		}
		err = settingsStore.SaveSettings(context.Background(), Settings{
			Enabled: false, UserID: testTVID, UDN: "uuid:test-dlna",
		})
		if err != nil {
			t.Fatal(err)
		}
		err = ctrl.Reload(context.Background())
		if err != nil {
			t.Fatalf("disable: %v", err)
		}
		_ = ctrl.Shutdown(context.Background())
	})
}

func TestStreamHandler_ServeMPEGTSRemux(t *testing.T) {
	t.Parallel()

	allure.Test(t, "remux mode pipes ffmpeg mpegts", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		// tiny valid-enough input: ffmpeg can remux a generated pattern via file
		src := filepath.Join(root, "in.mp4")
		gen := exec.CommandContext(context.Background(), //nolint:gosec // test fixture
			"ffmpeg", "-hide_banner", "-loglevel", "error",
			"-f", "lavfi", "-i", "color=c=black:s=16x16:d=0.2",
			"-c:v", "libx264", "-pix_fmt", "yuv420p", "-y", src,
		)
		runErr := gen.Run()
		if runErr != nil {
			t.Skip("ffmpeg generate fixture unavailable")
		}
		media, err := mediafs.New(root)
		if err != nil {
			t.Fatal(err)
		}
		secret := []byte("stream-secret-key-32-bytes-min!!")
		now := time.Unix(1_700_000_000, 0).UTC()
		query := SignStream(secret, testTVID, "in.mp4", StreamRemux, now)
		handler := &StreamHandler{
			Access: fakeAccess{allow: true},
			Media:  media,
			Auth: fakeUsers{user: &auth.User{
				ID: testTVID, Role: auth.RoleTV, Enabled: true,
			}},
			SignKey: secret,
			Now:     func() time.Time { return now },
		}
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/dlna/stream/in.mp4?"+query, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || rec.Body.Len() == 0 {
			t.Fatalf("code=%d len=%d", rec.Code, rec.Body.Len())
		}
	})
}

func TestFallbackInterfaceIPv4(t *testing.T) {
	t.Parallel()

	allure.Test(t, "fallbackInterfaceIPv4 returns LAN or ErrNoLANIPv4", func(a *allure.Context) {
		t := a.T()
		ip, err := fallbackInterfaceIPv4()
		if err != nil && !errors.Is(err, ErrNoLANIPv4) {
			t.Fatal(err)
		}
		if err == nil && ip == "" {
			t.Fatal("empty")
		}
	})
}

func TestCoverageExtras(t *testing.T) {
	t.Parallel()

	allure.Test(t, "extra branches for package coverage floor", func(a *allure.Context) {
		t := a.T()
		if StreamModeForDecision(playback.PlayMethod("nope")) != StreamTranscode {
			t.Fatal("default mode")
		}
		store := NewKVSettingsStore(&memoryKV{data: map[string][]byte{"dlna": []byte(`{`)}})
		_, err := store.GetSettings(context.Background())
		if err == nil {
			t.Fatal("expected decode error")
		}
		tag := soapTag([]byte(`<u:ObjectID>abc</u:ObjectID>`), "ObjectID")
		if tag != "abc" {
			t.Fatalf("tag %q", tag)
		}
		if soapActionName("") != "" {
			t.Fatal("empty action")
		}
		ctrl := NewController(Deps{Settings: NewKVSettingsStore(&memoryKV{}), SignKey: []byte("x")})
		_, err = ctrl.tvUser()
		if !errors.Is(err, ErrNotEnabled) {
			t.Fatalf("tvUser: %v", err)
		}
	})
}
