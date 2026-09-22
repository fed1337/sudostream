package network

import (
	"bytes"
	"context"
	"net"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestSocksUDPHeader_RoundTripIPv4(t *testing.T) {
	t.Parallel()

	allure.Test(t, "SOCKS5 UDP header encode/decode preserves IPv4 payload", func(a *allure.Context) {
		t := a.T()
		dst := socksAddr{atyp: socksATYPIPv4, ip: net.IPv4(1, 2, 3, 4), port: 9000}
		payload := []byte("AUTH user=x")
		framed, err := encodeSocksUDPHeader(dst, payload)
		if err != nil {
			t.Fatal(err)
		}
		got, src, err := decodeSocksUDPHeader(framed)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, payload) {
			t.Fatalf("payload=%q want=%q", got, payload)
		}
		udp, ok := src.(*net.UDPAddr)
		if !ok || !udp.IP.Equal(net.IPv4(1, 2, 3, 4)) || udp.Port != 9000 {
			t.Fatalf("src=%v", src)
		}
	})
}

func TestSocksUDPHeader_Domain(t *testing.T) {
	t.Parallel()

	allure.Test(t, "SOCKS5 UDP header encodes socks5h domain ATYP", func(a *allure.Context) {
		t := a.T()
		dst := socksAddr{atyp: socksATYPDomain, host: "api.anidb.net", port: 9000}
		framed, err := encodeSocksUDPHeader(dst, []byte("ping"))
		if err != nil {
			t.Fatal(err)
		}
		got, src, err := decodeSocksUDPHeader(framed)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "ping" {
			t.Fatalf("payload=%q", got)
		}
		if src.String() != "api.anidb.net:9000" {
			t.Fatalf("src=%s", src)
		}
	})
}

func listenUDPEcho(t *testing.T) net.PacketConn {
	t.Helper()
	var listenCfg net.ListenConfig
	echo, err := listenCfg.ListenPacket(t.Context(), "udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = echo.Close() })

	return echo
}

func TestDirectUDPDialer_Echo(t *testing.T) {
	t.Parallel()

	allure.Test(t, "DirectUDPDialer reaches a local UDP echo server", func(a *allure.Context) {
		t := a.T()
		echo := listenUDPEcho(t)
		go func() {
			buf := make([]byte, 2048)
			for {
				n, addr, readErr := echo.ReadFrom(buf)
				if readErr != nil {
					return
				}
				_, _ = echo.WriteTo(buf[:n], addr)
			}
		}()

		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
		defer cancel()
		conn, remote, err := DirectUDPDialer{}.DialUDP(ctx, echo.LocalAddr().String())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = conn.Close() })

		msg := []byte("echo-me")
		_, err = conn.WriteTo(msg, remote)
		if err != nil {
			t.Fatal(err)
		}
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		buf := make([]byte, 64)
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(buf[:n], msg) {
			t.Fatalf("got=%q", buf[:n])
		}
	})
}

func TestLive_DialUDP_DirectWhenHTTPProxy(t *testing.T) {
	t.Parallel()

	allure.Test(t, "Live DialUDP uses direct path when proxy is HTTP", func(a *allure.Context) {
		t := a.T()
		echo := listenUDPEcho(t)
		go func() {
			buf := make([]byte, 512)
			n, addr, readErr := echo.ReadFrom(buf)
			if readErr != nil {
				return
			}
			_, _ = echo.WriteTo(append([]byte("ok:"), buf[:n]...), addr)
		}()

		live := NewLive(Settings{ProxyURL: "http://127.0.0.1:1"})
		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
		defer cancel()
		conn, remote, err := live.DialUDP(ctx, echo.LocalAddr().String())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = conn.Close() })
		_, err = conn.WriteTo([]byte("x"), remote)
		if err != nil {
			t.Fatal(err)
		}
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		buf := make([]byte, 64)
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			t.Fatal(err)
		}
		if string(buf[:n]) != "ok:x" {
			t.Fatalf("got=%q", buf[:n])
		}
	})
}

func TestEncodeSocksAddr_RejectsBadDomain(t *testing.T) {
	t.Parallel()

	allure.Test(t, "encodeSocksAddr rejects empty domain", func(a *allure.Context) {
		t := a.T()
		_, err := encodeSocksAddr(socksAddr{atyp: socksATYPDomain, host: "", port: 1})
		if err == nil {
			t.Fatal("expected error")
		}
	})
}
