package dlna

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sudoStream/internal/access"
	"sudoStream/internal/auth"
	"sudoStream/internal/catalog"
	"sudoStream/internal/mediafs"
	"sudoStream/internal/transcode"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/koron/go-ssdp"
)

// Deps wires domain services into the DLNA listener.
// Listen / SSDP addresses come from process env (composition root), not admin settings.
type Deps struct {
	Settings SettingsStore
	Access   *access.Service
	Media    *mediafs.Service
	Catalog  *catalog.Service
	Auth     *auth.Service
	Probe    func(ctx context.Context, absPath string) (transcode.SourceInfo, error)
	SignKey  []byte
	// Port is SUDOSTREAM_DLNA_HTTP_PORT (default 8200).
	Port int
	// BindHost is SUDOSTREAM_DLNA_HTTP_BIND (empty = all interfaces).
	BindHost string
	// SSDPMulticast is SUDOSTREAM_DLNA_SSDP_MULTICAST (default 239.255.255.250:1900).
	SSDPMulticast string
	// AdvertiseHost is SUDOSTREAM_DLNA_ADVERTISE_HOST (empty = auto-detect LAN IPv4).
	AdvertiseHost string
}

// Controller starts/stops the DLNA HTTP + SSDP stack from settings.
type Controller struct {
	deps Deps

	mu         sync.Mutex
	httpServer *http.Server
	ads        []*ssdp.Advertiser
	aliveStop  chan struct{}
	settings   Settings
	baseURL    string
}

// NewController constructs a DLNA controller (does not listen until Reload).
func NewController(deps Deps) *Controller {
	if deps.Port <= 0 {
		deps.Port = defaultHTTPPort
	}

	return &Controller{deps: deps}
}

// Reload applies settings: stop when disabled, (re)start when enabled.
func (c *Controller) Reload(ctx context.Context) error {
	settings, err := c.deps.Settings.GetSettings(ctx)
	if err != nil {
		return fmt.Errorf("load dlna settings: %w", err)
	}
	if settings.Enabled && settings.UDN == "" {
		settings.UDN = "uuid:" + uuid.NewString()
		err = c.deps.Settings.SaveSettings(ctx, settings)
		if err != nil {
			return fmt.Errorf("persist dlna udn: %w", err)
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.stopLocked(ctx)
	c.settings = settings
	if !settings.Enabled {
		slog.Info("dlna disabled")

		return nil
	}
	if settings.UserID == "" {
		return ErrTVUserRequired
	}

	return c.startLocked(ctx, settings)
}

// Shutdown stops HTTP and SSDP.
func (c *Controller) Shutdown(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopLocked(ctx)

	return nil
}

func (c *Controller) stopLocked(ctx context.Context) {
	if c.aliveStop != nil {
		close(c.aliveStop)
		c.aliveStop = nil
	}
	for _, advertiser := range c.ads {
		_ = advertiser.Bye()
		_ = advertiser.Close()
	}
	c.ads = nil
	if c.httpServer != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, shutdownGrace)
		_ = c.httpServer.Shutdown(shutdownCtx)
		cancel()
		c.httpServer = nil
	}
}

func (c *Controller) startLocked(ctx context.Context, settings Settings) error {
	err := applySSDPMulticast(c.deps.SSDPMulticast)
	if err != nil {
		return err
	}

	lanIP := strings.TrimSpace(c.deps.AdvertiseHost)
	if lanIP == "" {
		detected, err := DetectLANIPv4(ctx)
		if err != nil {
			slog.Warn("dlna LAN IP detect failed; using 127.0.0.1", "err", err)
			lanIP = "127.0.0.1"
		} else {
			lanIP = detected
		}
	}
	c.baseURL = "http://" + net.JoinHostPort(lanIP, strconv.Itoa(c.deps.Port))

	browser := &Browser{
		Access:  c.deps.Access,
		Media:   c.deps.Media,
		Catalog: c.deps.Catalog,
		Probe:   c.deps.Probe,
		BaseURL: c.baseURL,
		SignKey: c.deps.SignKey,
	}
	soap := &SOAPHandler{
		Browser: browser,
		TVUser:  c.tvUser,
	}
	stream := &StreamHandler{
		Access:  c.deps.Access,
		Media:   c.deps.Media,
		Auth:    c.deps.Auth,
		SignKey: c.deps.SignKey,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/dlna/description.xml", func(writer http.ResponseWriter, _ *http.Request) {
		writeDescription(writer, settings.UDN)
	})
	mux.HandleFunc("/dlna/ContentDirectory.xml", func(writer http.ResponseWriter, _ *http.Request) {
		writeContentDirectorySCPD(writer)
	})
	mux.HandleFunc("/dlna/ConnectionManager.xml", func(writer http.ResponseWriter, _ *http.Request) {
		writeConnectionManagerSCPD(writer)
	})
	mux.HandleFunc("/dlna/ContentDirectory/control", soap.ServeContentDirectory)
	mux.HandleFunc("/dlna/ConnectionManager/control", soap.ServeConnectionManager)
	mux.HandleFunc("/dlna/ContentDirectory/event", func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/dlna/ConnectionManager/event", func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
	})
	mux.Handle("/dlna/stream/", stream)

	c.httpServer = &http.Server{
		Addr:              listenAddr(c.deps.BindHost, c.deps.Port),
		Handler:           mux,
		ReadHeaderTimeout: httpHeaderTimeout,
	}

	go func() {
		slog.Info("dlna listening", "addr", c.httpServer.Addr, "location", c.baseURL+"/dlna/description.xml")
		listenErr := c.httpServer.ListenAndServe()
		if listenErr != nil && !errors.Is(listenErr, http.ErrServerClosed) {
			slog.Error("dlna http failed", "err", listenErr)
		}
	}()

	location := c.baseURL + "/dlna/description.xml"
	serverHeader := "sudoStream/1 UPnP/1.0"
	ads, err := advertiseSSDP(settings.UDN, location, serverHeader)
	if err != nil {
		c.stopLocked(ctx)

		return fmt.Errorf("ssdp advertise: %w", err)
	}
	c.ads = ads
	c.aliveStop = make(chan struct{})
	go c.aliveLoop(c.aliveStop)

	return nil
}

func listenAddr(bindHost string, port int) string {
	host := strings.TrimSpace(bindHost)
	if host == "" {
		return fmt.Sprintf(":%d", port)
	}

	return net.JoinHostPort(host, strconv.Itoa(port))
}

func applySSDPMulticast(raw string) error {
	addr := strings.TrimSpace(raw)
	if addr == "" {
		addr = defaultSSDPMulticast
	}
	err := ssdp.SetMulticastRecvAddrIPv4(addr)
	if err != nil {
		return fmt.Errorf("ssdp recv addr: %w", err)
	}
	err = ssdp.SetMulticastSendAddrIPv4(addr)
	if err != nil {
		return fmt.Errorf("ssdp send addr: %w", err)
	}

	return nil
}

func (c *Controller) tvUser() (auth.PublicUser, error) {
	c.mu.Lock()
	settings := c.settings
	c.mu.Unlock()
	if !settings.Enabled || settings.UserID == "" {
		return auth.PublicUser{}, ErrNotEnabled
	}
	user, err := c.deps.Auth.GetUserByID(context.Background(), settings.UserID)
	if err != nil {
		return auth.PublicUser{}, fmt.Errorf("load tv user: %w", err)
	}
	if user.Role != auth.RoleTV || !user.Enabled {
		return auth.PublicUser{}, ErrInvalidTVUser
	}

	return auth.PublicUser{ID: user.ID, Email: user.Email, Role: user.Role}, nil
}

func (c *Controller) aliveLoop(stop <-chan struct{}) {
	ticker := time.NewTicker(time.Duration(ssdpMaxAgeSec/ssdpAliveDivisor) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			c.mu.Lock()
			for _, advertiser := range c.ads {
				_ = advertiser.Alive()
			}
			c.mu.Unlock()
		}
	}
}

func advertiseSSDP(udn, location, server string) ([]*ssdp.Advertiser, error) {
	type entry struct {
		st  string
		usn string
	}
	entries := []entry{
		{st: "upnp:rootdevice", usn: udn + "::upnp:rootdevice"},
		{st: "urn:schemas-upnp-org:device:MediaServer:1", usn: udn + "::urn:schemas-upnp-org:device:MediaServer:1"},
		{st: cdServiceType, usn: udn + "::" + cdServiceType},
		{st: cmServiceType, usn: udn + "::" + cmServiceType},
	}
	ads := make([]*ssdp.Advertiser, 0, len(entries))
	for _, item := range entries {
		advertiser, err := ssdp.Advertise(item.st, item.usn, location, server, ssdpMaxAgeSec)
		if err != nil {
			for _, existing := range ads {
				_ = existing.Bye()
				_ = existing.Close()
			}

			return nil, fmt.Errorf("advertise %s: %w", item.st, err)
		}
		_ = advertiser.Alive()
		ads = append(ads, advertiser)
	}

	return ads, nil
}
