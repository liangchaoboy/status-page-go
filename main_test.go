package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSaveAndLoadState(t *testing.T) {
	originalStateFile := config.StateFile

	statusMu.RLock()
	originalCurrentStatus := currentStatus
	originalStatusHistory := append([]HistoryItem(nil), statusHistory...)
	originalAlerts := append([]AlertItem(nil), alerts...)
	statusMu.RUnlock()

	webhookMu.RLock()
	originalWebhooks := append([]Webhook(nil), webhooks...)
	webhookMu.RUnlock()

	t.Cleanup(func() {
		config.StateFile = originalStateFile

		statusMu.Lock()
		currentStatus = originalCurrentStatus
		statusHistory = originalStatusHistory
		alerts = originalAlerts
		statusMu.Unlock()

		webhookMu.Lock()
		webhooks = originalWebhooks
		webhookMu.Unlock()
	})

	config.StateFile = filepath.Join(t.TempDir(), "state", "status-state.json")

	ratio := 0.12
	statusMu.Lock()
	currentStatus = &CurrentStatus{
		Timestamp: "2026-07-06T10:00:00Z",
		Status5m:  &UpstreamData{ErrorRatio: ratio},
		Overall:   OverallStatus{Status: "degraded", Message: "服务性能下降"},
	}
	statusHistory = []HistoryItem{
		{Timestamp: "2026-07-06T10:00:00Z", ErrorRatio: &ratio, Status: "degraded"},
	}
	alerts = []AlertItem{
		{Type: "alert", Message: "test alert", Timestamp: "2026-07-06T10:00:00Z"},
	}
	statusMu.Unlock()

	webhookMu.Lock()
	webhooks = []Webhook{
		{ID: "wh-1", URL: "https://example.com/webhook", Secret: "secret", Name: "primary"},
	}
	webhookMu.Unlock()

	saveState()

	if _, err := os.Stat(config.StateFile); err != nil {
		t.Fatalf("state file was not written: %v", err)
	}

	statusMu.Lock()
	currentStatus = nil
	statusHistory = nil
	alerts = nil
	statusMu.Unlock()

	webhookMu.Lock()
	webhooks = nil
	webhookMu.Unlock()

	loadState()

	statusMu.RLock()
	if currentStatus == nil || currentStatus.Overall.Status != "degraded" {
		t.Fatalf("current status was not restored: %#v", currentStatus)
	}
	if len(statusHistory) != 1 || statusHistory[0].ErrorRatio == nil || *statusHistory[0].ErrorRatio != ratio {
		t.Fatalf("history was not restored: %#v", statusHistory)
	}
	if len(alerts) != 1 || alerts[0].Type != "alert" {
		t.Fatalf("alerts were not restored: %#v", alerts)
	}
	statusMu.RUnlock()

	webhookMu.RLock()
	if len(webhooks) != 1 || webhooks[0].ID != "wh-1" || webhooks[0].Secret != "secret" {
		t.Fatalf("webhooks were not restored: %#v", webhooks)
	}
	webhookMu.RUnlock()
}

func TestSaveAndLoadConfig(t *testing.T) {
	configMu.RLock()
	originalConfig := config
	configMu.RUnlock()

	t.Cleanup(func() {
		configMu.Lock()
		config = originalConfig
		configMu.Unlock()
	})

	configMu.Lock()
	config.ConfigFile = filepath.Join(t.TempDir(), "config", "status-config.json")
	config.UpstreamAPI = "http://example.test/open/v1/upstream-status"
	config.Port = "13001"
	config.AlertThreshold = 0.025
	config.AlertConsecutivePoints = 3
	config.CheckInterval = 7
	config.StateFile = filepath.Join(t.TempDir(), "state", "status-state.json")
	config.ErrorLogFile = filepath.Join(t.TempDir(), "logs", "status-error.log")
	config.AuthEnabled = true
	config.AuthUsername = "ops"
	config.AuthPassword = "strong-password"
	config.AuthSecret = "session-secret"
	config.AuthSessionTTL = 24 * time.Hour
	config.AuthCookieSecure = true
	configMu.Unlock()

	if err := saveConfigFile(); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

	configMu.Lock()
	config.UpstreamAPI = "http://wrong.example"
	config.Port = "3001"
	config.AlertThreshold = 0.01
	config.AlertConsecutivePoints = 2
	config.CheckInterval = 1
	config.StateFile = "wrong-state.json"
	config.ErrorLogFile = "wrong-error.log"
	config.AuthEnabled = false
	config.AuthUsername = "wrong"
	config.AuthPassword = "wrong"
	config.AuthSecret = "wrong"
	config.AuthSessionTTL = time.Hour
	config.AuthCookieSecure = false
	configMu.Unlock()

	loadConfigFile()

	configMu.RLock()
	defer configMu.RUnlock()

	if config.UpstreamAPI != "http://example.test/open/v1/upstream-status" ||
		config.Port != "13001" ||
		config.AlertThreshold != 0.025 ||
		config.AlertConsecutivePoints != 3 ||
		config.CheckInterval != 7 ||
		!strings.HasSuffix(config.StateFile, "status-state.json") ||
		!strings.HasSuffix(config.ErrorLogFile, "status-error.log") ||
		!config.AuthEnabled ||
		config.AuthUsername != "ops" ||
		config.AuthPassword != "strong-password" ||
		config.AuthSecret != "session-secret" ||
		config.AuthSessionTTL != 24*time.Hour ||
		!config.AuthCookieSecure {
		t.Fatalf("config was not restored: %#v", config)
	}
}

func TestLogErrorToLocalFile(t *testing.T) {
	originalErrorLogFile := config.ErrorLogFile
	t.Cleanup(func() {
		config.ErrorLogFile = originalErrorLogFile
	})

	config.ErrorLogFile = filepath.Join(t.TempDir(), "logs", "status-error.log")
	logErrorToLocalFile("[Fetch] test error: %s", "upstream unavailable")

	data, err := os.ReadFile(config.ErrorLogFile)
	if err != nil {
		t.Fatalf("read error log failed: %v", err)
	}
	if !strings.Contains(string(data), "upstream unavailable") {
		t.Fatalf("expected error log to contain message, got %q", string(data))
	}
}

func TestAuthSessionCookie(t *testing.T) {
	withTestAuthConfig(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(newSessionCookie())
	if !isAuthenticated(req) {
		t.Fatal("expected signed session cookie to authenticate")
	}

	tampered := httptest.NewRequest(http.MethodGet, "/", nil)
	cookie, _ := req.Cookie(sessionCookieName)
	tamperedCookie := *cookie
	tamperedCookie.Value += "x"
	tampered.AddCookie(&tamperedCookie)
	if isAuthenticated(tampered) {
		t.Fatal("expected tampered session cookie to be rejected")
	}
}

func TestRequireAuthProtectsAPI(t *testing.T) {
	withTestAuthConfig(t)

	handler := requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated API request, got %d", rec.Code)
	}
}

func TestAlertRuleActiveRequiresConsecutivePointsAboveThreshold(t *testing.T) {
	ratio := func(v float64) *float64 { return &v }

	history := []HistoryItem{
		{ErrorRatio: ratio(0.02)},
	}
	if alertRuleActive(history, 2, 0.01) {
		t.Fatal("single high point should not trigger a two-point rule")
	}

	history = append(history, HistoryItem{ErrorRatio: ratio(0.015)})
	if !alertRuleActive(history, 2, 0.01) {
		t.Fatal("two consecutive points above threshold should trigger")
	}

	history = append(history, HistoryItem{ErrorRatio: ratio(0.01)})
	if alertRuleActive(history, 2, 0.01) {
		t.Fatal("point equal to threshold should not count as above threshold")
	}

	history = append(history, HistoryItem{ErrorRatio: nil}, HistoryItem{ErrorRatio: ratio(0.02)})
	if alertRuleActive(history, 2, 0.01) {
		t.Fatal("missing error ratio should break consecutive high points")
	}
}

func TestWeComWebhookBody(t *testing.T) {
	alert := AlertItem{
		Type:      "alert",
		Message:   "服务可用性告警",
		Timestamp: "2026-07-06T10:00:00Z",
		Data: map[string]interface{}{
			"error_ratio":        0.02,
			"threshold":          0.01,
			"consecutive_points": 2,
			"status":             "degraded",
		},
	}

	body, err := buildWebhookBody(webhookTypeWeCom, alert)
	if err != nil {
		t.Fatalf("build webhook body failed: %v", err)
	}

	var payload struct {
		MsgType string `json:"msgtype"`
		Text    struct {
			Content string `json:"content"`
		} `json:"text"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("invalid json body: %v", err)
	}

	if payload.MsgType != "text" {
		t.Fatalf("expected wecom text message, got %q", payload.MsgType)
	}
	for _, expected := range []string{"服务可用性告警", "错误率: 2.00%", "阈值: 1.00%", "连续点数: 2"} {
		if !strings.Contains(payload.Text.Content, expected) {
			t.Fatalf("expected content to contain %q, got %q", expected, payload.Text.Content)
		}
	}
}

func TestSendWeComWebhookValidatesErrCode(t *testing.T) {
	err := validateWeComResponse([]byte(`{"errcode":93000,"errmsg":"invalid webhook"}`))
	if err == nil || !strings.Contains(err.Error(), "wecom errcode 93000") {
		t.Fatalf("expected wecom errcode error, got %v", err)
	}

	if err := validateWeComResponse([]byte(`{"errcode":0,"errmsg":"ok"}`)); err != nil {
		t.Fatalf("expected successful wecom response, got %v", err)
	}
}

func withTestAuthConfig(t *testing.T) {
	t.Helper()

	originalEnabled := config.AuthEnabled
	originalUsername := config.AuthUsername
	originalPassword := config.AuthPassword
	originalSecret := config.AuthSecret
	originalTTL := config.AuthSessionTTL
	originalSecure := config.AuthCookieSecure

	config.AuthEnabled = true
	config.AuthUsername = "admin"
	config.AuthPassword = "password"
	config.AuthSecret = "test-secret"
	config.AuthSessionTTL = time.Hour
	config.AuthCookieSecure = false

	t.Cleanup(func() {
		config.AuthEnabled = originalEnabled
		config.AuthUsername = originalUsername
		config.AuthPassword = originalPassword
		config.AuthSecret = originalSecret
		config.AuthSessionTTL = originalTTL
		config.AuthCookieSecure = originalSecure
	})
}
