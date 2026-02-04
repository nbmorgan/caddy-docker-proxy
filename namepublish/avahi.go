package namepublish

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/lucaslorentz/caddy-docker-proxy/v2/config"
	"github.com/mistygrip/go-avahi"
	"go.uber.org/zap"
)

// AvahiPublisher publishes .local names via Avahi.
type AvahiPublisher struct {
	logger          *zap.Logger
	refreshInterval time.Duration
	resolver        *net.Resolver

	mu           sync.Mutex
	desiredNames []string
	desiredHost  string
	lastHash     string
	lastIPs      []string

	conn   *dbus.Conn
	server *avahi.Server
	group  *avahi.EntryGroup

	ticker *time.Ticker
	stopCh chan struct{}
}

// NewAvahiPublisher validates options and constructs a publisher.
func NewAvahiPublisher(opts config.AvahiOptions, logger *zap.Logger) (*AvahiPublisher, error) {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &AvahiPublisher{
		logger:          logger.Named("avahi"),
		refreshInterval: opts.RefreshInterval,
		resolver:        net.DefaultResolver,
	}, nil
}

// Publish publishes .local names via Avahi.
// TODO: Implement Avahi integration.
func (p *AvahiPublisher) Publish(ctx context.Context, names []string, caddyHost string) error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	p.desiredNames = append([]string{}, names...)
	p.desiredHost = caddyHost
	p.startTickerLocked()
	p.mu.Unlock()

	return p.reconcile(ctx)
}

func (p *AvahiPublisher) startTickerLocked() {
	if p.refreshInterval <= 0 || p.ticker != nil {
		return
	}
	p.ticker = time.NewTicker(p.refreshInterval)
	if p.stopCh == nil {
		p.stopCh = make(chan struct{})
	}
	go p.tick()
}

func (p *AvahiPublisher) tick() {
	for {
		select {
		case <-p.ticker.C:
			_ = p.reconcile(context.Background())
		case <-p.stopCh:
			return
		}
	}
}

func (p *AvahiPublisher) reconcile(ctx context.Context) error {
	p.mu.Lock()
	names := append([]string{}, p.desiredNames...)
	caddyHost := p.desiredHost
	p.mu.Unlock()

	localNames := filterLocalNames(names)
	if len(localNames) == 0 {
		return p.resetGroup()
	}

	ips, err := p.resolveIPs(ctx, caddyHost)
	if err != nil {
		return err
	}
	if len(ips) == 0 {
		return nil
	}

	hash := hashAvahiState(localNames, ips)
	needsPublish, err := p.ensureConnection(ctx)
	if err != nil {
		return err
	}

	p.mu.Lock()
	if !needsPublish && hash == p.lastHash && sameStringSlice(ips, p.lastIPs) {
		p.mu.Unlock()
		return nil
	}
	p.lastHash = hash
	p.lastIPs = append([]string{}, ips...)
	p.mu.Unlock()

	if err := p.group.Reset(); err != nil {
		return err
	}
	for _, name := range localNames {
		for _, ip := range ips {
			if err := p.group.AddAddress(avahi.InterfaceUnspec, avahi.ProtoUnspec, 0, name, ip); err != nil {
				return err
			}
		}
	}
	if err := p.group.Commit(); err != nil {
		return err
	}

	p.logger.Info("Avahi publish refreshed", zap.Int("local_names", len(localNames)), zap.Int("ips", len(ips)))
	return nil
}

func (p *AvahiPublisher) resolveIPs(ctx context.Context, host string) ([]string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil, nil
	}
	if ip := net.ParseIP(host); ip != nil {
		return []string{ip.String()}, nil
	}

	resolver := p.resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	addrs, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	var ips []string
	for _, addr := range addrs {
		if addr.IP == nil {
			continue
		}
		ipStr := addr.IP.String()
		if _, ok := seen[ipStr]; ok {
			continue
		}
		seen[ipStr] = struct{}{}
		ips = append(ips, ipStr)
	}
	sort.Strings(ips)
	return ips, nil
}

func (p *AvahiPublisher) ensureConnection(_ context.Context) (bool, error) {
	if p.server != nil {
		if _, err := p.server.GetState(); err == nil {
			if p.group != nil {
				return false, nil
			}
			group, err := p.newEntryGroup()
			if err != nil {
				return true, err
			}
			p.group = group
			return true, nil
		}
	}

	p.closeConnection()

	conn, err := dbus.SystemBus()
	if err != nil {
		return true, err
	}
	server, err := avahi.ServerNew(conn)
	if err != nil {
		conn.Close()
		return true, err
	}
	group, err := newEntryGroup(server, conn)
	if err != nil {
		server.Close()
		conn.Close()
		return true, err
	}
	p.conn = conn
	p.server = server
	p.group = group
	return true, nil
}

func (p *AvahiPublisher) newEntryGroup() (*avahi.EntryGroup, error) {
	if p.server == nil {
		return nil, fmt.Errorf("avahi server not initialized")
	}
	return p.server.EntryGroupNew()
}

func (p *AvahiPublisher) resetGroup() error {
	if p.group == nil {
		return nil
	}
	return p.group.Reset()
}

func (p *AvahiPublisher) closeConnection() {
	p.group = nil
	if p.server != nil {
		p.server.Close()
		p.server = nil
	}
	if p.conn != nil {
		_ = p.conn.Close()
		p.conn = nil
	}
}

func filterLocalNames(names []string) []string {
	var local []string
	for _, name := range names {
		if strings.HasSuffix(strings.ToLower(name), ".local") {
			local = append(local, name)
		}
	}
	return local
}

func hashAvahiState(names, ips []string) string {
	sort.Strings(names)
	sort.Strings(ips)
	return strings.Join(names, ",") + "|" + strings.Join(ips, ",")
}

func sameStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
