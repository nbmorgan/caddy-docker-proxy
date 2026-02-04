package namepublish

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lucaslorentz/caddy-docker-proxy/v2/config"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestTechnitiumRecordParams(t *testing.T) {
	publisher, err := NewTechnitiumPublisher(config.TechnitiumOptions{
		BaseURL: "http://example.test",
		Token:   "token",
		Zone:    "example.com",
	}, zap.NewNop())
	require.NoError(t, err)

	recordType, key, value, err := publisher.recordParams("192.168.1.10")
	require.NoError(t, err)
	require.Equal(t, "A", recordType)
	require.Equal(t, "ipAddress", key)
	require.Equal(t, "192.168.1.10", value)

	recordType, key, value, err = publisher.recordParams("caddy.example.com")
	require.NoError(t, err)
	require.Equal(t, "CNAME", recordType)
	require.Equal(t, "cname", key)
	require.Equal(t, "caddy.example.com", value)
}

func TestTechnitiumPublishRequests(t *testing.T) {
	var gotValues map[string][]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "POST", r.Method)
		require.NoError(t, r.ParseForm())
		gotValues = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	publisher, err := NewTechnitiumPublisher(config.TechnitiumOptions{
		BaseURL: server.URL,
		Token:   "token",
		Zone:    "example.com",
		TTL:     60,
	}, zap.NewNop())
	require.NoError(t, err)

	err = publisher.Publish(context.Background(), []string{"app.example.com"}, "caddy.example.com")
	require.NoError(t, err)

	require.Equal(t, []string{"token"}, gotValues["token"])
	require.Equal(t, []string{"example.com"}, gotValues["zone"])
	require.Equal(t, []string{"app.example.com"}, gotValues["domain"])
	require.Equal(t, []string{"CNAME"}, gotValues["type"])
	require.Equal(t, []string{"caddy.example.com"}, gotValues["cname"])
	require.Equal(t, []string{"true"}, gotValues["overwrite"])
	require.Equal(t, []string{"60"}, gotValues["ttl"])
}
