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

const technitiumManagedComment = "managed-by:caddy-docker-proxy"

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
	if len(names) == 0 {
		return nil
	}

	records, err := p.getZoneRecords(ctx)
	if err != nil {
		return err
	}

	desired := toNameSet(names)
	managedByName := map[string][]technitiumRecord{}
	existingByNameType := map[string][]technitiumRecord{}

	for _, record := range records {
		if !isSupportedType(record.Type) {
			continue
		}
		key := record.Name + "|" + record.Type
		existingByNameType[key] = append(existingByNameType[key], record)
		if record.Comments == technitiumManagedComment {
			managedByName[record.Name] = append(managedByName[record.Name], record)
		}
	}

	for name, managedRecords := range managedByName {
		if desired[name] {
			continue
		}
		for _, record := range managedRecords {
			if err := p.deleteRecord(ctx, record); err != nil {
				return err
			}
		}
	}

	for _, name := range names {
		if strings.TrimSpace(name) == "" {
			continue
		}
		key := name + "|" + recordType
		if hasUnmanaged(existingByNameType[key]) {
			p.logger.Warn("Skipping name; unmanaged record exists", zap.String("name", name), zap.String("type", recordType))
			continue
		}

		managedRecords := managedByName[name]
		var managedSameType []technitiumRecord
		var managedOtherTypes []technitiumRecord
		for _, record := range managedRecords {
			if record.Type == recordType {
				managedSameType = append(managedSameType, record)
			} else {
				managedOtherTypes = append(managedOtherTypes, record)
			}
		}

		for _, record := range managedOtherTypes {
			if err := p.deleteRecord(ctx, record); err != nil {
				return err
			}
		}

		if len(managedSameType) > 0 {
			if managedSameType[0].value() == rdataValue {
				continue
			}
			for _, record := range managedSameType {
				if err := p.deleteRecord(ctx, record); err != nil {
					return err
				}
			}
		}

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
	form.Set("comments", technitiumManagedComment)
	if p.ttl > 0 {
		form.Set("ttl", fmt.Sprintf("%d", p.ttl))
	}

	endpoint := strings.TrimRight(p.baseURL, "/") + "/api/zones/records/add"
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	payload, err := p.doJSON(req)
	if err != nil {
		return err
	}
	if payload.Status != "ok" {
		return fmt.Errorf("technitium add record failed: %s", payload.errorMessage())
	}

	p.logger.Debug("Record published", zap.String("domain", domain), zap.String("type", recordType))
	return nil
}

func (p *TechnitiumPublisher) deleteRecord(ctx context.Context, record technitiumRecord) error {
	value := record.value()
	if value == "" {
		return nil
	}

	form := url.Values{}
	form.Set("token", p.token)
	form.Set("domain", record.Name)
	form.Set("zone", p.zone)
	form.Set("type", record.Type)
	form.Set("value", value)

	endpoint := strings.TrimRight(p.baseURL, "/") + "/api/zones/records/delete"
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	payload, err := p.doJSON(req)
	if err != nil {
		return err
	}
	if payload.Status != "ok" {
		return fmt.Errorf("technitium delete record failed: %s", payload.errorMessage())
	}

	p.logger.Debug("Record deleted", zap.String("domain", record.Name), zap.String("type", record.Type))
	return nil
}

func (p *TechnitiumPublisher) getZoneRecords(ctx context.Context) ([]technitiumRecord, error) {
	endpoint := strings.TrimRight(p.baseURL, "/") + "/api/zones/records/get"
	query := url.Values{}
	query.Set("token", p.token)
	query.Set("domain", p.zone)
	query.Set("zone", p.zone)
	query.Set("listZone", "true")

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint+"?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var payload struct {
		Status       string `json:"status"`
		ErrorMessage string `json:"errorMessage"`
		Response     struct {
			Records []technitiumRecord `json:"records"`
		} `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if payload.Status != "ok" {
		if payload.ErrorMessage == "" {
			payload.ErrorMessage = "unknown error"
		}
		return nil, fmt.Errorf("technitium get records failed: %s", payload.ErrorMessage)
	}
	return payload.Response.Records, nil
}

type technitiumRecord struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Comments string `json:"comments"`
	RData    struct {
		CName     string `json:"cname"`
		IPAddress string `json:"ipAddress"`
	} `json:"rData"`
}

func (r technitiumRecord) value() string {
	switch strings.ToUpper(r.Type) {
	case "CNAME":
		return r.RData.CName
	case "A", "AAAA":
		return r.RData.IPAddress
	default:
		return ""
	}
}

type technitiumStatus struct {
	Status       string `json:"status"`
	ErrorMessage string `json:"errorMessage"`
}

func (s technitiumStatus) errorMessage() string {
	if s.ErrorMessage == "" {
		return "unknown error"
	}
	return s.ErrorMessage
}

func (p *TechnitiumPublisher) doJSON(req *http.Request) (technitiumStatus, error) {
	resp, err := p.client.Do(req)
	if err != nil {
		return technitiumStatus{}, err
	}
	defer resp.Body.Close()

	var payload technitiumStatus
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return technitiumStatus{}, err
	}
	return payload, nil
}

func toNameSet(names []string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, name := range names {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		set[trimmed] = true
	}
	return set
}

func hasUnmanaged(records []technitiumRecord) bool {
	for _, record := range records {
		if record.Comments != technitiumManagedComment {
			return true
		}
	}
	return false
}

func isSupportedType(recordType string) bool {
	switch strings.ToUpper(recordType) {
	case "A", "AAAA", "CNAME":
		return true
	default:
		return false
	}
}
