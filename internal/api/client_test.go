package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestBuildRecord_UnescapeWithoutCorruptingValue(t *testing.T) {
	publishAt := time.Unix(1700000000, 0)
	contentURL := "https://mp.weixin.qq.com/s?__biz=biz&mid=1&idx=2&sn=abc&key=amp;v1&amp;foo=bar#wechat_redirect"

	rec := buildRecord("title", contentURL, "", publishAt)
	if rec == nil {
		t.Fatal("expected buildRecord to return a record")
	}
	if strings.Contains(rec.DirectURL, "#wechat_redirect") {
		t.Fatalf("redirect fragment should be removed: %s", rec.DirectURL)
	}
	if !strings.Contains(rec.DirectURL, "key=amp;v1") {
		t.Fatalf("key value should be preserved: %s", rec.DirectURL)
	}
	if !strings.Contains(rec.DirectURL, "&foo=bar") {
		t.Fatalf("html-escaped separator should be unescaped: %s", rec.DirectURL)
	}
}

func TestReadRawInt(t *testing.T) {
	if got := readRawInt(json.RawMessage(`123`)); got != 123 {
		t.Fatalf("unexpected int parse: %d", got)
	}
	if got := readRawInt(json.RawMessage(`12.9`)); got != 12 {
		t.Fatalf("unexpected float-to-int parse: %d", got)
	}
	if got := readRawInt(json.RawMessage(`"bad"`)); got != 0 {
		t.Fatalf("unexpected invalid parse: %d", got)
	}
}

func TestReadRawString(t *testing.T) {
	if got := readRawString(json.RawMessage(`"  abc  "`)); got != "abc" {
		t.Fatalf("unexpected string parse: %q", got)
	}
	if got := readRawString(json.RawMessage(`null`)); got != "" {
		t.Fatalf("unexpected null parse result: %q", got)
	}
}
