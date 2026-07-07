package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// ============ 配置 ============

type Config struct {
	UpstreamAPI            string
	Port                   string
	AlertThreshold         float64
	AlertConsecutivePoints int
	CheckInterval          int
	ConfigFile             string
	StateFile              string
	ErrorLogFile           string
	AuthEnabled            bool
	AuthUsername           string
	AuthPassword           string
	AuthSecret             string
	AuthSessionTTL         time.Duration
	AuthCookieSecure       bool
}

var config = Config{
	UpstreamAPI:            "https://api.fenno.ai/open/v1/upstream-status",
	Port:                   "3000",
	AlertThreshold:         0.05,
	AlertConsecutivePoints: 2,
	CheckInterval:          5, // 5 分钟粒度
	ConfigFile:             getEnv("CONFIG_FILE", "status-config.json"),
	StateFile:              "data/status-state.json",
	ErrorLogFile:           "data/status-error.log",
	AuthEnabled:            true,
	AuthUsername:           "admin",
	AuthPassword:           "",
	AuthSecret:             "",
	AuthSessionTTL:         12 * time.Hour,
	AuthCookieSecure:       false,
}

const sessionCookieName = "status_page_session"

var loginTemplate = template.Must(template.New("login").Parse(loginHTML))

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func getAlertConfig() (float64, int) {
	configMu.RLock()
	defer configMu.RUnlock()
	return config.AlertThreshold, config.AlertConsecutivePoints
}

func setAlertConfig(threshold float64, consecutivePoints int) {
	configMu.Lock()
	config.AlertThreshold = threshold
	config.AlertConsecutivePoints = consecutivePoints
	configMu.Unlock()
}

// ============ 数据结构 ============

type TimeRange struct {
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
}

type UpstreamData struct {
	TimeRange  TimeRange `json:"time_range"`
	ErrorRatio float64   `json:"error_ratio"`
}

type UpstreamResponse struct {
	Code    int          `json:"code"`
	Message string       `json:"message"`
	Data    UpstreamData `json:"data"`
}

type OverallStatus struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

type CurrentStatus struct {
	Timestamp  string        `json:"timestamp"`
	Status5m   *UpstreamData `json:"status5m"`
	Status30m  *UpstreamData `json:"status30m"`
	Status60m  *UpstreamData `json:"status60m"`
	Status360m *UpstreamData `json:"status360m"` // 6 小时
	Overall    OverallStatus `json:"overall"`
}

type HistoryItem struct {
	Timestamp  string   `json:"timestamp"`
	ErrorRatio *float64 `json:"error_ratio"`
	Status     string   `json:"status"`
}

type AlertItem struct {
	Type             string      `json:"type"`
	Message          string      `json:"message"`
	Timestamp        string      `json:"timestamp"`
	Data             interface{} `json:"data"`
	WebhooksNotified int         `json:"webhooksNotified,omitempty"`
}

type Webhook struct {
	ID            string `json:"id"`
	URL           string `json:"url"`
	Secret        string `json:"secret,omitempty"`
	Name          string `json:"name"`
	Type          string `json:"type,omitempty"`
	CreatedAt     string `json:"createdAt"`
	LastTriggered string `json:"lastTriggered,omitempty"`
	TriggerCount  int    `json:"triggerCount"`
}

const (
	webhookTypeGeneric = "generic"
	webhookTypeWeCom   = "wecom"
)

type PersistedState struct {
	CurrentStatus *CurrentStatus `json:"currentStatus,omitempty"`
	StatusHistory []HistoryItem  `json:"statusHistory"`
	Alerts        []AlertItem    `json:"alerts"`
	Webhooks      []Webhook      `json:"webhooks"`
	SavedAt       string         `json:"savedAt"`
}

type PersistedConfig struct {
	UpstreamAPI            *string  `json:"upstreamAPI,omitempty"`
	Port                   *string  `json:"port,omitempty"`
	AlertThreshold         *float64 `json:"alertThreshold,omitempty"`
	AlertConsecutivePoints *int     `json:"alertConsecutivePoints,omitempty"`
	CheckInterval          *int     `json:"checkInterval,omitempty"`
	StateFile              *string  `json:"stateFile,omitempty"`
	ErrorLogFile           *string  `json:"errorLogFile,omitempty"`
	AuthEnabled            *bool    `json:"authEnabled,omitempty"`
	AuthUsername           *string  `json:"authUsername,omitempty"`
	AuthPassword           *string  `json:"authPassword,omitempty"`
	AuthSecret             *string  `json:"authSecret,omitempty"`
	AuthSessionHours       *int     `json:"authSessionHours,omitempty"`
	AuthCookieSecure       *bool    `json:"authCookieSecure,omitempty"`
	UpdatedAt              string   `json:"updatedAt"`
}

// ============ 全局状态 ============

var (
	configMu sync.RWMutex

	statusMu      sync.RWMutex
	currentStatus *CurrentStatus
	statusHistory []HistoryItem
	alerts        []AlertItem

	webhookMu sync.RWMutex
	webhooks  []Webhook

	stateMu    sync.Mutex
	errorLogMu sync.Mutex
)

// ============ 主函数 ============

func main() {
	loadConfigFile()
	validateConfig()

	// 初始化
	statusHistory = make([]HistoryItem, 0)
	alerts = make([]AlertItem, 0)
	webhooks = make([]Webhook, 0)

	loadState()

	// 演示用：预填充历史数据（SEED_HISTORY=1 时按 5 分钟粒度回填 24 小时）
	if os.Getenv("SEED_HISTORY") == "1" && len(statusHistory) == 0 {
		seedHistory()
		saveState()
	}

	// 设置路由
	rootMux := http.NewServeMux()
	protectedMux := http.NewServeMux()

	rootMux.HandleFunc("/login", handleLogin)
	rootMux.HandleFunc("/logout", handleLogout)

	// 静态文件
	protectedMux.HandleFunc("/", serveIndex)
	protectedMux.HandleFunc("/api-docs", serveAPIDocs)

	// API
	protectedMux.HandleFunc("/api/status", handleStatus)
	protectedMux.HandleFunc("/api/status/history", handleStatusHistory)
	protectedMux.HandleFunc("/api/alerts", handleAlerts)
	protectedMux.HandleFunc("/api/config", handleConfig)
	protectedMux.HandleFunc("/api/webhooks", handleWebhooks)
	protectedMux.HandleFunc("/api/webhooks/", handleWebhookByID)

	rootMux.Handle("/", requireAuth(protectedMux))

	// 定时检查
	c := cron.New()
	c.AddFunc(fmt.Sprintf("@every %dm", config.CheckInterval), checkStatus)
	c.Start()

	// 立即检查一次
	go checkStatus()

	alertThreshold, alertConsecutivePoints := getAlertConfig()
	fmt.Printf(`
╔════════════════════════════════════════════════════════════╗
║           服务状态监控页面已启动 (Go)                         ║
╠════════════════════════════════════════════════════════════╣
║  状态页面: http://localhost:%s                         ║
║  API 文档: http://localhost:%s/api-docs                ║
╠════════════════════════════════════════════════════════════╣
║  配置:                                                     ║
║  - 上游 API: %s
║  - 告警阈值: %.1f%% 错误率                                  ║
║  - 告警连续点数: %d
║  - 检查间隔: %d 分钟                                        ║
║  - 配置文件: %s
║  - 状态文件: %s
║  - 错误日志: %s
║  - 登录鉴权: %t
╚════════════════════════════════════════════════════════════╝
`, config.Port, config.Port, config.UpstreamAPI, alertThreshold*100, alertConsecutivePoints, config.CheckInterval, config.ConfigFile, config.StateFile, config.ErrorLogFile, config.AuthEnabled)

	log.Fatal(http.ListenAndServe(":"+config.Port, rootMux))
}

// ============ 登录鉴权 ============

func validateConfig() {
	configMu.RLock()
	upstreamAPI := config.UpstreamAPI
	port := config.Port
	checkInterval := config.CheckInterval
	configMu.RUnlock()

	if strings.TrimSpace(upstreamAPI) == "" {
		log.Fatal("[Config] upstreamAPI 不能为空")
	}
	if strings.TrimSpace(port) == "" {
		log.Fatal("[Config] port 不能为空")
	}
	if checkInterval < 1 {
		log.Fatal("[Config] checkInterval 必须大于等于 1")
	}

	threshold, points := getAlertConfig()
	if err := validateAlertConfig(threshold, points); err != nil {
		log.Fatalf("[Config] %v", err)
	}
	validateAuthConfig()
}

func validateAlertConfig(threshold float64, consecutivePoints int) error {
	if threshold < 0 || threshold > 1 {
		return fmt.Errorf("alertThreshold 必须在 0 到 1 之间")
	}
	if consecutivePoints < 1 {
		return fmt.Errorf("alertConsecutivePoints 必须大于等于 1")
	}
	return nil
}

func validateAuthConfig() {
	if !config.AuthEnabled {
		log.Printf("[Auth] 登录鉴权已关闭，公网部署不建议使用")
		return
	}
	if config.AuthUsername == "" {
		log.Fatal("[Auth] authUsername 不能为空")
	}
	if config.AuthPassword == "" {
		log.Fatal("[Auth] 请在配置文件设置 authPassword 后再启动服务；如仅本地演示可设置 authEnabled=false")
	}
	if config.AuthSessionTTL <= 0 {
		log.Fatal("[Auth] authSessionHours 必须大于 0")
	}
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if !config.AuthEnabled {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	next := safeRedirectPath(r.URL.Query().Get("next"))
	if next == "" {
		next = "/"
	}

	switch r.Method {
	case "GET":
		if isAuthenticated(r) {
			http.Redirect(w, r, next, http.StatusFound)
			return
		}
		renderLogin(w, next, "")
	case "POST":
		if err := r.ParseForm(); err != nil {
			renderLogin(w, next, "请求格式不正确")
			return
		}

		next = safeRedirectPath(r.FormValue("next"))
		if next == "" {
			next = "/"
		}

		if credentialsMatch(r.FormValue("username"), r.FormValue("password")) {
			http.SetCookie(w, newSessionCookie())
			http.Redirect(w, r, next, http.StatusFound)
			return
		}

		renderLogin(w, next, "用户名或密码错误")
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   config.AuthCookieSecure,
	})
	http.Redirect(w, r, "/login", http.StatusFound)
}

func renderLogin(w http.ResponseWriter, next, errorMessage string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data := map[string]string{
		"Next":  next,
		"Error": errorMessage,
	}
	if err := loginTemplate.Execute(w, data); err != nil {
		log.Printf("[Auth] 渲染登录页失败: %v", err)
	}
}

func requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !config.AuthEnabled || isAuthenticated(r) {
			next.ServeHTTP(w, r)
			return
		}

		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"code":    401,
				"message": "Unauthorized",
			})
			return
		}

		loginURL := "/login?next=" + url.QueryEscape(r.URL.RequestURI())
		http.Redirect(w, r, loginURL, http.StatusFound)
	})
}

func credentialsMatch(username, password string) bool {
	userOK := subtle.ConstantTimeCompare([]byte(username), []byte(config.AuthUsername)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(password), []byte(config.AuthPassword)) == 1
	return userOK && passOK
}

func newSessionCookie() *http.Cookie {
	expiresAt := time.Now().UTC().Add(config.AuthSessionTTL).Unix()
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		nonce = []byte(strconv.FormatInt(time.Now().UnixNano(), 10))
	}

	message := strings.Join([]string{
		config.AuthUsername,
		strconv.FormatInt(expiresAt, 10),
		base64.RawURLEncoding.EncodeToString(nonce),
	}, "|")
	signature := signSession(message)
	value := base64.RawURLEncoding.EncodeToString([]byte(message + "|" + signature))

	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    value,
		Path:     "/",
		Expires:  time.Unix(expiresAt, 0),
		MaxAge:   int(config.AuthSessionTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   config.AuthCookieSecure,
	}
}

func isAuthenticated(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return false
	}

	raw, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		return false
	}

	parts := strings.Split(string(raw), "|")
	if len(parts) != 4 {
		return false
	}

	username, expiresRaw, nonce, signature := parts[0], parts[1], parts[2], parts[3]
	if username != config.AuthUsername || nonce == "" {
		return false
	}

	expiresAt, err := strconv.ParseInt(expiresRaw, 10, 64)
	if err != nil || time.Now().UTC().Unix() > expiresAt {
		return false
	}

	message := strings.Join([]string{username, expiresRaw, nonce}, "|")
	expected := signSession(message)
	return subtle.ConstantTimeCompare([]byte(signature), []byte(expected)) == 1
}

func signSession(message string) string {
	mac := hmac.New(sha256.New, authSecret())
	mac.Write([]byte(message))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func authSecret() []byte {
	if config.AuthSecret != "" {
		return []byte(config.AuthSecret)
	}
	return []byte(config.AuthPassword)
}

func safeRedirectPath(path string) string {
	if path == "" || !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return ""
	}
	return path
}

// ============ 本地配置持久化 ============

func loadConfigFile() {
	if config.ConfigFile == "" {
		return
	}

	f, err := os.Open(config.ConfigFile)
	if os.IsNotExist(err) {
		if err := saveConfigFile(); err != nil {
			log.Printf("[Config] 创建配置文件失败: %v", err)
			return
		}
		log.Printf("[Config] 未找到配置文件，已按当前配置创建: %s", config.ConfigFile)
		return
	}
	if err != nil {
		log.Printf("[Config] 打开配置文件失败: %v", err)
		return
	}
	defer f.Close()

	var persisted PersistedConfig
	if err := json.NewDecoder(f).Decode(&persisted); err != nil {
		log.Printf("[Config] 读取配置文件失败: %v", err)
		return
	}

	applyPersistedConfig(persisted)
	if err := saveConfigFile(); err != nil {
		log.Printf("[Config] 补全配置文件失败: %v", err)
	}

	threshold, points := getAlertConfig()
	log.Printf("[Config] 已加载配置文件: threshold=%.4f consecutivePoints=%d updatedAt=%s",
		threshold, points, persisted.UpdatedAt)
}

func saveConfigFile() error {
	if config.ConfigFile == "" {
		return nil
	}

	persisted := snapshotConfig()
	if err := writeConfigFile(config.ConfigFile, persisted); err != nil {
		log.Printf("[Config] 保存配置失败: %v", err)
		return err
	}
	return nil
}

func snapshotConfig() PersistedConfig {
	configMu.RLock()
	upstreamAPI := config.UpstreamAPI
	port := config.Port
	threshold := config.AlertThreshold
	points := config.AlertConsecutivePoints
	checkInterval := config.CheckInterval
	stateFile := config.StateFile
	errorLogFile := config.ErrorLogFile
	authEnabled := config.AuthEnabled
	authUsername := config.AuthUsername
	authPassword := config.AuthPassword
	authSecret := config.AuthSecret
	authSessionHours := int(config.AuthSessionTTL / time.Hour)
	authCookieSecure := config.AuthCookieSecure
	configMu.RUnlock()

	return PersistedConfig{
		UpstreamAPI:            &upstreamAPI,
		Port:                   &port,
		AlertThreshold:         &threshold,
		AlertConsecutivePoints: &points,
		CheckInterval:          &checkInterval,
		StateFile:              &stateFile,
		ErrorLogFile:           &errorLogFile,
		AuthEnabled:            &authEnabled,
		AuthUsername:           &authUsername,
		AuthPassword:           &authPassword,
		AuthSecret:             &authSecret,
		AuthSessionHours:       &authSessionHours,
		AuthCookieSecure:       &authCookieSecure,
		UpdatedAt:              time.Now().UTC().Format(time.RFC3339),
	}
}

func applyPersistedConfig(persisted PersistedConfig) {
	configMu.Lock()
	defer configMu.Unlock()

	if persisted.UpstreamAPI != nil {
		config.UpstreamAPI = *persisted.UpstreamAPI
	}
	if persisted.Port != nil {
		config.Port = *persisted.Port
	}
	if persisted.AlertThreshold != nil {
		config.AlertThreshold = *persisted.AlertThreshold
	}
	if persisted.AlertConsecutivePoints != nil {
		config.AlertConsecutivePoints = *persisted.AlertConsecutivePoints
	}
	if persisted.CheckInterval != nil {
		config.CheckInterval = *persisted.CheckInterval
	}
	if persisted.StateFile != nil {
		config.StateFile = *persisted.StateFile
	}
	if persisted.ErrorLogFile != nil {
		config.ErrorLogFile = *persisted.ErrorLogFile
	}
	if persisted.AuthEnabled != nil {
		config.AuthEnabled = *persisted.AuthEnabled
	}
	if persisted.AuthUsername != nil {
		config.AuthUsername = *persisted.AuthUsername
	}
	if persisted.AuthPassword != nil {
		config.AuthPassword = *persisted.AuthPassword
	}
	if persisted.AuthSecret != nil {
		config.AuthSecret = *persisted.AuthSecret
	}
	if persisted.AuthSessionHours != nil {
		config.AuthSessionTTL = time.Duration(*persisted.AuthSessionHours) * time.Hour
	}
	if persisted.AuthCookieSecure != nil {
		config.AuthCookieSecure = *persisted.AuthCookieSecure
	}
}

func writeConfigFile(path string, persisted PersistedConfig) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	removeTmp := true
	defer func() {
		if removeTmp {
			_ = os.Remove(tmpName)
		}
	}()

	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(persisted); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	removeTmp = false
	return nil
}

// ============ 本地错误日志 ============

func logErrorToLocalFile(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	log.Printf("%s", message)

	if config.ErrorLogFile == "" {
		return
	}

	errorLogMu.Lock()
	defer errorLogMu.Unlock()

	if err := os.MkdirAll(filepath.Dir(config.ErrorLogFile), 0755); err != nil {
		log.Printf("[ErrorLog] mkdir failed: %v", err)
		return
	}

	f, err := os.OpenFile(config.ErrorLogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Printf("[ErrorLog] open failed: %v", err)
		return
	}
	defer f.Close()

	line := fmt.Sprintf("%s %s\n", time.Now().UTC().Format(time.RFC3339), message)
	if _, err := f.WriteString(line); err != nil {
		log.Printf("[ErrorLog] write failed: %v", err)
	}
}

// ============ 本地状态持久化 ============

func loadState() {
	if config.StateFile == "" {
		return
	}

	f, err := os.Open(config.StateFile)
	if os.IsNotExist(err) {
		log.Printf("[State] 未找到状态文件，将创建新状态: %s", config.StateFile)
		return
	}
	if err != nil {
		log.Printf("[State] 打开状态文件失败: %v", err)
		return
	}
	defer f.Close()

	var state PersistedState
	if err := json.NewDecoder(f).Decode(&state); err != nil {
		log.Printf("[State] 读取状态文件失败: %v", err)
		return
	}

	statusMu.Lock()
	currentStatus = state.CurrentStatus
	statusHistory = append([]HistoryItem(nil), state.StatusHistory...)
	pruneStatusHistoryLocked()
	alerts = append([]AlertItem(nil), state.Alerts...)
	pruneAlertsLocked()
	statusMu.Unlock()

	webhookMu.Lock()
	webhooks = append([]Webhook(nil), state.Webhooks...)
	for i := range webhooks {
		webhooks[i].Type = normalizeWebhookType(webhooks[i].Type, webhooks[i].URL)
	}
	webhookMu.Unlock()

	log.Printf("[State] 已恢复状态: history=%d alerts=%d webhooks=%d savedAt=%s",
		len(statusHistory), len(alerts), len(webhooks), state.SavedAt)
}

func saveState() {
	if config.StateFile == "" {
		return
	}

	stateMu.Lock()
	defer stateMu.Unlock()

	state := snapshotState()
	if err := writeStateFile(config.StateFile, state); err != nil {
		log.Printf("[State] 保存状态失败: %v", err)
	}
}

func snapshotState() PersistedState {
	state := PersistedState{
		SavedAt: time.Now().UTC().Format(time.RFC3339),
	}

	statusMu.RLock()
	state.CurrentStatus = currentStatus
	state.StatusHistory = append([]HistoryItem(nil), statusHistory...)
	state.Alerts = append([]AlertItem(nil), alerts...)
	statusMu.RUnlock()

	webhookMu.RLock()
	state.Webhooks = append([]Webhook(nil), webhooks...)
	webhookMu.RUnlock()

	return state
}

func writeStateFile(path string, state PersistedState) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	removeTmp := true
	defer func() {
		if removeTmp {
			_ = os.Remove(tmpName)
		}
	}()

	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(state); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	removeTmp = false
	return nil
}

func pruneStatusHistoryLocked() {
	if len(statusHistory) > 288 {
		statusHistory = statusHistory[len(statusHistory)-288:]
	}
}

func pruneAlertsLocked() {
	if len(alerts) > 100 {
		alerts = alerts[len(alerts)-100:]
	}
}

// ============ 状态检查 ============

func fetchUpstreamStatus(timeRange int) *UpstreamData {
	url := fmt.Sprintf("%s?time_range=%d", config.UpstreamAPI, timeRange)
	const maxAttempts = 2

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		data, err := fetchUpstreamStatusOnce(url)
		if err == nil {
			if attempt > 1 {
				log.Printf("[Fetch] Retry succeeded: time_range=%d attempt=%d", timeRange, attempt)
			}
			return data
		}

		lastErr = err
		if attempt < maxAttempts {
			log.Printf("[Fetch] Error: time_range=%d attempt=%d/%d: %v; retrying once", timeRange, attempt, maxAttempts, err)
			time.Sleep(500 * time.Millisecond)
			continue
		}
	}

	logErrorToLocalFile("[Fetch] Failed after retry: time_range=%d attempts=%d url=%s error=%v", timeRange, maxAttempts, url, lastErr)
	return nil
}

func fetchUpstreamStatusOnce(url string) (*UpstreamData, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result UpstreamResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response body: %w; body=%s", err, strings.TrimSpace(string(body)))
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("api code=%d message=%s", result.Code, result.Message)
	}

	return &result.Data, nil
}

func determineOverallStatus(status5m, status30m, status60m *UpstreamData, alertThreshold float64) OverallStatus {
	if status5m == nil && status30m == nil && status60m == nil {
		return OverallStatus{Status: "unknown", Message: "无法获取状态数据"}
	}

	ratio5m := 0.0
	ratio30m := 0.0
	if status5m != nil {
		ratio5m = status5m.ErrorRatio
	}
	if status30m != nil {
		ratio30m = status30m.ErrorRatio
	}

	if ratio5m >= 0.5 {
		return OverallStatus{Status: "major_outage", Message: fmt.Sprintf("服务严重故障：当前失败率 %s，基准线 %s", formatRatio(ratio5m), formatRatio(alertThreshold))}
	}
	if ratio5m >= 0.2 {
		return OverallStatus{Status: "partial_outage", Message: fmt.Sprintf("服务部分中断：当前失败率 %s，基准线 %s", formatRatio(ratio5m), formatRatio(alertThreshold))}
	}
	if ratio5m >= alertThreshold {
		return OverallStatus{Status: "degraded", Message: fmt.Sprintf("性能下降：当前失败率 %s，基准线 %s", formatRatio(ratio5m), formatRatio(alertThreshold))}
	}
	if ratio30m >= alertThreshold {
		return OverallStatus{Status: "degraded", Message: fmt.Sprintf("近期性能波动：30分钟失败率 %s，基准线 %s", formatRatio(ratio30m), formatRatio(alertThreshold))}
	}

	if status5m != nil {
		return OverallStatus{Status: "operational", Message: fmt.Sprintf("服务正常运行：当前失败率 %s，基准线 %s", formatRatio(ratio5m), formatRatio(alertThreshold))}
	}
	return OverallStatus{Status: "operational", Message: fmt.Sprintf("服务正常运行：基准线 %s", formatRatio(alertThreshold))}
}

func errorRatioValue(ratio *float64) interface{} {
	if ratio == nil {
		return nil
	}
	return *ratio
}

func formatErrorRatio(ratio *float64) string {
	if ratio == nil {
		return "未知"
	}
	return fmt.Sprintf("%.2f%%", *ratio*100)
}

func formatRatio(ratio float64) string {
	return fmt.Sprintf("%.2f%%", ratio*100)
}

func alertRuleActive(history []HistoryItem, consecutivePoints int, threshold float64) bool {
	if consecutivePoints < 1 || len(history) < consecutivePoints {
		return false
	}

	start := len(history) - consecutivePoints
	for _, item := range history[start:] {
		if item.ErrorRatio == nil || *item.ErrorRatio <= threshold {
			return false
		}
	}
	return true
}

// seedHistory 演示用：按 5 分钟粒度回填 24 小时（288 个点）的历史错误率
func seedHistory() {
	now := time.Now().UTC()
	alertThreshold, _ := getAlertConfig()
	seed := make([]HistoryItem, 0, 288)
	for i := 287; i >= 0; i-- {
		ts := now.Add(-time.Duration(i*5) * time.Minute)
		// 基线 2% 波动，偶尔制造一个尖峰以便观察不同颜色/状态
		ratio := 0.02 + math.Sin(float64(i)/12.0)*0.015
		if ratio < 0 {
			ratio = 0
		}
		if i%47 == 0 { // 周期性尖峰
			ratio += 0.12
		}
		status := "operational"
		if ratio >= 0.2 {
			status = "partial_outage"
		} else if ratio >= alertThreshold {
			status = "degraded"
		}
		r := ratio
		seed = append(seed, HistoryItem{
			Timestamp:  ts.Format(time.RFC3339),
			ErrorRatio: &r,
			Status:     status,
		})
	}
	statusHistory = seed
	log.Printf("[Seed] 已回填 %d 个历史采样点（5 分钟粒度，覆盖 24 小时）", len(seed))
}

func checkStatus() {
	timestamp := time.Now().UTC().Format(time.RFC3339)
	log.Printf("[Check] %s", timestamp)

	// 并发获取四个时间窗口的数据
	var wg sync.WaitGroup
	var status5m, status30m, status60m, status360m *UpstreamData

	wg.Add(4)
	go func() { defer wg.Done(); status5m = fetchUpstreamStatus(5) }()
	go func() { defer wg.Done(); status30m = fetchUpstreamStatus(30) }()
	go func() { defer wg.Done(); status60m = fetchUpstreamStatus(60) }()
	go func() { defer wg.Done(); status360m = fetchUpstreamStatus(360) }() // 6 小时
	wg.Wait()

	alertThreshold, alertConsecutivePoints := getAlertConfig()
	overall := determineOverallStatus(status5m, status30m, status60m, alertThreshold)

	statusMu.Lock()
	wasAlerting := alertRuleActive(statusHistory, alertConsecutivePoints, alertThreshold)

	currentStatus = &CurrentStatus{
		Timestamp:  timestamp,
		Status5m:   status5m,
		Status30m:  status30m,
		Status60m:  status60m,
		Status360m: status360m,
		Overall:    overall,
	}

	// 添加历史记录
	var errorRatio *float64
	if status5m != nil {
		errorRatio = &status5m.ErrorRatio
	}
	statusHistory = append(statusHistory, HistoryItem{
		Timestamp:  timestamp,
		ErrorRatio: errorRatio,
		Status:     overall.Status,
	})

	// 保留最近 288 条（每 5 分钟一条，约 24 小时）
	pruneStatusHistoryLocked()
	isAlerting := alertRuleActive(statusHistory, alertConsecutivePoints, alertThreshold)
	statusMu.Unlock()

	saveState()

	// 检查是否需要告警
	webhookMu.RLock()
	webhookCount := len(webhooks)
	webhookMu.RUnlock()

	if webhookCount > 0 {
		if isAlerting && !wasAlerting {
			// 最近 N 个采样点连续高于阈值，触发告警。
			go triggerAlerts(AlertItem{
				Type:    "alert",
				Message: fmt.Sprintf("服务可用性告警: 连续 %d 个采样点错误率高于 %.2f%%，当前错误率 %s", alertConsecutivePoints, alertThreshold*100, formatErrorRatio(errorRatio)),
				Data: map[string]interface{}{
					"error_ratio":        errorRatioValue(errorRatio),
					"threshold":          alertThreshold,
					"consecutive_points": alertConsecutivePoints,
					"status":             overall.Status,
				},
			})
		} else if !isAlerting && wasAlerting {
			// 连续高错误率中断，发送恢复通知。
			go triggerAlerts(AlertItem{
				Type:    "recovery",
				Message: "服务已恢复正常",
				Data: map[string]interface{}{
					"error_ratio":        errorRatioValue(errorRatio),
					"threshold":          alertThreshold,
					"consecutive_points": alertConsecutivePoints,
					"status":             overall.Status,
				},
			})
		}
	}
}

// ============ 告警推送 ============

func normalizeWebhookType(input, rawURL string) string {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case webhookTypeGeneric:
		return webhookTypeGeneric
	case webhookTypeWeCom, "wecom_robot", "wechat_work", "enterprise_wechat":
		return webhookTypeWeCom
	}
	if isWeComWebhookURL(rawURL) {
		return webhookTypeWeCom
	}
	return webhookTypeGeneric
}

func isWeComWebhookURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Hostname(), "qyapi.weixin.qq.com") && u.Path == "/cgi-bin/webhook/send"
}

func sendWebhook(webhook *Webhook, alert AlertItem) error {
	webhookType := normalizeWebhookType(webhook.Type, webhook.URL)
	body, err := buildWebhookBody(webhookType, alert)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", webhook.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "StatusPage-Alert/1.0")
	if webhook.Secret != "" {
		req.Header.Set("X-Webhook-Secret", webhook.Secret)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return readErr
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	if webhookType == webhookTypeWeCom {
		return validateWeComResponse(respBody)
	}

	return nil
}

func buildWebhookBody(webhookType string, alert AlertItem) ([]byte, error) {
	if webhookType == webhookTypeWeCom {
		return json.Marshal(map[string]interface{}{
			"msgtype": "text",
			"text": map[string]string{
				"content": formatWeComContent(alert),
			},
		})
	}
	return json.Marshal(alert)
}

func validateWeComResponse(body []byte) error {
	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("invalid wecom response: %s", strings.TrimSpace(string(body)))
	}
	if result.ErrCode != 0 {
		return fmt.Errorf("wecom errcode %d: %s", result.ErrCode, result.ErrMsg)
	}
	return nil
}

func formatWeComContent(alert AlertItem) string {
	title := "状态页通知"
	switch alert.Type {
	case "alert":
		title = "服务可用性告警"
	case "recovery":
		title = "服务恢复通知"
	case "test":
		title = "状态页测试消息"
	}

	lines := []string{
		fmt.Sprintf("[Status Page] %s", title),
		fmt.Sprintf("消息: %s", alert.Message),
		fmt.Sprintf("时间: %s", alert.Timestamp),
	}

	data, ok := alert.Data.(map[string]interface{})
	if ok {
		if status, exists := data["status"]; exists {
			lines = append(lines, fmt.Sprintf("状态: %v", status))
		}
		if ratio, exists := data["error_ratio"]; exists {
			lines = append(lines, fmt.Sprintf("错误率: %s", formatWebhookPercent(ratio)))
		}
		if threshold, exists := data["threshold"]; exists {
			lines = append(lines, fmt.Sprintf("阈值: %s", formatWebhookPercent(threshold)))
		}
		if points, exists := data["consecutive_points"]; exists {
			lines = append(lines, fmt.Sprintf("连续点数: %v", points))
		}
	}

	return strings.Join(lines, "\n")
}

func formatWebhookPercent(value interface{}) string {
	switch v := value.(type) {
	case nil:
		return "未知"
	case float64:
		return fmt.Sprintf("%.2f%%", v*100)
	case float32:
		return fmt.Sprintf("%.2f%%", float64(v)*100)
	case int:
		return fmt.Sprintf("%.2f%%", float64(v)*100)
	case int64:
		return fmt.Sprintf("%.2f%%", float64(v)*100)
	case json.Number:
		if f, err := v.Float64(); err == nil {
			return fmt.Sprintf("%.2f%%", f*100)
		}
	}
	return fmt.Sprintf("%v", value)
}

func triggerAlerts(alert AlertItem) {
	alert.Timestamp = time.Now().UTC().Format(time.RFC3339)

	webhookMu.RLock()
	webhooksCopy := make([]Webhook, len(webhooks))
	copy(webhooksCopy, webhooks)
	webhookMu.RUnlock()

	// 记录告警
	statusMu.Lock()
	alert.WebhooksNotified = len(webhooksCopy)
	alerts = append(alerts, alert)
	pruneAlertsLocked()
	statusMu.Unlock()

	// 发送到所有 Webhook
	var success, failed int
	for i := range webhooksCopy {
		if err := sendWebhook(&webhooksCopy[i], alert); err != nil {
			failed++
			log.Printf("[Webhook] Failed to send to %s: %v", webhooksCopy[i].Name, err)
		} else {
			success++
			// 更新触发统计
			webhookMu.Lock()
			for j := range webhooks {
				if webhooks[j].ID == webhooksCopy[i].ID {
					webhooks[j].LastTriggered = time.Now().UTC().Format(time.RFC3339)
					webhooks[j].TriggerCount++
					break
				}
			}
			webhookMu.Unlock()
		}
	}

	saveState()
	log.Printf("[Alert] Sent to %d/%d webhooks (%d failed)", success, len(webhooksCopy), failed)
}

// ============ HTTP 处理器 ============

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", 405)
		return
	}

	statusMu.RLock()
	defer statusMu.RUnlock()
	alertThreshold, alertConsecutivePoints := getAlertConfig()

	writeJSON(w, map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{
			"current": currentStatus,
			"config": map[string]interface{}{
				"alertThreshold":         alertThreshold,
				"alertConsecutivePoints": alertConsecutivePoints,
				"checkInterval":          config.CheckInterval,
			},
		},
	})
}

func handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		alertThreshold, alertConsecutivePoints := getAlertConfig()
		writeJSON(w, map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"alertThreshold":         alertThreshold,
				"alertConsecutivePoints": alertConsecutivePoints,
				"configFile":             config.ConfigFile,
			},
		})

	case "PUT", "POST":
		var req struct {
			AlertThreshold         *float64 `json:"alertThreshold"`
			AlertConsecutivePoints *int     `json:"alertConsecutivePoints"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]interface{}{"code": 1, "message": "Invalid JSON"})
			return
		}

		oldThreshold, oldPoints := getAlertConfig()
		newThreshold, newPoints := oldThreshold, oldPoints
		if req.AlertThreshold != nil {
			newThreshold = *req.AlertThreshold
		}
		if req.AlertConsecutivePoints != nil {
			newPoints = *req.AlertConsecutivePoints
		}

		if err := validateAlertConfig(newThreshold, newPoints); err != nil {
			writeJSON(w, map[string]interface{}{"code": 1, "message": err.Error()})
			return
		}

		setAlertConfig(newThreshold, newPoints)
		if err := saveConfigFile(); err != nil {
			setAlertConfig(oldThreshold, oldPoints)
			writeJSON(w, map[string]interface{}{"code": 1, "message": fmt.Sprintf("Failed to save config: %v", err)})
			return
		}

		writeJSON(w, map[string]interface{}{
			"code":    0,
			"message": "Config updated",
			"data": map[string]interface{}{
				"alertThreshold":         newThreshold,
				"alertConsecutivePoints": newPoints,
				"configFile":             config.ConfigFile,
			},
		})

	default:
		http.Error(w, "Method not allowed", 405)
	}
}

func handleStatusHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", 405)
		return
	}

	limit := 24 // 默认 2 小时（每 5 分钟一条，24 条）
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 {
		limit = l
	}
	if limit > 288 {
		limit = 288 // 最多 24 小时
	}

	statusMu.RLock()
	defer statusMu.RUnlock()

	start := 0
	if len(statusHistory) > limit {
		start = len(statusHistory) - limit
	}

	writeJSON(w, map[string]interface{}{
		"code": 0,
		"data": statusHistory[start:],
	})
}

func handleAlerts(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", 405)
		return
	}

	limit := 20
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 {
		limit = l
	}
	if limit > 100 {
		limit = 100
	}

	statusMu.RLock()
	defer statusMu.RUnlock()

	start := 0
	if len(alerts) > limit {
		start = len(alerts) - limit
	}

	writeJSON(w, map[string]interface{}{
		"code": 0,
		"data": alerts[start:],
	})
}

func handleWebhooks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		webhookMu.RLock()
		defer webhookMu.RUnlock()

		// 隐藏敏感信息
		result := make([]map[string]interface{}, len(webhooks))
		for i, wh := range webhooks {
			result[i] = map[string]interface{}{
				"id":            wh.ID,
				"name":          wh.Name,
				"url":           wh.URL,
				"type":          normalizeWebhookType(wh.Type, wh.URL),
				"createdAt":     wh.CreatedAt,
				"lastTriggered": wh.LastTriggered,
				"triggerCount":  wh.TriggerCount,
			}
		}

		writeJSON(w, map[string]interface{}{
			"code": 0,
			"data": result,
		})

	case "POST":
		var req struct {
			URL    string `json:"url"`
			Name   string `json:"name"`
			Secret string `json:"secret"`
			Type   string `json:"type"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, map[string]interface{}{"code": 1, "message": "Invalid JSON"})
			return
		}

		if req.URL == "" {
			writeJSON(w, map[string]interface{}{"code": 1, "message": "url is required"})
			return
		}

		webhookMu.Lock()

		// 检查重复
		for _, wh := range webhooks {
			if wh.URL == req.URL {
				webhookMu.Unlock()
				writeJSON(w, map[string]interface{}{"code": 2, "message": "Webhook already registered"})
				return
			}
		}

		name := req.Name
		if name == "" {
			name = req.URL
		}

		webhook := Webhook{
			ID:        fmt.Sprintf("%d%s", time.Now().UnixNano(), randString(6)),
			URL:       req.URL,
			Secret:    req.Secret,
			Name:      name,
			Type:      normalizeWebhookType(req.Type, req.URL),
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		}

		webhooks = append(webhooks, webhook)
		webhookMu.Unlock()
		log.Printf("[Webhook] Registered: %s (%s)", webhook.Name, webhook.ID)

		saveState()

		writeJSON(w, map[string]interface{}{
			"code":    0,
			"message": "Webhook registered successfully",
			"data":    map[string]string{"id": webhook.ID, "name": webhook.Name},
		})

	default:
		http.Error(w, "Method not allowed", 405)
	}
}

func handleWebhookByID(w http.ResponseWriter, r *http.Request) {
	// 解析 ID
	path := r.URL.Path
	if len(path) <= len("/api/webhooks/") {
		http.Error(w, "Not found", 404)
		return
	}

	parts := path[len("/api/webhooks/"):]
	id := parts
	isTest := false

	// 检查是否是测试请求
	if len(parts) > 5 && parts[len(parts)-5:] == "/test" {
		id = parts[:len(parts)-5]
		isTest = true
	}

	switch {
	case r.Method == "DELETE" && !isTest:
		webhookMu.Lock()

		for i, wh := range webhooks {
			if wh.ID == id {
				webhooks = append(webhooks[:i], webhooks[i+1:]...)
				webhookMu.Unlock()
				log.Printf("[Webhook] Removed: %s", wh.Name)
				saveState()
				writeJSON(w, map[string]interface{}{"code": 0, "message": "Webhook removed"})
				return
			}
		}
		webhookMu.Unlock()
		writeJSON(w, map[string]interface{}{"code": 1, "message": "Webhook not found"})

	case r.Method == "POST" && isTest:
		webhookMu.RLock()
		var webhook Webhook
		found := false
		for i := range webhooks {
			if webhooks[i].ID == id {
				webhook = webhooks[i]
				found = true
				break
			}
		}
		webhookMu.RUnlock()

		if !found {
			writeJSON(w, map[string]interface{}{"code": 1, "message": "Webhook not found"})
			return
		}

		alertThreshold, alertConsecutivePoints := getAlertConfig()
		testPayload := AlertItem{
			Type:      "test",
			Message:   "这是一条测试告警",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Data: map[string]interface{}{
				"error_ratio":        0.1,
				"threshold":          alertThreshold,
				"consecutive_points": alertConsecutivePoints,
				"status":             "degraded",
			},
		}

		if err := sendWebhook(&webhook, testPayload); err != nil {
			writeJSON(w, map[string]interface{}{"code": 1, "message": fmt.Sprintf("Failed to send: %v", err)})
			return
		}

		// 更新统计
		webhookMu.Lock()
		for i := range webhooks {
			if webhooks[i].ID == id {
				webhooks[i].LastTriggered = time.Now().UTC().Format(time.RFC3339)
				webhooks[i].TriggerCount++
				break
			}
		}
		webhookMu.Unlock()
		saveState()

		writeJSON(w, map[string]interface{}{"code": 0, "message": "Test alert sent successfully"})

	default:
		http.Error(w, "Method not allowed", 405)
	}
}

func randString(n int) string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = chars[time.Now().UnixNano()%int64(len(chars))]
		time.Sleep(time.Nanosecond)
	}
	return string(b)
}

// ============ 静态页面 ============

func serveIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(indexHTML))
}

func serveAPIDocs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(apiDocsHTML))
}
