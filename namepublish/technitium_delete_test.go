package namepublish

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lucaslorentz/caddy-docker-proxy/v2/config"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestTechnitiumDeletesManagedOnly(t *testing.T) {
	var deleteCalls []map[string][]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/zones/records/get":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "ok",
				"response": map[string]interface{}{
					"records": []map[string]interface{}{
						{
							"name":     "old.int.nbmc.app",
							"type":     "CNAME",
							"comments": technitiumManagedComment,
							"rData":    map[string]interface{}{"cname": "caddy.int.nbmc.app"},
						},
						{
							"name":     "unmanaged.int.nbmc.app",
							"type":     "CNAME",
							"comments": "manual",
							"rData":    map[string]interface{}{"cname": "manual.int.nbmc.app"},
						},
					},
				},
			})
		case "/api/zones/records/delete":
			require.NoError(t, r.ParseForm())
			deleteCalls = append(deleteCalls, r.PostForm)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/api/zones/records/add":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	publisher, err := NewTechnitiumPublisher(config.TechnitiumOptions{
		BaseURL: server.URL,
		Token:   "token",
		Zone:    "int.nbmc.app",
	}, zap.NewNop())
	require.NoError(t, err)

	err = publisher.Publish(context.Background(), []string{"keep.int.nbmc.app"}, "caddy.int.nbmc.app")
	require.NoError(t, err)

	require.Len(t, deleteCalls, 1)
	require.Equal(t, []string{"old.int.nbmc.app"}, deleteCalls[0]["domain"])
	require.Equal(t, []string{"CNAME"}, deleteCalls[0]["type"])
	require.Equal(t, []string{"caddy.int.nbmc.app"}, deleteCalls[0]["value"])
}

func TestTechnitiumSkipsUnmanagedConflict(t *testing.T) {
	var addCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/zones/records/get":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "ok",
				"response": map[string]interface{}{
					"records": []map[string]interface{}{
						{
							"name":     "app.int.nbmc.app",
							"type":     "CNAME",
							"comments": "manual",
							"rData":    map[string]interface{}{"cname": "other.int.nbmc.app"},
						},
					},
				},
			})
		case "/api/zones/records/add":
			addCalled = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	publisher, err := NewTechnitiumPublisher(config.TechnitiumOptions{
		BaseURL: server.URL,
		Token:   "token",
		Zone:    "int.nbmc.app",
	}, zap.NewNop())
	require.NoError(t, err)

	err = publisher.Publish(context.Background(), []string{"app.int.nbmc.app"}, "caddy.int.nbmc.app")
	require.NoError(t, err)
	require.False(t, addCalled)
}

func TestTechnitiumDeletesManagedWrongType(t *testing.T) {
	var deleteCalls []string
	var addCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/zones/records/get":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "ok",
				"response": map[string]interface{}{
					"records": []map[string]interface{}{
						{
							"name":     "app.int.nbmc.app",
							"type":     "A",
							"comments": technitiumManagedComment,
							"rData":    map[string]interface{}{"ipAddress": "10.0.0.2"},
						},
					},
				},
			})
		case "/api/zones/records/delete":
			require.NoError(t, r.ParseForm())
			deleteCalls = append(deleteCalls, strings.Join(r.PostForm["type"], ","))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/api/zones/records/add":
			addCalled = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	publisher, err := NewTechnitiumPublisher(config.TechnitiumOptions{
		BaseURL: server.URL,
		Token:   "token",
		Zone:    "int.nbmc.app",
	}, zap.NewNop())
	require.NoError(t, err)

	err = publisher.Publish(context.Background(), []string{"app.int.nbmc.app"}, "caddy.int.nbmc.app")
	require.NoError(t, err)
	require.True(t, addCalled)
	require.Len(t, deleteCalls, 1)
	require.Equal(t, "A", deleteCalls[0])
}
