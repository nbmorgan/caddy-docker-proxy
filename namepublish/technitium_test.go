package namepublish

import (
	"context"
	"encoding/json"
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

func TestTechnitiumPublishAddsManagedComment(t *testing.T) {
	var addValues map[string][]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/zones/records/get" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "ok",
				"response": map[string]interface{}{
					"records": []interface{}{},
				},
			})
			return
		}
		if r.URL.Path == "/api/zones/records/add" {
			require.Equal(t, "POST", r.Method)
			require.NoError(t, r.ParseForm())
			addValues = r.PostForm
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
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

	require.Equal(t, []string{"token"}, addValues["token"])
	require.Equal(t, []string{"example.com"}, addValues["zone"])
	require.Equal(t, []string{"app.example.com"}, addValues["domain"])
	require.Equal(t, []string{"CNAME"}, addValues["type"])
	require.Equal(t, []string{"caddy.example.com"}, addValues["cname"])
	require.Equal(t, []string{"true"}, addValues["overwrite"])
	require.Equal(t, []string{"60"}, addValues["ttl"])
	require.Equal(t, []string{technitiumManagedComment}, addValues["comments"])
}
