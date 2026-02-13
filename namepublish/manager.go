package namepublish

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

const publishTimeout = 3*time.Minute + 15*time.Second

// Manager coordinates name publication after config apply.
type Manager struct {
	caddyHost  string
	publishers []PublisherEntry
	logger     *zap.Logger

	mu          sync.Mutex
	lastVersion int64
	lastHash    string
}

// NewManager builds a manager for the provided publishers and caddy host.
func NewManager(caddyHost string, publishers []PublisherEntry, logger *zap.Logger) *Manager {
	if logger == nil {
		logger = zap.NewNop()
	}
	if caddyHost == "" || len(publishers) == 0 {
		return nil
	}
	return &Manager{
		caddyHost:  caddyHost,
		publishers: publishers,
		logger:     logger,
	}
}

// MaybePublish extracts names and publishes once per config version/hash.
func (m *Manager) MaybePublish(version int64, caddyfileBytes []byte) {
	if m == nil {
		return
	}

	names, err := ExtractDesiredNames(caddyfileBytes)
	if err != nil {
		m.logger.Warn("Failed to extract desired names", zap.Error(err))
		return
	}
	if len(names) == 0 {
		return
	}

	hash := hashNames(names, m.caddyHost)

	m.mu.Lock()
	if version == m.lastVersion && hash == m.lastHash {
		m.mu.Unlock()
		return
	}
	m.lastVersion = version
	m.lastHash = hash
	m.mu.Unlock()

	m.logger.Info(
		"Publishing names",
		zap.Int("count", len(names)),
		zap.Strings("names", names),
	)

	go m.publish(names)
}

func (m *Manager) publish(names []string) {
	ctx, cancel := context.WithTimeout(context.Background(), publishTimeout)
	defer cancel()

	for _, entry := range m.publishers {
		if entry.Publisher == nil {
			continue
		}
		start := time.Now()
		if err := entry.Publisher.Publish(ctx, names, m.caddyHost); err != nil {
			m.logger.Warn(
				"Name publisher failed",
				zap.String("publisher", entry.Name),
				zap.Duration("elapsed", time.Since(start)),
				zap.Error(err),
			)
		} else {
			m.logger.Info(
				"Name publisher succeeded",
				zap.String("publisher", entry.Name),
				zap.Duration("elapsed", time.Since(start)),
			)
		}
	}
}

func hashNames(names []string, caddyHost string) string {
	sorted := append([]string{}, names...)
	sort.Strings(sorted)
	joined := strings.Join(sorted, ",") + "|" + caddyHost
	sum := sha256.Sum256([]byte(joined))
	return hex.EncodeToString(sum[:])
}
