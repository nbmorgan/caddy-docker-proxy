package namepublish

import "context"

// Publisher publishes desired names to an external system.
type Publisher interface {
	Publish(ctx context.Context, names []string, caddyHost string) error
}

// PublisherEntry ties a publisher to a display name for logging.
type PublisherEntry struct {
	Name      string
	Publisher Publisher
}
