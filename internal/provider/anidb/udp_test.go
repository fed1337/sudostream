package anidb_test

import (
	"context"
	"net"
	"strings"
	"sudoStream/internal/provider"
	"sudoStream/internal/provider/anidb"
	"sync"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

type udpScriptDialer struct {
	mu        sync.Mutex
	fileBody  string
	authFail  bool
	banned    bool
	dialErr   error
	lastWrite string
}

func (d *udpScriptDialer) DialUDP(
	_ context.Context,
	_ string,
) (net.PacketConn, net.Addr, error) {
	if d.dialErr != nil {
		return nil, nil, d.dialErr
	}
	remote := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9000}

	return &scriptPacketConn{dialer: d, remote: remote}, remote, nil
}

type scriptPacketConn struct {
	dialer *udpScriptDialer
	remote net.Addr
	reply  []byte
}

func (c *scriptPacketConn) ReadFrom(p []byte) (int, net.Addr, error) {
	if len(c.reply) == 0 {
		return 0, c.remote, net.ErrClosed
	}
	n := copy(p, c.reply)
	c.reply = nil

	return n, c.remote, nil
}

func (c *scriptPacketConn) WriteTo(payload []byte, _ net.Addr) (int, error) {
	cmd := string(payload)
	c.dialer.mu.Lock()
	c.dialer.lastWrite = cmd
	c.dialer.mu.Unlock()

	switch {
	case strings.HasPrefix(cmd, "AUTH "):
		switch {
		case c.dialer.banned:
			c.reply = []byte("555 BANNED test")
		case c.dialer.authFail:
			c.reply = []byte("500 LOGIN FAILED")
		default:
			c.reply = []byte("200 testsess LOGIN ACCEPTED")
		}
	case strings.HasPrefix(cmd, "FILE "):
		if c.dialer.fileBody != "" {
			c.reply = []byte(c.dialer.fileBody)
		} else {
			c.reply = []byte("320 NO SUCH FILE")
		}
	default:
		c.reply = []byte("505 ILLEGAL INPUT OR ACCESS DENIED")
	}

	return len(payload), nil
}

func (c *scriptPacketConn) Close() error { return nil }
func (c *scriptPacketConn) LocalAddr() net.Addr {
	return &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0}
}
func (c *scriptPacketConn) SetDeadline(time.Time) error      { return nil }
func (c *scriptPacketConn) SetReadDeadline(time.Time) error  { return nil }
func (c *scriptPacketConn) SetWriteDeadline(time.Time) error { return nil }

func TestMatchFileHash_OK(t *testing.T) { //nolint:cyclop // field assertions
	t.Parallel()

	allure.Test(t, "anidb MatchFileHash maps FILE reply and enriches via HTTP anime", func(a *allure.Context) {
		t := a.T()
		server := newAniDBServer(t)
		dialer := &udpScriptDialer{
			fileBody: "220 FILE 99|1|10|100|" +
				"aabbccddeeff00112233445566778899|md5|26|" +
				"Cowboy Bebop||Cowboy Bebop|1|Asteroid Blues|Asteroid Blues|",
		}
		adapter := anidb.New(
			anidb.WithBaseURL(server.URL),
			anidb.WithTitlesURL(server.URL+"/anime-titles.xml"),
			anidb.WithCacheDir(t.TempDir()),
			anidb.WithMinInterval(0),
			anidb.WithHTTPClient(server.Client()),
			anidb.WithClient("testclient"),
			anidb.WithCredentials("user", "pass"),
			anidb.WithUDPDialer(dialer),
			anidb.WithUDPAddr("127.0.0.1:9000"),
		)

		status, fields, err := adapter.MatchFileHash(t.Context(), provider.FileHashHint{
			Ed2kHash:     "aabbccddeeff00112233445566778899",
			HashFileSize: 100,
		})
		if err != nil || status != provider.MatchOK {
			t.Fatalf("status=%v err=%v", status, err)
		}
		if fields.IDs.AnidbID == nil || *fields.IDs.AnidbID != "1" {
			t.Fatalf("ids: %+v", fields.IDs)
		}
		if fields.Show == nil || *fields.Show != cowboyBebop {
			t.Fatalf("show: %+v", fields.Show)
		}
		if fields.Episode == nil || *fields.Episode != 1 {
			t.Fatalf("episode: %+v", fields.Episode)
		}
		if fields.EpisodeTitle == nil || *fields.EpisodeTitle == "" {
			t.Fatalf("episode title: %+v", fields.EpisodeTitle)
		}
	})
}

func TestMatchFileHash_NoCreds(t *testing.T) {
	t.Parallel()

	allure.Test(t, "anidb MatchFileHash returns MatchNone when UDP creds missing", func(a *allure.Context) {
		t := a.T()
		adapter := anidb.New(
			anidb.WithMinInterval(0),
			anidb.WithClient("testclient"),
			anidb.WithCredentials("", ""),
			anidb.WithUDPDialer(&udpScriptDialer{fileBody: "220 FILE 1|1|1|1|x|x|1|A|B|C|1|E|||"}),
		)
		status, fields, err := adapter.MatchFileHash(t.Context(), provider.FileHashHint{
			Ed2kHash:     "aabb",
			HashFileSize: 1,
		})
		if err != nil || status != provider.MatchNone {
			t.Fatalf("status=%v err=%v fields=%+v", status, err, fields)
		}
	})
}

func TestMatchFileHash_DialFailFallsBack(t *testing.T) {
	t.Parallel()

	allure.Test(t, "anidb MatchFileHash soft-fails when DialUDP errors", func(a *allure.Context) {
		t := a.T()
		adapter := anidb.New(
			anidb.WithMinInterval(0),
			anidb.WithClient("testclient"),
			anidb.WithCredentials("u", "p"),
			anidb.WithUDPDialer(&udpScriptDialer{dialErr: net.ErrClosed}),
		)
		status, _, err := adapter.MatchFileHash(t.Context(), provider.FileHashHint{
			Ed2kHash:     "aabb",
			HashFileSize: 10,
		})
		if err != nil || status != provider.MatchNone {
			t.Fatalf("status=%v err=%v", status, err)
		}
	})
}

func TestMatchShow_StillWorksWithoutUDP(t *testing.T) {
	t.Parallel()

	allure.Test(t, "anidb MatchShow title path works without UDP dialer or creds", func(a *allure.Context) {
		t := a.T()
		server := newAniDBServer(t)
		adapter := anidb.New(
			anidb.WithBaseURL(server.URL),
			anidb.WithTitlesURL(server.URL+"/anime-titles.xml"),
			anidb.WithCacheDir(t.TempDir()),
			anidb.WithMinInterval(0),
			anidb.WithHTTPClient(server.Client()),
			anidb.WithClient("testclient"),
		)
		status, fields, err := adapter.MatchShow(t.Context(), provider.ShowHint{Show: cowboyBebop})
		if err != nil || status != provider.MatchOK || fields.Title == nil {
			t.Fatalf("status=%v err=%v fields=%+v", status, err, fields)
		}
	})
}

func TestMatchFileHash_NoSuchFile(t *testing.T) {
	t.Parallel()

	allure.Test(t, "anidb MatchFileHash returns MatchNone on 320 NO SUCH FILE", func(a *allure.Context) {
		t := a.T()
		adapter := anidb.New(
			anidb.WithMinInterval(0),
			anidb.WithClient("testclient"),
			anidb.WithCredentials("u", "p"),
			anidb.WithUDPDialer(&udpScriptDialer{fileBody: "320 NO SUCH FILE"}),
			anidb.WithSession("forced"),
		)
		status, _, err := adapter.MatchFileHash(t.Context(), provider.FileHashHint{
			Ed2kHash:     "deadbeef",
			HashFileSize: 42,
		})
		if err != nil || status != provider.MatchNone {
			t.Fatalf("status=%v err=%v", status, err)
		}
	})
}
