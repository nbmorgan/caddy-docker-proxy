package namepublish

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/lucaslorentz/caddy-docker-proxy/v2/config"
	"go.uber.org/zap"
)

// TechnitiumPublisher publishes names to Technitium DNS.
type TechnitiumPublisher struct {
	baseURL string
	token   string
	zone    string
	ttl     int
	client  *http.Client
	logger  *zap.Logger
}

// NewTechnitiumPublisher validates options and constructs a publisher.
func NewTechnitiumPublisher(opts config.TechnitiumOptions, logger *zap.Logger) (*TechnitiumPublisher, error) {
	if strings.TrimSpace(opts.BaseURL) == "" {
		return nil, fmt.Errorf("technitium base URL is required")
	}
	if strings.TrimSpace(opts.Token) == "" {
		return nil, fmt.Errorf("technitium token is required")
	}
	if strings.TrimSpace(opts.Zone) == "" {
		return nil, fmt.Errorf("technitium zone is required")
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	publisher := &TechnitiumPublisher{
		baseURL: opts.BaseURL,
		token:   opts.Token,
		zone:    opts.Zone,
		ttl:     opts.TTL,
		client:  &http.Client{Timeout: 10 * time.Second},
		logger:  logger.Named("technitium"),
	}
	return publisher, nil
}

// Publish publishes desired names to Technitium DNS.
func (p *TechnitiumPublisher) Publish(ctx context.Context, names []string, caddyHost string) error {
	if p == nil {
		return nil
	}

	recordType, rdataKey, rdataValue, err := p.recordParams(caddyHost)
	if err != nil {
		return err
	}

	for _, name := range names {
		if err := p.addRecord(ctx, name, recordType, rdataKey, rdataValue); err != nil {
			return err
		}
	}
	return nil
}

func (p *TechnitiumPublisher) recordParams(caddyHost string) (string, string, string, error) {
	if ip := net.ParseIP(caddyHost); ip != nil {
		if ip.To4() != nil {
			return "A", "ipAddress", ip.String(), nil
		}
		return "AAAA", "ipAddress", ip.String(), nil
	}
	host := strings.TrimSpace(caddyHost)
	if host == "" {
		return "", "", "", fmt.Errorf("technitium caddy host is empty")
	}
	return "CNAME", "cname", host, nil
}

func (p *TechnitiumPublisher) addRecord(ctx context.Context, domain, recordType, rdataKey, rdataValue string) error {
	if strings.TrimSpace(domain) == "" {
		return nil
	}

	form := url.Values{}
	form.Set("token", p.token)
	form.Set("domain", domain)
	form.Set("zone", p.zone)
	form.Set("type", recordType)
	form.Set("overwrite", "true")
	form.Set(rdataKey, rdataValue)
	if p.ttl > 0 {
		form.Set("ttl", fmt.Sprintf("%d", p.ttl))
	}

	endpoint := strings.TrimRight(p.baseURL, "/") + "/api/zones/records/add"
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var payload struct {
		Status       string `json:"status"`
		ErrorMessage string `json:"errorMessage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}
	if payload.Status != "ok" {
		if payload.ErrorMessage == "" {
			payload.ErrorMessage = "unknown error"
		}
		return fmt.Errorf("technitium add record failed: %s", payload.ErrorMessage)
	}

	p.logger.Debug("Record published", zap.String("domain", domain), zap.String("type", recordType))
	return nil
}
