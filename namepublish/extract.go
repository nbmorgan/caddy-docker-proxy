package namepublish

import (
	"net"
	"net/url"
	"strings"

	"github.com/lucaslorentz/caddy-docker-proxy/v2/caddyfile"
)

// ExtractDesiredNames parses a Caddyfile and returns site names to publish.
func ExtractDesiredNames(caddyfileBytes []byte) ([]string, error) {
	container, err := caddyfile.Unmarshal(caddyfileBytes)
	if err != nil {
		return nil, err
	}

	seen := map[string]struct{}{}
	var names []string
	for _, block := range container.Children {
		if block.IsGlobalBlock() || block.IsSnippet() || block.IsMatcher() {
			continue
		}
		for _, token := range block.Keys {
			for _, part := range splitNameToken(token) {
				name, ok := normalizeName(part)
				if !ok {
					continue
				}
				if _, exists := seen[name]; exists {
					continue
				}
				seen[name] = struct{}{}
				names = append(names, name)
			}
		}
	}

	return names, nil
}

func splitNameToken(token string) []string {
	parts := strings.Split(token, ",")
	var out []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func normalizeName(raw string) (string, bool) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", false
	}

	if strings.Contains(name, "://") {
		u, err := url.Parse(name)
		if err == nil {
			name = u.Host
		}
	}

	if strings.Contains(name, "/") {
		name = strings.SplitN(name, "/", 2)[0]
	}

	if strings.HasPrefix(name, ":") {
		return "", false
	}

	if strings.HasPrefix(name, "[") && strings.HasSuffix(name, "]") {
		name = strings.TrimPrefix(strings.TrimSuffix(name, "]"), "[")
	}

	if host, _, err := net.SplitHostPort(name); err == nil {
		name = host
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return "", false
	}

	return name, true
}
