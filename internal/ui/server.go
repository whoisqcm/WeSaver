package ui

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"wesaver/internal/export"
	"wesaver/internal/models"
	"wesaver/internal/pipeline"
	"wesaver/internal/proxy"
)

//go:embed static/*
var staticFiles embed.FS

type Server struct {
	mu            sync.Mutex
	proxyCapture  *proxy.Capture
	proxySnapshot *proxy.WinInetSnapshot
	runCancel     context.CancelFunc
	running       bool
	logs          []string
	logMu         sync.Mutex
	httpServer    *http.Server
	httpListener  net.Listener
}

func NewServer() *Server {
	return &Server{}
}

func requestOrigin(r *http.Request) string {
	if origin := r.Header.Get("Origin"); origin != "" {
		return origin
	}
	if ref := r.Referer(); ref != "" {
		if u, err := url.Parse(ref); err == nil {
			return u.Scheme + "://" + u.Host
		}
	}
	return ""
}

func isSameOrigin(r *http.Request) bool {
	origin := requestOrigin(r)
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}

func ensureMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method != method {
		writeJSON(w, map[string]interface{}{"ok": false, "message": "方法不允许"})
		return false
	}
	return true
}

func ensureSameOrigin(w http.ResponseWriter, r *http.Request) bool {
	if !isSameOrigin(r) {
		writeJSON(w, map[string]interface{}{"ok": false, "message": "跨站请求被拒绝"})
		return false
	}
	return true
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst interface{}) bool {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeJSON(w, map[string]interface{}{"ok": false, "message": "请求参数无效"})
		return false
	}
	if dec.More() {
		writeJSON(w, map[string]interface{}{"ok": false, "message": "请求体格式错误"})
		return false
	}
	return true
}

// StartInBackground starts the HTTP server on a random port and returns the URL.
// The server runs in a background goroutine.
func (s *Server) StartInBackground() (string, error) {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" {
			path = "/static/index.html"
		} else {
			path = "/static" + path
		}
		data, err := staticFiles.ReadFile(strings.TrimPrefix(path, "/"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if strings.HasSuffix(path, ".html") {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		} else if strings.HasSuffix(path, ".css") {
			w.Header().Set("Content-Type", "text/css")
		} else if strings.HasSuffix(path, ".js") {
			w.Header().Set("Content-Type", "application/javascript")
		}
		w.Write(data)
	})

	mux.HandleFunc("/api/validate-token", s.handleValidateToken)
	mux.HandleFunc("/api/start-capture", s.handleStartCapture)
	mux.HandleFunc("/api/stop-capture", s.handleStopCapture)
	mux.HandleFunc("/api/capture-status", s.handleCaptureStatus)
	mux.HandleFunc("/api/run-task", s.handleRunTask)
	mux.HandleFunc("/api/cancel-task", s.handleCancelTask)
	mux.HandleFunc("/api/logs", s.handleGetLogs)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}

	port := ln.Addr().(*net.TCPAddr).Port
	url := fmt.Sprintf("http://127.0.0.1:%d", port)

	httpSrv := &http.Server{Handler: mux}

	s.mu.Lock()
	s.httpServer = httpSrv
	s.httpListener = ln
	s.mu.Unlock()

	go httpSrv.Serve(ln)

	return url, nil
}

// Shutdown performs graceful cleanup: cancels running tasks, stops proxy,
// and restores system proxy. CA certificate is kept in the trust store
// so that the next launch does not require another confirmation prompt.
func (s *Server) Shutdown() {
	s.mu.Lock()
	cancel := s.runCancel
	capture := s.proxyCapture
	snapshot := s.proxySnapshot
	httpSrv := s.httpServer
	listener := s.httpListener
	s.runCancel = nil
	s.proxyCapture = nil
	s.proxySnapshot = nil
	s.httpServer = nil
	s.httpListener = nil
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if capture != nil {
		capture.Stop()
	}
	if snapshot != nil {
		proxy.RestoreSystemProxy(snapshot)
		proxy.NotifyProxyChanged()
	}
	if httpSrv != nil {
		ctx, cancelShutdown := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancelShutdown()
		_ = httpSrv.Shutdown(ctx)
	}
	if listener != nil {
		_ = listener.Close()
	}
}

// OpenBrowser opens the given URL in the default browser (fallback when WebView2 is unavailable).
func OpenBrowser(url string) {
	time.Sleep(300 * time.Millisecond)
	switch runtime.GOOS {
	case "windows":
		exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		exec.Command("open", url).Start()
	default:
		exec.Command("xdg-open", url).Start()
	}
}

// caDataDir returns the directory where CA cert/key are persisted.
func caDataDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "wesaver_data"
	}
	return filepath.Join(filepath.Dir(exe), "wesaver_data")
}

func (s *Server) appendLog(msg string) {
	s.logMu.Lock()
	defer s.logMu.Unlock()
	line := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), msg)
	s.logs = append(s.logs, line)
	if len(s.logs) > 2000 {
		s.logs = s.logs[len(s.logs)-1000:]
	}
}

func (s *Server) handleValidateToken(w http.ResponseWriter, r *http.Request) {
	if !ensureMethod(w, r, http.MethodPost) || !ensureSameOrigin(w, r) {
		return
	}

	var req struct {
		TokenURL string `json:"token_url"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}

	valid, message, count, taskName := validateTokenImpl(req.TokenURL)
	if valid {
		s.appendLog(fmt.Sprintf("token 校验通过：首屏返回 %d 篇", count))
		writeJSON(w, map[string]interface{}{
			"valid":     true,
			"message":   fmt.Sprintf("有效（首屏 %d 篇）", count),
			"count":     count,
			"task_name": export.SanitizePathSegment(taskName),
		})
	} else {
		writeJSON(w, map[string]interface{}{"valid": false, "message": message})
	}
}

func (s *Server) handleStartCapture(w http.ResponseWriter, r *http.Request) {
	if !ensureMethod(w, r, http.MethodPost) || !ensureSameOrigin(w, r) {
		return
	}

	s.mu.Lock()
	if s.proxyCapture != nil {
		s.mu.Unlock()
		writeJSON(w, map[string]interface{}{"ok": false, "message": "代理已在运行"})
		return
	}
	s.mu.Unlock()

	// Generate or load CA certificate (I/O — outside lock)
	ca, err := proxy.NewCA(caDataDir())
	if err != nil {
		writeJSON(w, map[string]interface{}{"ok": false, "message": "生成 CA 证书失败: " + err.Error()})
		return
	}

	// Install CA to Windows trusted root store (Windows API — outside lock)
	if err := ca.InstallToStore(); err != nil {
		s.appendLog("安装 CA 证书失败: " + err.Error())
		writeJSON(w, map[string]interface{}{"ok": false, "message": "安装 CA 证书失败: " + err.Error()})
		return
	}
	s.appendLog("CA 证书已安装到受信任的根证书存储")

	// Capture current proxy settings before overriding (registry I/O — outside lock)
	snapshot, _ := proxy.CaptureSystemProxy()

	capture := proxy.NewCapture(ca)
	if err := capture.Start(8899); err != nil {
		writeJSON(w, map[string]interface{}{"ok": false, "message": "启动代理失败: " + err.Error()})
		return
	}

	// Now acquire the lock to update shared state
	s.mu.Lock()
	// Re-check in case another request came in concurrently
	if s.proxyCapture != nil {
		s.mu.Unlock()
		capture.Stop()
		writeJSON(w, map[string]interface{}{"ok": false, "message": "代理已在运行"})
		return
	}
	s.proxyCapture = capture
	s.proxySnapshot = snapshot
	s.mu.Unlock()

	proxyAddr := fmt.Sprintf("127.0.0.1:%d", 8899)
	if err := proxy.ApplySystemProxy(proxyAddr, "<local>"); err != nil {
		s.appendLog("设置系统代理失败: " + err.Error())
	}
	proxy.NotifyProxyChanged()

	s.appendLog(fmt.Sprintf("代理已启动：%s", proxyAddr))
	writeJSON(w, map[string]interface{}{"ok": true, "message": "代理已启动", "port": 8899})
}

func (s *Server) handleStopCapture(w http.ResponseWriter, r *http.Request) {
	if !ensureMethod(w, r, http.MethodPost) || !ensureSameOrigin(w, r) {
		return
	}

	s.mu.Lock()
	capture := s.proxyCapture
	snapshot := s.proxySnapshot
	s.proxyCapture = nil
	s.proxySnapshot = nil
	s.mu.Unlock()

	if capture != nil {
		capture.Stop()
		s.appendLog("代理已停止")
	}

	if snapshot != nil {
		proxy.RestoreSystemProxy(snapshot)
		proxy.NotifyProxyChanged()
	}

	s.appendLog("代理已停止，系统代理已恢复。")
	writeJSON(w, map[string]interface{}{"ok": true})
}

func (s *Server) handleCaptureStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := map[string]interface{}{
		"running":   false,
		"token_url": "",
	}

	if s.proxyCapture != nil {
		stats := s.proxyCapture.GetStats()
		result["running"] = true
		result["stats"] = stats

		if url, ok := s.proxyCapture.TryGetCapturedToken(); ok {
			result["token_url"] = url
			s.appendLog("已捕获 token 链接！")
		}
	}

	writeJSON(w, result)
}

func (s *Server) handleRunTask(w http.ResponseWriter, r *http.Request) {
	if !ensureMethod(w, r, http.MethodPost) || !ensureSameOrigin(w, r) {
		return
	}

	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		writeJSON(w, map[string]interface{}{"ok": false, "message": "任务已在运行中"})
		return
	}
	s.running = true
	s.mu.Unlock()

	var req struct {
		TokenURL      string  `json:"token_url"`
		TaskName      string  `json:"task_name"`
		Pages         int     `json:"pages"`
		MaxArticles   int     `json:"max_articles"`
		OutputRoot    string  `json:"output_root"`
		SpeedIndex    int     `json:"speed_index"`
		Concurrency   int     `json:"concurrency"`
		ExportHTML    bool    `json:"export_html"`
		ExportMD      bool    `json:"export_markdown"`
		ExportExcel   bool    `json:"export_excel"`
		FetchComments bool    `json:"fetch_comments"`
		SampleRate    float64 `json:"sample_rate"`
		Resume        bool    `json:"resume"`
		Overwrite     bool    `json:"overwrite"`
	}
	if !decodeJSONBody(w, r, &req) {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
		return
	}

	token, ok := models.ParseTokenLink(req.TokenURL)
	if !ok {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
		writeJSON(w, map[string]interface{}{"ok": false, "message": "token 无效"})
		return
	}

	profiles := models.SpeedProfiles()
	speedIdx := req.SpeedIndex
	if speedIdx < 0 || speedIdx >= len(profiles) {
		speedIdx = 0
	}
	speed := profiles[speedIdx]

	opts := models.TaskOptions{
		OutputRoot:         req.OutputRoot,
		ExportHTML:         req.ExportHTML,
		ExportMarkdown:     req.ExportMD,
		ExportExcelDetails: req.ExportExcel,
		MaxRetries:         2,
		HTTPTimeout:        25 * time.Second,
		MaxConcurrency:     max(1, req.Concurrency),
		ListPageDelayMs:    speed.ListPageDelayMs,
		ArticleDelayMinMs:  speed.ArticleDelayMinMs,
		ArticleDelayMaxMs:  speed.ArticleDelayMaxMs,
		FetchComments:      req.FetchComments,
		DetailSampleRate:   req.SampleRate,
		ExportQueueCap:     1024,
		ExportWorkers:      2,
		MaxArticles:        req.MaxArticles,
		ResumeByDefault:    req.Resume,
		OverwriteExisting:  req.Overwrite,
	}

	if opts.OutputRoot == "" {
		opts.OutputRoot = "output"
	}

	taskName := req.TaskName
	if taskName == "" {
		taskName = export.SanitizePathSegment("mp_" + token.Biz)
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.runCancel = cancel
	s.mu.Unlock()

	s.appendLog(fmt.Sprintf("任务开始: %s (speed: %s, concurrency: %d)", taskName, speed.Name, opts.MaxConcurrency))

	go func() {
		defer func() {
			s.mu.Lock()
			s.running = false
			s.runCancel = nil
			s.mu.Unlock()
		}()

		p := pipeline.New(opts, func(msg string) {
			s.appendLog(msg)
		})

		result, err := p.Run(ctx, taskName, token, req.Pages)
		if err != nil {
			s.appendLog("任务失败: " + err.Error())
			return
		}

		s.appendLog(fmt.Sprintf("完成统计 => 总数: %d, 完成: %d, 跳过: %d, 失败: %d", result.Total, result.Completed, result.Skipped, result.Failed))
		s.appendLog("结果目录: " + result.OutputRoot)

		if runtime.GOOS == "windows" {
			exec.Command("explorer", result.OutputRoot).Start()
		}
	}()

	writeJSON(w, map[string]interface{}{"ok": true, "message": "任务已启动"})
}

func (s *Server) handleCancelTask(w http.ResponseWriter, r *http.Request) {
	if !ensureMethod(w, r, http.MethodPost) || !ensureSameOrigin(w, r) {
		return
	}

	s.mu.Lock()
	cancel := s.runCancel
	s.mu.Unlock()

	if cancel != nil {
		cancel()
		s.appendLog("任务取消中...")
	}
	writeJSON(w, map[string]interface{}{"ok": true})
}

func (s *Server) handleGetLogs(w http.ResponseWriter, r *http.Request) {
	s.logMu.Lock()
	logs := make([]string, len(s.logs))
	copy(logs, s.logs)
	s.logMu.Unlock()

	running := false
	s.mu.Lock()
	running = s.running
	s.mu.Unlock()

	writeJSON(w, map[string]interface{}{
		"logs":    logs,
		"running": running,
	})
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
