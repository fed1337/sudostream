package network

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
)

const schemeSOCKS5H = "socks5h"

// UDPDialer prepares a UDP packet path (direct or via SOCKS5 UDP ASSOCIATE).
type UDPDialer interface {
	DialUDP(ctx context.Context, hostPort string) (conn net.PacketConn, remote net.Addr, err error)
}

// DirectUDPDialer always dials UDP without a proxy (tests / no-proxy paths).
type DirectUDPDialer struct{}

// DialUDP implements UDPDialer with a local unbound UDP socket.
func (DirectUDPDialer) DialUDP(ctx context.Context, hostPort string) (net.PacketConn, net.Addr, error) {
	return dialUDPDirect(ctx, hostPort)
}

// DialUDP prepares a UDP packet path to hostPort (e.g. "api.anidb.net:9000").
//
//   - No proxy / HTTP(S) proxy / NoProxy bypass → ListenPacket("udp") + resolved UDP addr
//   - socks5 → UDP ASSOCIATE; resolve host locally; SOCKS header uses IPv4/IPv6
//   - socks5h → UDP ASSOCIATE; SOCKS header uses ATYP domain (no local resolve)
func (l *Live) DialUDP(ctx context.Context, hostPort string) (net.PacketConn, net.Addr, error) {
	hostPort = strings.TrimSpace(hostPort)
	if hostPort == "" {
		return nil, nil, fmt.Errorf("%w: empty hostPort", ErrInvalidProxyURL)
	}
	host, _, err := net.SplitHostPort(hostPort)
	if err != nil {
		return nil, nil, fmt.Errorf("split hostPort: %w", err)
	}

	settings := l.load()
	if settings.ProxyURL == "" || !settings.IsSOCKS() || hostBypassed(host, settings.NoProxy) {
		return dialUDPDirect(ctx, hostPort)
	}

	return dialUDPSocks(ctx, settings, hostPort)
}

func dialUDPDirect(ctx context.Context, hostPort string) (net.PacketConn, net.Addr, error) {
	udpAddr, err := resolveUDPAddr(ctx, hostPort)
	if err != nil {
		return nil, nil, err
	}
	var listenCfg net.ListenConfig
	packet, err := listenCfg.ListenPacket(ctx, "udp", "")
	if err != nil {
		return nil, nil, fmt.Errorf("listen udp: %w", err)
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = packet.SetDeadline(deadline)
	}

	return packet, udpAddr, nil
}

func dialUDPSocks(
	ctx context.Context,
	settings Settings,
	hostPort string,
) (net.PacketConn, net.Addr, error) {
	parsed, err := url.Parse(settings.ProxyURL)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", ErrInvalidProxyURL, err)
	}
	proxyHost := parsed.Host
	if proxyHost == "" {
		return nil, nil, fmt.Errorf("%w: missing proxy host", ErrInvalidProxyURL)
	}

	var tcpDialer net.Dialer
	control, err := tcpDialer.DialContext(ctx, "tcp", proxyHost)
	if err != nil {
		return nil, nil, fmt.Errorf("socks tcp dial: %w", err)
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = control.SetDeadline(deadline)
	}

	user := strings.TrimSpace(settings.ProxyUser)
	pass := settings.ProxyPassword
	err = socks5Handshake(control, user, pass)
	if err != nil {
		_ = control.Close()

		return nil, nil, err
	}

	relayTCP, err := socks5UDPAssociate(control)
	if err != nil {
		_ = control.Close()

		return nil, nil, err
	}

	var listenCfg net.ListenConfig
	packet, err := listenCfg.ListenPacket(ctx, "udp", "")
	if err != nil {
		_ = control.Close()

		return nil, nil, fmt.Errorf("listen udp for socks: %w", err)
	}

	remoteDNS := strings.EqualFold(parsed.Scheme, schemeSOCKS5H)
	remote, target, err := socksUDPTarget(ctx, hostPort, remoteDNS)
	if err != nil {
		_ = packet.Close()
		_ = control.Close()

		return nil, nil, err
	}

	conn := &socksUDPConn{
		packet:  packet,
		control: control,
		relay:   relayTCP,
		target:  target,
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	return conn, remote, nil
}

func socksUDPTarget(
	ctx context.Context,
	hostPort string,
	remoteDNS bool,
) (net.Addr, socksAddr, error) {
	host, portStr, splitErr := net.SplitHostPort(hostPort)
	if splitErr != nil {
		return nil, socksAddr{}, fmt.Errorf("split hostPort: %w", splitErr)
	}
	port, err := parsePort(portStr)
	if err != nil {
		return nil, socksAddr{}, err
	}

	if remoteDNS {
		return hostPortAddr{hostPort: hostPort}, socksAddr{
			atyp: socksATYPDomain,
			host: host,
			port: port,
		}, nil
	}

	udpAddr, err := resolveUDPAddr(ctx, hostPort)
	if err != nil {
		return nil, socksAddr{}, err
	}
	target, err := socksAddrFromIP(udpAddr.IP, port)
	if err != nil {
		return nil, socksAddr{}, err
	}

	return udpAddr, target, nil
}

func resolveUDPAddr(ctx context.Context, hostPort string) (*net.UDPAddr, error) {
	host, portStr, err := net.SplitHostPort(hostPort)
	if err != nil {
		return nil, fmt.Errorf("split hostPort: %w", err)
	}
	port, err := parsePort(portStr)
	if err != nil {
		return nil, err
	}
	var resolver net.Resolver
	ips, err := resolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("resolve udp %s: %w", hostPort, err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("%w: %s", errNoUDPAddress, hostPort)
	}

	return &net.UDPAddr{IP: preferIPv4(ips), Port: int(port)}, nil
}

func preferIPv4(ips []net.IP) net.IP {
	for _, ip := range ips {
		if v4 := ip.To4(); v4 != nil {
			return v4
		}
	}

	return ips[0]
}

type hostPortAddr struct {
	hostPort string
}

func (a hostPortAddr) Network() string { return "udp" }
func (a hostPortAddr) String() string  { return a.hostPort }
