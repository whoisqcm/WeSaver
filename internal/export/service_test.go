package export

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
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

func TestSaveHTML_ConcurrentNameAllocationDoesNotOverwrite(t *testing.T) {
	paths := NewPaths(t.TempDir(), "测试公众号", time.Unix(1700000000, 0), "")
	if err := paths.EnsureFolders(); err != nil {
		t.Fatalf("ensure folders: %v", err)
	}

	svc := NewService()
	publish := "2026-03-06 09:08:07"

	const writers = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < writers; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			html := "<html><body>article-" + string(rune('A'+i)) + "</body></html>"
			if err := svc.SaveHTML(paths, "same_article_id", "同名文章", &publish, html); err != nil {
				t.Errorf("SaveHTML failed: %v", err)
			}
		}()
	}

	close(start)
	wg.Wait()

	entries, err := os.ReadDir(paths.HTMLDir)
	if err != nil {
		t.Fatalf("read html dir: %v", err)
	}
	if len(entries) != writers {
		t.Fatalf("expected %d files, got %d", writers, len(entries))
	}
}
