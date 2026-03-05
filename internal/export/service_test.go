package export

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestBuildArticleOutputPath_HandlesMultipleCollisions(t *testing.T) {
	dir := t.TempDir()
	publish := "2026-03-05 12:34:56"
	stem := buildArticleFileStem("标题", &publish)
	suffix := SanitizePathSegment("biz_mid_idx_sn")

	original := filepath.Join(dir, stem+".html")
	withID := filepath.Join(dir, stem+"_"+suffix+".html")
	if err := os.WriteFile(original, []byte("x"), 0o644); err != nil {
		t.Fatalf("write original: %v", err)
	}
	if err := os.WriteFile(withID, []byte("x"), 0o644); err != nil {
		t.Fatalf("write with id: %v", err)
	}

	got := buildArticleOutputPath(dir, "标题", &publish, "biz_mid_idx_sn", ".html")
	want := filepath.Join(dir, stem+"_"+suffix+"_2.html")
	if got != want {
		t.Fatalf("unexpected output path:\nwant: %s\ngot:  %s", want, got)
	}
}

func TestBuildArticleFileStem_RuneSafeTruncation(t *testing.T) {
	title := strings.Repeat("中", 220)
	stem := buildArticleFileStem(title, nil)

	if !utf8.ValidString(stem) {
		t.Fatalf("stem should remain valid UTF-8, got: %q", stem)
	}
	if utf8.RuneCountInString(stem) > 150 {
		t.Fatalf("stem rune count should be <= 150, got %d", utf8.RuneCountInString(stem))
	}
}
