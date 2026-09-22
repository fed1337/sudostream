package network

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"
)

const (
	socksVersion5     = 0x05
	socksAuthVersion  = 0x01
	socksMethodNone   = 0x00
	socksMethodUser   = 0x02
	socksMethodNoAcc  = 0xff
	socksCmdUDPAssoc  = 0x03
	socksRepSuccess   = 0x00
	socksATYPIPv4     = 0x01
	socksATYPDomain   = 0x03
	socksATYPIPv6     = 0x04
	socksUDPHeaderRSV = 2
	socksMaxDomainLen = 255
	socksAuthMaxLen   = 255
	socksIPv4Size     = 4
	socksIPv6Size     = 16
	socksPortSize     = 2
	socksReplyHdrLen  = 4
	socksUDPReadSlack = 512
	socksFragNone     = 0x00
)

var (
	errSocksAuthRejected    = errors.New("socks5 authentication rejected")
	errSocksUDPAssociate    = errors.New("socks5 udp associate failed")
	errSocksUDPHeader       = errors.New("socks5 udp header invalid")
	errSocksUnsupportedATYP = errors.New("socks5 unsupported address type")
	errNoUDPAddress         = errors.New("no udp addresses")
)

// socksAddr is a SOCKS5 address used in UDP headers.
type socksAddr struct {
	atyp byte
	ip   net.IP
	host string
	port uint16
}

func socksAddrFromIP(ip net.IP, port uint16) (socksAddr, error) {
	if v4 := ip.To4(); v4 != nil {
		return socksAddr{atyp: socksATYPIPv4, ip: v4, port: port}, nil
	}
	if v6 := ip.To16(); v6 != nil && ip.To4() == nil {
		return socksAddr{atyp: socksATYPIPv6, ip: v6, port: port}, nil
	}

	return socksAddr{}, fmt.Errorf("%w: empty ip", errSocksUnsupportedATYP)
}

func parsePort(raw string) (uint16, error) {
	port64, err := strconv.ParseUint(raw, 10, 16)
	if err != nil {
		return 0, fmt.Errorf("parse port: %w", err)
	}

	return uint16(port64), nil
}

func socks5Handshake(conn net.Conn, user, pass string) error {
	methods := []byte{socksMethodNone}
	if user != "" {
		methods = []byte{socksMethodUser, socksMethodNone}
	}
	greeting := make([]byte, 0, socksUDPHeaderRSV+len(methods))
	greeting = append(greeting, socksVersion5, byte(len(methods))) //nolint:gosec // G115: method count ≤2
	greeting = append(greeting, methods...)
	_, err := conn.Write(greeting)
	if err != nil {
		return fmt.Errorf("socks greeting: %w", err)
	}

	var reply [2]byte
	_, err = io.ReadFull(conn, reply[:])
	if err != nil {
		return fmt.Errorf("socks greeting reply: %w", err)
	}
	if reply[0] != socksVersion5 {
		return fmt.Errorf("%w: version %d", errSocksUDPAssociate, reply[0])
	}
	switch reply[1] {
	case socksMethodNone:
		return nil
	case socksMethodUser:
		return socks5UserPassAuth(conn, user, pass)
	case socksMethodNoAcc:
		return fmt.Errorf("%w: no acceptable method", errSocksAuthRejected)
	default:
		return fmt.Errorf("%w: method %#x", errSocksAuthRejected, reply[1])
	}
}

func socks5UserPassAuth(conn net.Conn, user, pass string) error {
	if len(user) > socksAuthMaxLen || len(pass) > socksAuthMaxLen {
		return fmt.Errorf("%w: credentials too long", errSocksAuthRejected)
	}
	req := make([]byte, 0, 3+len(user)+len(pass))
	req = append(req, socksAuthVersion, byte(len(user))) //nolint:gosec // G115: bounded above
	req = append(req, user...)
	req = append(req, byte(len(pass))) //nolint:gosec // G115: bounded above
	req = append(req, pass...)
	_, err := conn.Write(req)
	if err != nil {
		return fmt.Errorf("socks auth: %w", err)
	}
	var reply [2]byte
	_, err = io.ReadFull(conn, reply[:])
	if err != nil {
		return fmt.Errorf("socks auth reply: %w", err)
	}
	if reply[0] != socksAuthVersion || reply[1] != 0x00 {
		return fmt.Errorf("%w: status %#x", errSocksAuthRejected, reply[1])
	}

	return nil
}

// socks5UDPAssociate issues CMD UDP ASSOCIATE with dst 0.0.0.0:0 and returns the relay UDP addr.
func socks5UDPAssociate(conn net.Conn) (*net.UDPAddr, error) {
	req := []byte{
		socksVersion5,
		socksCmdUDPAssoc,
		0x00,
		socksATYPIPv4,
		0, 0, 0, 0,
		0, 0,
	}
	_, err := conn.Write(req)
	if err != nil {
		return nil, fmt.Errorf("socks udp associate request: %w", err)
	}

	header := make([]byte, socksReplyHdrLen)
	_, err = io.ReadFull(conn, header)
	if err != nil {
		return nil, fmt.Errorf("socks udp associate reply: %w", err)
	}
	if header[0] != socksVersion5 {
		return nil, fmt.Errorf("%w: version %d", errSocksUDPAssociate, header[0])
	}
	if header[1] != socksRepSuccess {
		return nil, fmt.Errorf("%w: rep %#x", errSocksUDPAssociate, header[1])
	}
	addr, _, err := readSocksAddr(conn, header[3])
	if err != nil {
		return nil, fmt.Errorf("socks udp associate bind addr: %w", err)
	}
	bindIP := addr.ip
	if bindIP == nil {
		return nil, fmt.Errorf("%w: domain bind addr unsupported", errSocksUDPAssociate)
	}
	if bindIP.IsUnspecified() {
		host, _, splitErr := net.SplitHostPort(conn.RemoteAddr().String())
		if splitErr != nil {
			return nil, fmt.Errorf("%w: zero bind addr: %w", errSocksUDPAssociate, splitErr)
		}
		bindIP = net.ParseIP(host)
		if bindIP == nil {
			return nil, fmt.Errorf("%w: cannot resolve bind host %q", errSocksUDPAssociate, host)
		}
	}

	return &net.UDPAddr{IP: bindIP, Port: int(addr.port)}, nil
}

func readSocksAddr(reader io.Reader, atyp byte) (socksAddr, int, error) {
	switch atyp {
	case socksATYPIPv4:
		buf := make([]byte, socksIPv4Size+socksPortSize)
		_, err := io.ReadFull(reader, buf)
		if err != nil {
			return socksAddr{}, 0, fmt.Errorf("read socks ipv4: %w", err)
		}

		return socksAddr{
			atyp: atyp,
			ip:   net.IP(buf[:socksIPv4Size]),
			port: binary.BigEndian.Uint16(buf[socksIPv4Size:]),
		}, len(buf), nil
	case socksATYPIPv6:
		buf := make([]byte, socksIPv6Size+socksPortSize)
		_, err := io.ReadFull(reader, buf)
		if err != nil {
			return socksAddr{}, 0, fmt.Errorf("read socks ipv6: %w", err)
		}

		return socksAddr{
			atyp: atyp,
			ip:   net.IP(buf[:socksIPv6Size]),
			port: binary.BigEndian.Uint16(buf[socksIPv6Size:]),
		}, len(buf), nil
	case socksATYPDomain:
		var length [1]byte
		_, err := io.ReadFull(reader, length[:])
		if err != nil {
			return socksAddr{}, 0, fmt.Errorf("read socks domain len: %w", err)
		}
		buf := make([]byte, int(length[0])+socksPortSize)
		_, err = io.ReadFull(reader, buf)
		if err != nil {
			return socksAddr{}, 0, fmt.Errorf("read socks domain: %w", err)
		}

		return socksAddr{
			atyp: atyp,
			host: string(buf[:length[0]]),
			port: binary.BigEndian.Uint16(buf[length[0]:]),
		}, 1 + len(buf), nil
	default:
		return socksAddr{}, 0, fmt.Errorf("%w: %#x", errSocksUnsupportedATYP, atyp)
	}
}

// encodeSocksUDPHeader prepends RSV RSV FRAG + address + port for a SOCKS5 UDP request.
func encodeSocksUDPHeader(dst socksAddr, payload []byte) ([]byte, error) {
	addrBytes, err := encodeSocksAddr(dst)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, socksUDPHeaderRSV+1+len(addrBytes)+len(payload))
	out = append(out, socksFragNone, socksFragNone) // RSV
	out = append(out, socksFragNone)
	out = append(out, addrBytes...)
	out = append(out, payload...)

	return out, nil
}

func encodeSocksAddr(addr socksAddr) ([]byte, error) {
	switch addr.atyp {
	case socksATYPIPv4:
		ip := addr.ip.To4()
		if ip == nil {
			return nil, fmt.Errorf("%w: ipv4", errSocksUnsupportedATYP)
		}
		out := make([]byte, 1+socksIPv4Size+socksPortSize)
		out[0] = socksATYPIPv4
		copy(out[1:], ip)
		binary.BigEndian.PutUint16(out[1+socksIPv4Size:], addr.port)

		return out, nil
	case socksATYPIPv6:
		ip := addr.ip.To16()
		if ip == nil || addr.ip.To4() != nil {
			return nil, fmt.Errorf("%w: ipv6", errSocksUnsupportedATYP)
		}
		out := make([]byte, 1+socksIPv6Size+socksPortSize)
		out[0] = socksATYPIPv6
		copy(out[1:], ip)
		binary.BigEndian.PutUint16(out[1+socksIPv6Size:], addr.port)

		return out, nil
	case socksATYPDomain:
		if addr.host == "" || len(addr.host) > socksMaxDomainLen {
			return nil, fmt.Errorf("%w: domain length", errSocksUDPHeader)
		}
		out := make([]byte, 0, 1+1+len(addr.host)+socksPortSize)
		out = append(out, socksATYPDomain, byte(len(addr.host))) //nolint:gosec // G115: host ≤255
		out = append(out, addr.host...)
		var port [socksPortSize]byte
		binary.BigEndian.PutUint16(port[:], addr.port)
		out = append(out, port[:]...)

		return out, nil
	default:
		return nil, fmt.Errorf("%w: %#x", errSocksUnsupportedATYP, addr.atyp)
	}
}

// decodeSocksUDPHeader strips the SOCKS5 UDP request header and returns payload + source addr.
func decodeSocksUDPHeader(packet []byte) ([]byte, net.Addr, error) {
	minLen := socksUDPHeaderRSV + 1 + 1 + socksPortSize
	if len(packet) < minLen {
		return nil, nil, fmt.Errorf("%w: short packet", errSocksUDPHeader)
	}
	if packet[2] != socksFragNone {
		return nil, nil, fmt.Errorf("%w: fragmented", errSocksUDPHeader)
	}
	atyp := packet[3]
	rest := packet[4:]
	addr, consumed, err := readSocksAddrFromBytes(rest, atyp)
	if err != nil {
		return nil, nil, err
	}
	payload := rest[consumed:]
	src, err := addr.toNetAddr()
	if err != nil {
		return nil, nil, err
	}

	return payload, src, nil
}

func readSocksAddrFromBytes(buf []byte, atyp byte) (socksAddr, int, error) {
	switch atyp {
	case socksATYPIPv4:
		need := socksIPv4Size + socksPortSize
		if len(buf) < need {
			return socksAddr{}, 0, fmt.Errorf("%w: short ipv4", errSocksUDPHeader)
		}

		return socksAddr{
			atyp: atyp,
			ip:   append(net.IP(nil), buf[:socksIPv4Size]...),
			port: binary.BigEndian.Uint16(buf[socksIPv4Size:need]),
		}, need, nil
	case socksATYPIPv6:
		need := socksIPv6Size + socksPortSize
		if len(buf) < need {
			return socksAddr{}, 0, fmt.Errorf("%w: short ipv6", errSocksUDPHeader)
		}

		return socksAddr{
			atyp: atyp,
			ip:   append(net.IP(nil), buf[:socksIPv6Size]...),
			port: binary.BigEndian.Uint16(buf[socksIPv6Size:need]),
		}, need, nil
	case socksATYPDomain:
		if len(buf) < 1 {
			return socksAddr{}, 0, fmt.Errorf("%w: short domain len", errSocksUDPHeader)
		}
		hostLen := int(buf[0])
		need := 1 + hostLen + socksPortSize
		if len(buf) < need {
			return socksAddr{}, 0, fmt.Errorf("%w: short domain", errSocksUDPHeader)
		}

		return socksAddr{
			atyp: atyp,
			host: string(buf[1 : 1+hostLen]),
			port: binary.BigEndian.Uint16(buf[1+hostLen : need]),
		}, need, nil
	default:
		return socksAddr{}, 0, fmt.Errorf("%w: %#x", errSocksUnsupportedATYP, atyp)
	}
}

func (a socksAddr) toNetAddr() (net.Addr, error) {
	switch a.atyp {
	case socksATYPIPv4, socksATYPIPv6:
		return &net.UDPAddr{IP: a.ip, Port: int(a.port)}, nil
	case socksATYPDomain:
		return hostPortAddr{hostPort: net.JoinHostPort(a.host, strconv.Itoa(int(a.port)))}, nil
	default:
		return nil, fmt.Errorf("%w: %#x", errSocksUnsupportedATYP, a.atyp)
	}
}

// socksUDPConn wraps a local UDP socket with SOCKS5 UDP headers and keeps the TCP control conn open.
type socksUDPConn struct {
	packet  net.PacketConn
	control net.Conn
	relay   *net.UDPAddr
	target  socksAddr
}

func (c *socksUDPConn) ReadFrom(buf []byte) (int, net.Addr, error) {
	scratch := make([]byte, len(buf)+socksUDPReadSlack)
	readN, _, err := c.packet.ReadFrom(scratch)
	if err != nil {
		return 0, nil, fmt.Errorf("socks udp read: %w", err)
	}
	payload, src, err := decodeSocksUDPHeader(scratch[:readN])
	if err != nil {
		return 0, nil, err
	}
	copied := copy(buf, payload)
	if copied < len(payload) {
		return copied, src, fmt.Errorf("socks udp read: %w", io.ErrShortBuffer)
	}

	return copied, src, nil
}

func (c *socksUDPConn) WriteTo(payload []byte, _ net.Addr) (int, error) {
	framed, err := encodeSocksUDPHeader(c.target, payload)
	if err != nil {
		return 0, err
	}
	_, err = c.packet.WriteTo(framed, c.relay)
	if err != nil {
		return 0, fmt.Errorf("socks udp write: %w", err)
	}

	return len(payload), nil
}

func (c *socksUDPConn) Close() error {
	var first error
	if c.packet != nil {
		err := c.packet.Close()
		if err != nil && first == nil {
			first = err
		}
	}
	if c.control != nil {
		err := c.control.Close()
		if err != nil && first == nil {
			first = err
		}
	}

	return first
}

func (c *socksUDPConn) LocalAddr() net.Addr {
	if c.packet == nil {
		return nil
	}

	return c.packet.LocalAddr()
}

func (c *socksUDPConn) SetDeadline(t time.Time) error {
	if c.packet == nil {
		return nil
	}
	err := c.packet.SetDeadline(t)
	if err != nil {
		return fmt.Errorf("socks udp deadline: %w", err)
	}

	return nil
}

func (c *socksUDPConn) SetReadDeadline(t time.Time) error {
	if c.packet == nil {
		return nil
	}
	err := c.packet.SetReadDeadline(t)
	if err != nil {
		return fmt.Errorf("socks udp read deadline: %w", err)
	}

	return nil
}

func (c *socksUDPConn) SetWriteDeadline(t time.Time) error {
	if c.packet == nil {
		return nil
	}
	err := c.packet.SetWriteDeadline(t)
	if err != nil {
		return fmt.Errorf("socks udp write deadline: %w", err)
	}

	return nil
}
