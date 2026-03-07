package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSONBody_ValidSingleObject(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/test", strings.NewReader(`{"a":1}`))
	w := httptest.NewRecorder()

	var dst struct {
		A int `json:"a"`
	}
	if ok := decodeJSONBody(w, req, &dst); !ok {
		t.Fatal("expected decodeJSONBody to accept valid JSON object")
	}
	if dst.A != 1 {
		t.Fatalf("unexpected decoded value: %d", dst.A)
	}
}

func TestDecodeJSONBody_RejectsTrailingJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/test", strings.NewReader(`{"a":1} {"b":2}`))
	w := httptest.NewRecorder()

	var dst struct {
		A int `json:"a"`
	}
	if ok := decodeJSONBody(w, req, &dst); ok {
		t.Fatal("expected decodeJSONBody to reject trailing JSON data")
	}
	if !strings.Contains(w.Body.String(), "请求体格式错误") {
		t.Fatalf("unexpected response body: %s", w.Body.String())
	}
}

func TestDecodeJSONBody_RejectsUnknownFields(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/test", strings.NewReader(`{"a":1,"extra":2}`))
	w := httptest.NewRecorder()

	var dst struct {
		A int `json:"a"`
	}
	if ok := decodeJSONBody(w, req, &dst); ok {
		t.Fatal("expected decodeJSONBody to reject unknown fields")
	}
	if !strings.Contains(w.Body.String(), "请求参数无效") {
		t.Fatalf("unexpected response body: %s", w.Body.String())
	}
}

func TestHandleGetLogs_ReturnsTaskState(t *testing.T) {
	s := NewServer()
	s.logs = []string{"line1", "line2"}
	s.setTaskState(false, "failed", "任务失败")

	req := httptest.NewRequest(http.MethodPost, "/api/logs", strings.NewReader(`{}`))
	w := httptest.NewRecorder()

	s.handleGetLogs(w, req)

	var resp struct {
		Logs        []string `json:"logs"`
		Running     bool     `json:"running"`
		TaskStatus  string   `json:"task_status"`
		TaskMessage string   `json:"task_message"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Running {
		t.Fatal("expected running=false")
	}
	if resp.TaskStatus != "failed" {
		t.Fatalf("unexpected task status: %q", resp.TaskStatus)
	}
	if resp.TaskMessage != "任务失败" {
		t.Fatalf("unexpected task message: %q", resp.TaskMessage)
	}
	if len(resp.Logs) != 2 {
		t.Fatalf("unexpected logs count: %d", len(resp.Logs))
	}
}
