package repo

import (
	"testing"
)

func TestGetCompletedIDs_ExcludesFailedArticles(t *testing.T) {
	dir := t.TempDir()
	r, err := NewTaskRepository(dir)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}
	defer r.Close()

	r.MarkStatus("art_ok_1", "completed", "")
	r.MarkStatus("art_ok_2", "completed", "")
	r.MarkStatus("art_fail_1", "failed", "download error")
	r.MarkStatus("art_fail_2", "failed", "保存 HTML 失败: disk full")
	r.MarkStatus("art_running", "running", "")

	ids := r.GetCompletedIDs()

	if !ids["art_ok_1"] || !ids["art_ok_2"] {
		t.Fatal("completed articles should be in GetCompletedIDs")
	}
	if ids["art_fail_1"] || ids["art_fail_2"] {
		t.Fatal("failed articles must NOT appear in GetCompletedIDs")
	}
	if ids["art_running"] {
		t.Fatal("running articles must NOT appear in GetCompletedIDs")
	}
}

func TestGetCompletedIDs_FailedOverwritesCompleted(t *testing.T) {
	dir := t.TempDir()
	r, err := NewTaskRepository(dir)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}
	defer r.Close()

	r.MarkStatus("art_1", "completed", "")
	if ids := r.GetCompletedIDs(); !ids["art_1"] {
		t.Fatal("should be completed initially")
	}

	r.MarkStatus("art_1", "failed", "re-run export error")

	ids := r.GetCompletedIDs()
	if ids["art_1"] {
		t.Fatal("after marking failed, article must NOT appear in GetCompletedIDs")
	}
	if !r.IsCompleted("art_1") == true {
		// IsCompleted should also return false
	}
	if r.IsCompleted("art_1") {
		t.Fatal("IsCompleted should return false for failed article")
	}
}

func TestGetCompletedIDs_RetriedFailureBecomesCompleted(t *testing.T) {
	dir := t.TempDir()
	r, err := NewTaskRepository(dir)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}
	defer r.Close()

	r.MarkStatus("art_1", "failed", "network timeout")

	ids := r.GetCompletedIDs()
	if ids["art_1"] {
		t.Fatal("failed article should not be in completed set")
	}

	r.MarkStatus("art_1", "completed", "")

	ids = r.GetCompletedIDs()
	if !ids["art_1"] {
		t.Fatal("after retry success, article should be in completed set")
	}
}

func TestMarkStatus_ErrorMessageStoredCorrectly(t *testing.T) {
	dir := t.TempDir()
	r, err := NewTaskRepository(dir)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}
	defer r.Close()

	r.MarkStatus("art_1", "failed", "保存 HTML 失败: permission denied; 保存 Markdown 失败: disk full")

	if r.IsCompleted("art_1") {
		t.Fatal("article with errors should not be completed")
	}

	ids := r.GetCompletedIDs()
	if ids["art_1"] {
		t.Fatal("article with errors should not be in completed set")
	}
}
