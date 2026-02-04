package namepublish

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type testPublisher struct {
	calls chan struct{}
}

func (p *testPublisher) Publish(_ context.Context, _ []string, _ string) error {
	p.calls <- struct{}{}
	return nil
}

func TestManagerDedupesByVersionAndHash(t *testing.T) {
	calls := make(chan struct{}, 2)
	publisher := &testPublisher{calls: calls}
	manager := NewManager("caddy.example.com", []PublisherEntry{{Name: "test", Publisher: publisher}}, zap.NewNop())
	require.NotNil(t, manager)

	caddyfile := []byte("example.com {\nrespond \"ok\"\n}\n")

	manager.MaybePublish(1, caddyfile)
	select {
	case <-calls:
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for publish")
	}

	manager.MaybePublish(1, caddyfile)
	select {
	case <-calls:
		t.Fatal("unexpected second publish for same version and config")
	case <-time.After(200 * time.Millisecond):
	}
}
