package panelapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"dockium/internal/control"
	"dockium/internal/db"
	"dockium/internal/hub"
	"dockium/internal/panelapi"
	"dockium/internal/realtime"
	"dockium/internal/scheduler"
)

type testServer struct {
	api      *httptest.Server
	store    *db.DB
	admin    *http.Client
	operator *http.Client
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	ctx := context.Background()
	store, err := db.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	publisher := realtime.NewPublisher()
	agentHub := hub.New(store, publisher, logger)
	controller := control.New(store, agentHub, publisher, logger)
	taskScheduler := scheduler.New(store, controller, agentHub, logger)
	server := panelapi.New(panelapi.Options{
		Store:         store,
		Hub:           agentHub,
		Publisher:     publisher,
		Control:       controller,
		Scheduler:     taskScheduler,
		Log:           logger,
		Version:       "test-version",
		AgentEndpoint: "localhost:3443",
	})
	handler, err := server.Handler()
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	apiServer := httptest.NewServer(handler)
	t.Cleanup(apiServer.Close)

	adminJar, _ := cookiejar.New(nil)
	operatorJar, _ := cookiejar.New(nil)
	return &testServer{
		api:      apiServer,
		store:    store,
		admin:    &http.Client{Jar: adminJar},
		operator: &http.Client{Jar: operatorJar},
	}
}

func (s *testServer) call(t *testing.T, client *http.Client, procedure string, payload any, headers map[string]string) (int, map[string]any) {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	request, err := http.NewRequest(http.MethodPost, s.api.URL+procedure, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Connect-Protocol-Version", "1")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("do %s: %v", procedure, err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	var decoded map[string]any
	if len(data) > 0 {
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("decode %s (%d): %v in %q", procedure, response.StatusCode, err, data)
		}
	}
	return response.StatusCode, decoded
}

func errorCode(payload map[string]any) string {
	code, _ := payload["code"].(string)
	return code
}

func TestSetupLoginPermissionsAndCsrf(t *testing.T) {
	server := newTestServer(t)

	status, payload := server.call(t, server.admin, "/dockium.panel.v1.SetupService/GetStatus", map[string]any{}, nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if payload["version"] != "test-version" {
		t.Fatalf("version = %v", payload["version"])
	}

	status, payload = server.call(t, server.admin, "/dockium.panel.v1.SetupService/CreateAdmin", map[string]any{
		"credentials": map[string]string{"login": "admin", "password": "Dockium2026!"},
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("create admin status = %d, payload = %v", status, payload)
	}
	cookies := server.admin.Jar.Cookies(mustURL(t, server.api.URL))
	if len(cookies) == 0 {
		t.Fatal("session cookie missing")
	}

	status, payload = server.call(t, server.admin, "/dockium.panel.v1.AuthService/Me", map[string]any{}, nil)
	if status != http.StatusOK || payload["user"] == nil {
		t.Fatalf("me status = %d, payload = %v", status, payload)
	}
	user, _ := payload["user"].(map[string]any)
	if user["role"] != "admin" || payload["securityAccess"] != true {
		t.Fatalf("me payload = %v", payload)
	}

	status, payload = server.call(t, server.admin, "/dockium.panel.v1.SetupService/CreateAdmin", map[string]any{
		"credentials": map[string]string{"login": "other", "password": "Dockium2026!"},
	}, nil)
	if errorCode(payload) != "failed_precondition" {
		t.Fatalf("second setup status = %d, payload = %v", status, payload)
	}

	status, payload = server.call(t, server.admin, "/dockium.panel.v1.UserService/Create", map[string]any{
		"login": "operator", "password": "Operator2026!", "role": "user",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("create user status = %d, payload = %v", status, payload)
	}

	status, payload = server.call(t, server.operator, "/dockium.panel.v1.AuthService/Login", map[string]any{
		"login": "operator", "password": "Operator2026!",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("operator login status = %d, payload = %v", status, payload)
	}

	_, payload = server.call(t, server.operator, "/dockium.panel.v1.AgentService/List", map[string]any{}, nil)
	if errorCode(payload) != "permission_denied" {
		t.Fatalf("operator agent list payload = %v", payload)
	}
	_, payload = server.call(t, server.operator, "/dockium.panel.v1.InstanceService/Create", map[string]any{
		"name": "bad", "agentId": "1", "image": "alpine",
	}, nil)
	if errorCode(payload) != "permission_denied" {
		t.Fatalf("operator create instance payload = %v", payload)
	}

	status, payload = server.call(t, server.admin, "/dockium.panel.v1.AgentService/Create", map[string]any{
		"name": "local", "country": "RU",
	}, nil)
	if status != http.StatusOK || payload["token"] == nil || payload["endpoint"] != "localhost:3443" {
		t.Fatalf("create agent status = %d, payload = %v", status, payload)
	}

	_, payload = server.call(t, server.admin, "/dockium.panel.v1.AgentService/Update", map[string]any{
		"id": "1", "name": "local", "country": "RU", "enabled": false,
	}, nil)
	disabledFlag, _ := payload["enabled"].(bool)
	if status != http.StatusOK || disabledFlag {
		t.Fatalf("disable agent status = %d, payload = %v", status, payload)
	}
	_, payload = server.call(t, server.admin, "/dockium.panel.v1.AgentService/List", map[string]any{}, nil)
	agentItems, _ := payload["items"].([]any)
	if len(agentItems) != 1 {
		t.Fatalf("agents = %v", payload)
	}
	agentItem, _ := agentItems[0].(map[string]any)
	agentEnabled, _ := agentItem["enabled"].(bool)
	if agentEnabled {
		t.Fatalf("agent enabled = %v", agentItem["enabled"])
	}
	_, payload = server.call(t, server.admin, "/dockium.panel.v1.AgentService/Update", map[string]any{
		"id": "1", "name": "local", "country": "RU", "enabled": true,
	}, nil)
	if status != http.StatusOK || payload["enabled"] != true {
		t.Fatalf("enable agent status = %d, payload = %v", status, payload)
	}
	_, payload = server.call(t, server.admin, "/dockium.panel.v1.AgentService/Reconnect", map[string]any{"id": "1"}, nil)
	if errorCode(payload) != "failed_precondition" {
		t.Fatalf("reconnect offline payload = %v", payload)
	}

	status, payload = server.call(t, server.admin, "/dockium.panel.v1.InstanceService/Create", map[string]any{
		"name": "Web test", "agentId": "1", "image": "nginx:alpine", "autoStart": true,
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("create instance status = %d, payload = %v", status, payload)
	}
	instances, err := server.store.ListInstances(context.Background())
	if err != nil || len(instances) != 1 {
		t.Fatalf("instances = %v, err = %v", instances, err)
	}
	instance := instances[0]
	if instance.Status != db.StatusStopped {
		t.Fatalf("offline create status = %s", instance.Status)
	}

	operatorUser, err := server.store.UserByLogin(context.Background(), "operator")
	if err != nil {
		t.Fatalf("operator lookup: %v", err)
	}
	status, payload = server.call(t, server.admin, "/dockium.panel.v1.UserService/SetAccess", map[string]any{
		"userId": operatorUser.ID,
		"items": []map[string]any{
			{"instanceId": instance.ID, "control": true, "console": true, "files": true},
		},
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("set access status = %d, payload = %v", status, payload)
	}

	_, payload = server.call(t, server.operator, "/dockium.panel.v1.InstanceService/List", map[string]any{}, nil)
	items, _ := payload["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("operator instances = %v", payload)
	}
	first, _ := items[0].(map[string]any)
	permissions, _ := first["permissions"].(map[string]any)
	if permissions["control"] != true || permissions["console"] != true {
		t.Fatalf("permissions = %v", permissions)
	}

	_, payload = server.call(t, server.operator, "/dockium.panel.v1.InstanceService/Start", map[string]any{"id": instance.ID}, nil)
	if errorCode(payload) != "failed_precondition" {
		t.Fatalf("start offline payload = %v", payload)
	}

	_, payload = server.call(t, server.admin, "/dockium.panel.v1.AuthService/Me", map[string]any{}, map[string]string{"Origin": "http://evil.example"})
	if errorCode(payload) != "permission_denied" {
		t.Fatalf("csrf payload = %v", payload)
	}

	status, payload = server.call(t, server.admin, "/dockium.panel.v1.AuthService/Me", map[string]any{}, map[string]string{"Origin": "http://127.0.0.1:49590"})
	if status != http.StatusOK {
		t.Fatalf("loopback origin status = %d, payload = %v", status, payload)
	}

	status, payload = server.call(t, server.admin, "/dockium.panel.v1.AuthService/Logout", map[string]any{}, nil)
	if status != http.StatusOK {
		t.Fatalf("logout status = %d, payload = %v", status, payload)
	}
	_, payload = server.call(t, server.admin, "/dockium.panel.v1.AuthService/Me", map[string]any{}, nil)
	if errorCode(payload) != "unauthenticated" {
		t.Fatalf("logout me payload = %v", payload)
	}

	_, payload = server.call(t, server.operator, "/dockium.panel.v1.AuthService/Login", map[string]any{
		"login": "operator", "password": "wrong",
	}, nil)
	if errorCode(payload) != "unauthenticated" {
		t.Fatalf("wrong password payload = %v", payload)
	}
}

func mustURL(t *testing.T, value string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(value)
	if err != nil {
		t.Fatalf("url: %v", err)
	}
	return parsed
}
