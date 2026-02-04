package namepublish

import (
	"fmt"
	"strings"

	"github.com/lucaslorentz/caddy-docker-proxy/v2/config"
	"go.uber.org/zap"
)

// BuildPublishers constructs enabled publishers from options.
func BuildPublishers(opts config.NamePublishOptions, logger *zap.Logger) ([]PublisherEntry, error) {
	if logger == nil {
		logger = zap.NewNop()
	}
	if !opts.Enabled {
		return nil, nil
	}
	if strings.TrimSpace(opts.CaddyHost) == "" {
		return nil, fmt.Errorf("name publish enabled but caddy host is empty")
	}

	var publishers []PublisherEntry
	if opts.Technitium.Enabled {
		publisher, err := NewTechnitiumPublisher(opts.Technitium, logger)
		if err != nil {
			return nil, err
		}
		publishers = append(publishers, PublisherEntry{Name: "technitium", Publisher: publisher})
	}

	if opts.Avahi.Enabled {
		publisher, err := NewAvahiPublisher(opts.Avahi, logger)
		if err != nil {
			return nil, err
		}
		publishers = append(publishers, PublisherEntry{Name: "avahi", Publisher: publisher})
	}

	return publishers, nil
}
