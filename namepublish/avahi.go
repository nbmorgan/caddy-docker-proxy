package namepublish

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/mdns"
	"github.com/lucaslorentz/caddy-docker-proxy/v2/config"
	"github.com/miekg/dns"
	"go.uber.org/zap"
)

// AvahiPublisher publishes .local names via mDNS.
type AvahiPublisher struct {
	logger          *zap.Logger
	refreshInterval time.Duration
	resolver        *net.Resolver

	mu           sync.RWMutex
	desiredNames []string
	desiredHost  string
	lastHash     string
	lastIPs      []string

	server *mdns.Server
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

// Publish publishes .local names via mDNS.
func (p *AvahiPublisher) Publish(ctx context.Context, names []string, caddyHost string) error {
	if p == nil {
		return nil
	}

	p.mu.Lock()
	p.desiredNames = append([]string{}, names...)
	p.desiredHost = caddyHost
	p.startTickerLocked()
	p.mu.Unlock()

	if err := p.ensureServer(); err != nil {
		return err
	}

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
	p.mu.RLock()
	names := append([]string{}, p.desiredNames...)
	caddyHost := p.desiredHost
	p.mu.RUnlock()

	localNames := filterLocalNames(names)
	if len(localNames) == 0 {
		p.updateState(nil, nil)
		return nil
	}

	ips, err := p.resolveIPs(ctx, caddyHost)
	if err != nil {
		return err
	}
	if len(ips) == 0 {
		return nil
	}

	hash := hashAvahiState(localNames, ips)
	p.mu.Lock()
	if hash == p.lastHash && sameStringSlice(ips, p.lastIPs) {
		p.mu.Unlock()
		return nil
	}
	p.lastHash = hash
	p.lastIPs = append([]string{}, ips...)
	p.mu.Unlock()

	p.updateState(localNames, ips)
	p.logger.Info("mDNS publish refreshed", zap.Int("local_names", len(localNames)), zap.Int("ips", len(ips)))
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

func (p *AvahiPublisher) ensureServer() error {
	if p.server != nil {
		return nil
	}
	server, err := mdns.NewServer(&mdns.Config{Zone: p})
	if err != nil {
		return fmt.Errorf("failed to start mdns server: %w", err)
	}
	p.server = server
	return nil
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

func (p *AvahiPublisher) updateState(names []string, ips []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.desiredNames = append([]string{}, names...)
	p.lastIPs = append([]string{}, ips...)
}

// Records implements mdns.Zone.
func (p *AvahiPublisher) Records(q dns.Question) []dns.RR {
	if q.Qtype != dns.TypeA && q.Qtype != dns.TypeAAAA {
		return nil
	}

	name := strings.TrimSuffix(strings.ToLower(q.Name), ".")
	if !nameHasLocalSuffix(name) {
		return nil
	}

	p.mu.RLock()
	names := append([]string{}, p.desiredNames...)
	ips := append([]string{}, p.lastIPs...)
	p.mu.RUnlock()

	if !containsName(names, name) {
		return nil
	}

	var records []dns.RR
	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}
		switch q.Qtype {
		case dns.TypeA:
			if ip.To4() == nil {
				continue
			}
			records = append(records, &dns.A{
				Hdr: dns.RR_Header{Name: dns.Fqdn(name), Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 120},
				A:   ip.To4(),
			})
		case dns.TypeAAAA:
			if ip.To4() != nil {
				continue
			}
			records = append(records, &dns.AAAA{
				Hdr:  dns.RR_Header{Name: dns.Fqdn(name), Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: 120},
				AAAA: ip,
			})
		}
	}
	return records
}

// NSEC implements mdns.Zone.
func (p *AvahiPublisher) NSEC(_ dns.Question) []dns.RR {
	return nil
}

func containsName(names []string, candidate string) bool {
	for _, name := range names {
		if strings.EqualFold(name, candidate) {
			return true
		}
	}
	return false
}

func nameHasLocalSuffix(name string) bool {
	return strings.HasSuffix(strings.ToLower(name), ".local")
}
