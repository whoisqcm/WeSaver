package proxy

import (
	"strings"
	"testing"
)

func TestExtractHostHeader(t *testing.T) {
	raw := "GET /abc HTTP/1.1\r\nHost: mp.weixin.qq.com\r\nUser-Agent: test\r\n\r\n"
	if got := extractHostHeader(raw); got != "mp.weixin.qq.com" {
		t.Fatalf("unexpected host header: %q", got)
	}
}

func TestNormalizeConnectHost(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{in: "mp.weixin.qq.com:443", want: "mp.weixin.qq.com:443"},
		{in: "mp.weixin.qq.com", want: "mp.weixin.qq.com:443"},
		{in: "[::1]", want: "[::1]:443"},
	}
	for _, tc := range cases {
		if got := normalizeConnectHost(tc.in); got != tc.want {
			t.Fatalf("normalizeConnectHost(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestBuildHTTPSURL(t *testing.T) {
	if got := buildHTTPSURL("mp.weixin.qq.com", "/mp/profile_ext?a=1"); got != "https://mp.weixin.qq.com/mp/profile_ext?a=1" {
		t.Fatalf("unexpected url build: %q", got)
	}
	if got := buildHTTPSURL("mp.weixin.qq.com", "https://example.com/x"); got != "https://example.com/x" {
		t.Fatalf("absolute url should be preserved: %q", got)
	}
	if got := buildHTTPSURL("mp.weixin.qq.com", ""); got != "https://mp.weixin.qq.com/" {
		t.Fatalf("empty path should yield root url: %q", got)
	}
}

func TestWriteFullHandlesShortWrites(t *testing.T) {
	w := &limitedWriter{maxPerWrite: 2}
	payload := []byte("123456789")
	if err := writeFull(w, payload); err != nil {
		t.Fatalf("writeFull returned error: %v", err)
	}
	if got := string(w.buf); got != string(payload) {
		t.Fatalf("unexpected written payload: %q", got)
	}
}

type limitedWriter struct {
	maxPerWrite int
	buf         []byte
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	n := w.maxPerWrite
	if n <= 0 {
		n = 1
	}
	if n > len(p) {
		n = len(p)
	}
	w.buf = append(w.buf, p[:n]...)
	return n, nil
}

func TestExtractHostFromURL(t *testing.T) {
	if got := extractHostFromURL("https://mp.weixin.qq.com/mp/profile_ext"); got != "mp.weixin.qq.com" {
		t.Fatalf("unexpected host for absolute url: %q", got)
	}
	if got := extractHostFromURL("/relative/path"); got != "" {
		t.Fatalf("relative path should not produce host, got %q", got)
	}
	if got := extractHostFromURL("mp.weixin.qq.com/path"); !strings.HasPrefix(got, "mp.weixin.qq.com") {
		t.Fatalf("unexpected host for host/path input: %q", got)
	}
}
