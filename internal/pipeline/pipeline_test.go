package pipeline

import (
	"testing"
	"time"

	"wesaver/internal/models"
	"wesaver/internal/repo"
)

// --- Pure function tests ---

func TestShouldCollectDetail_BoundaryValues(t *testing.T) {
	id := "biz_mid_idx_sn"

	if !shouldCollectDetail(id, 1.0) {
		t.Fatal("rate=1.0 should always collect")
	}
	if shouldCollectDetail(id, 0.0) {
		t.Fatal("rate=0.0 should never collect")
	}
	if !shouldCollectDetail(id, 1.1) {
		t.Fatal("rate>1 should still collect (clamped)")
	}
	if shouldCollectDetail(id, -0.5) {
		t.Fatal("rate<0 should not collect")
	}
}

func TestStableHash_Deterministic(t *testing.T) {
	id := "biz123_mid456_1_snABC"
	h1 := stableHash(id)
	h2 := stableHash(id)
	if h1 != h2 {
		t.Fatalf("stableHash should be deterministic: %d != %d", h1, h2)
	}
	if h1 == 0 {
		t.Fatal("stableHash should not be zero for non-empty input")
	}
}

func TestBuildErrorRow_Fields(t *testing.T) {
	pt := time.Unix(1700000000, 0)
	article := &models.ArticleRecord{
		Biz:       "biz",
		Mid:       "mid",
		Idx:       "1",
		Sn:        "sn",
		Title:     "测试文章",
		DirectURL: "https://example.com/article",
	}
	article.PublishTime = &pt

	row := buildErrorRow(article, "保存 HTML 失败: disk full")
	if row["article_id"] != "biz_mid_1_sn" {
		t.Fatalf("unexpected article_id: %v", row["article_id"])
	}
	if row["title"] != "测试文章" {
		t.Fatalf("unexpected title: %v", row["title"])
	}
	if row["error_message"] != "保存 HTML 失败: disk full" {
		t.Fatalf("unexpected error_message: %v", row["error_message"])
	}
}

func TestBuildSkippedDetailRow_HasAllFields(t *testing.T) {
	pt := time.Unix(1700000000, 0)
	article := &models.ArticleRecord{
		Biz:         "biz",
		Mid:         "mid",
		Idx:         "1",
		Sn:          "sn",
		Title:       "标题",
		PublishTime: &pt,
		DirectURL:   "https://example.com/a",
		SourceURL:   "https://example.com/b",
	}

	row := buildSkippedDetailRow(article)
	if row["article_id"] != "biz_mid_1_sn" {
		t.Fatalf("unexpected article_id: %v", row["article_id"])
	}
	if row["detail_sampled"] != 0 {
		t.Fatalf("skipped row should have detail_sampled=0")
	}
	if row["comments"] != "[]" {
		t.Fatalf("skipped row should have empty comments")
	}
}

// --- Resume filtering integration tests ---

func TestResumeFiltering_FailedArticlesAreRetried(t *testing.T) {
	dir := t.TempDir()
	repository, err := repo.NewTaskRepository(dir)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}
	defer repository.Close()

	pt := time.Unix(1700000000, 0)
	allArticles := []models.ArticleRecord{
		{Biz: "b", Mid: "1", Idx: "1", Sn: "a", Title: "成功文章1", PublishTime: &pt, DirectURL: "u1"},
		{Biz: "b", Mid: "2", Idx: "1", Sn: "b", Title: "失败文章1", PublishTime: &pt, DirectURL: "u2"},
		{Biz: "b", Mid: "3", Idx: "1", Sn: "c", Title: "成功文章2", PublishTime: &pt, DirectURL: "u3"},
		{Biz: "b", Mid: "4", Idx: "1", Sn: "d", Title: "失败文章2", PublishTime: &pt, DirectURL: "u4"},
		{Biz: "b", Mid: "5", Idx: "1", Sn: "e", Title: "新文章", PublishTime: &pt, DirectURL: "u5"},
	}

	// Simulate first run: 2 succeeded, 2 failed, 1 never processed
	repository.MarkStatus(allArticles[0].ArticleID(), "completed", "")
	repository.MarkStatus(allArticles[1].ArticleID(), "failed", "保存 HTML 失败: disk full")
	repository.MarkStatus(allArticles[2].ArticleID(), "completed", "")
	repository.MarkStatus(allArticles[3].ArticleID(), "failed", "download timeout")
	// allArticles[4] was never processed (e.g. task was cancelled before reaching it)

	// Simulate resume run: Phase 3 filtering (same logic as pipeline.Run)
	completedIDs := repository.GetCompletedIDs()

	var pendingArticles []models.ArticleRecord
	for _, a := range allArticles {
		if !completedIDs[a.ArticleID()] {
			pendingArticles = append(pendingArticles, a)
		}
	}
	skipped := len(allArticles) - len(pendingArticles)

	// Assertions
	if skipped != 2 {
		t.Fatalf("expected 2 skipped (completed), got %d", skipped)
	}
	if len(pendingArticles) != 3 {
		t.Fatalf("expected 3 pending (2 failed + 1 new), got %d", len(pendingArticles))
	}

	pendingIDs := make(map[string]bool)
	for _, a := range pendingArticles {
		pendingIDs[a.ArticleID()] = true
	}

	if !pendingIDs[allArticles[1].ArticleID()] {
		t.Fatal("failed article '失败文章1' should be retried on resume")
	}
	if !pendingIDs[allArticles[3].ArticleID()] {
		t.Fatal("failed article '失败文章2' should be retried on resume")
	}
	if !pendingIDs[allArticles[4].ArticleID()] {
		t.Fatal("unprocessed article '新文章' should be pending on resume")
	}

	if pendingIDs[allArticles[0].ArticleID()] {
		t.Fatal("completed article '成功文章1' should be skipped")
	}
	if pendingIDs[allArticles[2].ArticleID()] {
		t.Fatal("completed article '成功文章2' should be skipped")
	}
}

func TestResumeFiltering_OverwriteIgnoresCompletedStatus(t *testing.T) {
	dir := t.TempDir()
	repository, err := repo.NewTaskRepository(dir)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}
	defer repository.Close()

	pt := time.Unix(1700000000, 0)
	allArticles := []models.ArticleRecord{
		{Biz: "b", Mid: "1", Idx: "1", Sn: "a", Title: "A", PublishTime: &pt, DirectURL: "u1"},
		{Biz: "b", Mid: "2", Idx: "1", Sn: "b", Title: "B", PublishTime: &pt, DirectURL: "u2"},
	}

	repository.MarkStatus(allArticles[0].ArticleID(), "completed", "")
	repository.MarkStatus(allArticles[1].ArticleID(), "completed", "")

	// Simulate overwrite mode (OverwriteExisting = true): skip GetCompletedIDs
	overwriteExisting := true
	completedIDs := make(map[string]bool)
	if !overwriteExisting {
		completedIDs = repository.GetCompletedIDs()
	}

	var pendingArticles []models.ArticleRecord
	for _, a := range allArticles {
		if !completedIDs[a.ArticleID()] {
			pendingArticles = append(pendingArticles, a)
		}
	}

	if len(pendingArticles) != 2 {
		t.Fatalf("overwrite mode should re-process all articles, got %d pending", len(pendingArticles))
	}
}

func TestResumeFiltering_MultipleFailureCyclesConverge(t *testing.T) {
	dir := t.TempDir()
	repository, err := repo.NewTaskRepository(dir)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}
	defer repository.Close()

	pt := time.Unix(1700000000, 0)
	articles := []models.ArticleRecord{
		{Biz: "b", Mid: "1", Idx: "1", Sn: "a", Title: "A", PublishTime: &pt, DirectURL: "u1"},
		{Biz: "b", Mid: "2", Idx: "1", Sn: "b", Title: "B", PublishTime: &pt, DirectURL: "u2"},
		{Biz: "b", Mid: "3", Idx: "1", Sn: "c", Title: "C", PublishTime: &pt, DirectURL: "u3"},
	}

	// Cycle 1: all fail
	for _, a := range articles {
		repository.MarkStatus(a.ArticleID(), "failed", "network error")
	}
	ids := repository.GetCompletedIDs()
	if len(ids) != 0 {
		t.Fatalf("cycle 1: expected 0 completed, got %d", len(ids))
	}

	// Cycle 2: first two succeed, third still fails
	repository.MarkStatus(articles[0].ArticleID(), "completed", "")
	repository.MarkStatus(articles[1].ArticleID(), "completed", "")
	repository.MarkStatus(articles[2].ArticleID(), "failed", "timeout again")

	ids = repository.GetCompletedIDs()
	if len(ids) != 2 {
		t.Fatalf("cycle 2: expected 2 completed, got %d", len(ids))
	}

	var pending []models.ArticleRecord
	for _, a := range articles {
		if !ids[a.ArticleID()] {
			pending = append(pending, a)
		}
	}
	if len(pending) != 1 {
		t.Fatalf("cycle 2: expected 1 pending, got %d", len(pending))
	}
	if pending[0].ArticleID() != articles[2].ArticleID() {
		t.Fatalf("cycle 2: wrong pending article: %s", pending[0].ArticleID())
	}

	// Cycle 3: last one finally succeeds
	repository.MarkStatus(articles[2].ArticleID(), "completed", "")
	ids = repository.GetCompletedIDs()
	if len(ids) != 3 {
		t.Fatalf("cycle 3: expected 3 completed, got %d", len(ids))
	}
}
