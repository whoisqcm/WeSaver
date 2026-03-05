package ui

import (
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
