package dlna

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

const (
	lanProbeTimeout   = 2 * time.Second
	httpHeaderTimeout = 10 * time.Second
	shutdownGrace     = 5 * time.Second
	ssdpAliveDivisor  = 3
)

func writeDescription(writer http.ResponseWriter, udn string) {
	payload := fmt.Sprintf(`<?xml version="1.0"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
  <specVersion><major>1</major><minor>0</minor></specVersion>
  <device>
    <deviceType>urn:schemas-upnp-org:device:MediaServer:1</deviceType>
    <friendlyName>sudoStream</friendlyName>
    <manufacturer>sudoStream</manufacturer>
    <manufacturerURL>https://github.com/fed1337/sudostream</manufacturerURL>
    <modelDescription>sudoStream DLNA Media Server</modelDescription>
    <modelName>sudoStream</modelName>
    <modelNumber>1</modelNumber>
    <UDN>%s</UDN>
    <serviceList>
      <service>
        <serviceType>%s</serviceType>
        <serviceId>urn:upnp-org:serviceId:ContentDirectory</serviceId>
        <SCPDURL>/dlna/ContentDirectory.xml</SCPDURL>
        <controlURL>/dlna/ContentDirectory/control</controlURL>
        <eventSubURL>/dlna/ContentDirectory/event</eventSubURL>
      </service>
      <service>
        <serviceType>%s</serviceType>
        <serviceId>urn:upnp-org:serviceId:ConnectionManager</serviceId>
        <SCPDURL>/dlna/ConnectionManager.xml</SCPDURL>
        <controlURL>/dlna/ConnectionManager/control</controlURL>
        <eventSubURL>/dlna/ConnectionManager/event</eventSubURL>
      </service>
    </serviceList>
  </device>
</root>`, udn, cdServiceType, cmServiceType)
	writer.Header().Set("Content-Type", `text/xml; charset="utf-8"`)
	_, _ = writer.Write([]byte(payload))
}

func writeContentDirectorySCPD(writer http.ResponseWriter) {
	writer.Header().Set("Content-Type", `text/xml; charset="utf-8"`)
	_, _ = writer.Write([]byte(`<?xml version="1.0"?>
<scpd xmlns="urn:schemas-upnp-org:service-1-0">
  <specVersion><major>1</major><minor>0</minor></specVersion>
  <actionList>
    <action><name>Browse</name></action>
    <action><name>GetSearchCapabilities</name></action>
    <action><name>GetSortCapabilities</name></action>
    <action><name>GetSystemUpdateID</name></action>
  </actionList>
  <serviceStateTable></serviceStateTable>
</scpd>`))
}

func writeConnectionManagerSCPD(writer http.ResponseWriter) {
	writer.Header().Set("Content-Type", `text/xml; charset="utf-8"`)
	_, _ = writer.Write([]byte(`<?xml version="1.0"?>
<scpd xmlns="urn:schemas-upnp-org:service-1-0">
  <specVersion><major>1</major><minor>0</minor></specVersion>
  <actionList>
    <action><name>GetProtocolInfo</name></action>
    <action><name>GetCurrentConnectionIDs</name></action>
    <action><name>GetCurrentConnectionInfo</name></action>
  </actionList>
  <serviceStateTable></serviceStateTable>
</scpd>`))
}

// DetectLANIPv4 returns a non-loopback IPv4 suitable for LOCATION URLs.
func DetectLANIPv4(ctx context.Context) (string, error) {
	dialer := net.Dialer{Timeout: lanProbeTimeout} //nolint:exhaustruct // dial probe only
	conn, err := dialer.DialContext(ctx, "udp", "8.8.8.8:80")
	if err != nil {
		return fallbackInterfaceIPv4()
	}
	defer func() { _ = conn.Close() }()

	host, _, err := net.SplitHostPort(conn.LocalAddr().String())
	if err != nil {
		return "", fmt.Errorf("split local addr: %w", err)
	}
	if ipAddr := net.ParseIP(host); ipAddr != nil && ipAddr.To4() != nil && !ipAddr.IsLoopback() {
		return host, nil
	}

	return fallbackInterfaceIPv4()
}

func fallbackInterfaceIPv4() (string, error) { //nolint:cyclop // iface walk
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", fmt.Errorf("list interfaces: %w", err)
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, addrErr := iface.Addrs()
		if addrErr != nil {
			continue
		}
		for _, addr := range addrs {
			var ipAddr net.IP
			switch typed := addr.(type) {
			case *net.IPNet:
				ipAddr = typed.IP
			case *net.IPAddr:
				ipAddr = typed.IP
			}
			if ipAddr == nil || ipAddr.IsLoopback() || ipAddr.To4() == nil {
				continue
			}

			return ipAddr.String(), nil
		}
	}

	return "", ErrNoLANIPv4
}
