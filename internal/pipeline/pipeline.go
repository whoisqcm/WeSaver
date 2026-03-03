package pipeline

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"wesaver/internal/api"
	"wesaver/internal/export"
	"wesaver/internal/models"
	"wesaver/internal/repo"
)

type Logger func(msg string)

type Pipeline struct {
	opts models.TaskOptions
	log  Logger
}

func New(opts models.TaskOptions, log Logger) *Pipeline {
	return &Pipeline{opts: opts, log: log}
}

func (p *Pipeline) Run(ctx context.Context, taskName string, token *models.TokenLink, pages int) (*models.PipelineResult, error) {
	startedAt := time.Now()

	var resumeRoot string
	if p.opts.ResumeByDefault && !p.opts.OverwriteExisting {
		resumeRoot = tryResolveResumeRoot(p.opts.OutputRoot, taskName)
	}

	paths := export.NewPaths(p.opts.OutputRoot, taskName, startedAt, resumeRoot)
	if err := paths.EnsureFolders(); err != nil {
		return nil, fmt.Errorf("create folders: %w", err)
	}

	repository, err := repo.NewTaskRepository(paths.TaskRoot)
	if err != nil {
		return nil, fmt.Errorf("open task db: %w", err)
	}
	defer repository.Close()

	client := api.NewClient(p.opts)
	excelExp := export.NewExcelExporter()
	svc := export.NewService()

	if resumeRoot == "" {
		p.logf("输出目录: %s", paths.TaskRoot)
	} else {
		p.logf("输出目录: %s (默认断点续传)", paths.TaskRoot)
	}

	// Phase 1: Collect article list
	allArticles, _ := p.collectArticles(ctx, client, token, pages)
	p.logf("列表抓取完成: %d 篇(去重后)", len(allArticles))

	originalTotal := len(allArticles)
	if p.opts.MaxArticles > 0 && len(allArticles) > p.opts.MaxArticles {
		allArticles = allArticles[:p.opts.MaxArticles]
		p.logf("已按抓取数量限制: %d/%d 篇", len(allArticles), originalTotal)
	}

	// Phase 2: Save article list Excel
	listRows := make([]map[string]interface{}, 0, len(allArticles))
	for _, a := range allArticles {
		publishTime := ""
		if a.PublishTime != nil {
			publishTime = a.PublishTime.Format("2006-01-02 15:04:05")
		}
		listRows = append(listRows, map[string]interface{}{
			"article_id":   a.ArticleID(),
			"title":        a.Title,
			"publish_time": publishTime,
			"direct_url":   a.DirectURL,
			"source_url":   a.SourceURL,
			"cover_url":    a.CoverURL,
		})
	}

	existingIDs := excelExp.ReadColumnValues(paths.ArticleListXlsx(), "article_id")
	var newListRows []map[string]interface{}
	for _, row := range listRows {
		id, _ := row["article_id"].(string)
		if id != "" && !existingIDs[id] {
			newListRows = append(newListRows, row)
		}
	}

	if err := excelExp.AppendRows(paths.ArticleListXlsx(), newListRows); err != nil {
		p.logf("写入列表 Excel 失败: %v", err)
	}
	if len(newListRows) != len(listRows) {
		p.logf("列表入库去重: 新增 %d，跳过重复 %d", len(newListRows), len(listRows)-len(newListRows))
	}

	// Phase 3: Filter pending articles
	completedIDs := make(map[string]bool)
	if !p.opts.OverwriteExisting {
		completedIDs = repository.GetCompletedIDs()
	}

	var pendingArticles []models.ArticleRecord
	for _, a := range allArticles {
		if !completedIDs[a.ArticleID()] {
			pendingArticles = append(pendingArticles, a)
		}
	}
	skipped := len(allArticles) - len(pendingArticles)
	p.logf("待处理: %d，已完成跳过: %d", len(pendingArticles), skipped)

	// Phase 4: Process articles concurrently
	var (
		completed  atomic.Int32
		failed     atomic.Int32
		processed  atomic.Int32
		detailRows sync.Map
		errorRows  sync.Map
		detailIdx  atomic.Int32
		errorIdx   atomic.Int32
	)

	sem := make(chan struct{}, max(1, p.opts.MaxConcurrency))
	var wg sync.WaitGroup

	cancelledEarly := false
	for _, article := range pendingArticles {
		if cancelledEarly {
			break
		}
		select {
		case <-ctx.Done():
			cancelledEarly = true
			continue
		default:
		}

		article := article
		sem <- struct{}{}
		wg.Add(1)

		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			articleID := article.ArticleID()
			repository.MarkStatus(articleID, "running", "")

			if p.opts.ArticleDelayMaxMs > 0 {
				minD := max(0, p.opts.ArticleDelayMinMs)
				maxD := max(minD, p.opts.ArticleDelayMaxMs)
				delay := minD
				if maxD > minD {
					delay = minD + rand.IntN(maxD-minD+1)
				}
				if delay > 0 {
					select {
					case <-ctx.Done():
						return
					case <-time.After(time.Duration(delay) * time.Millisecond):
					}
				}
			}

			html, err := client.DownloadHTML(ctx, article.DirectURL)
			if err != nil {
				repository.MarkStatus(articleID, "failed", err.Error())
				errorRows.Store(errorIdx.Add(1), map[string]interface{}{
					"article_id":    articleID,
					"title":         article.Title,
					"direct_url":    article.DirectURL,
					"error_message": err.Error(),
				})
				failed.Add(1)
				cur := processed.Add(1)
				if cur%10 == 0 {
					p.logf("进度: %d/%d | 完成: %d | 失败: %d", cur, len(pendingArticles), completed.Load(), failed.Load())
				}
				return
			}

			if p.opts.ExportHTML {
				publishStr := formatPublishTime(article.PublishTime)
				if err := svc.SaveHTML(paths, articleID, article.Title, publishStr, html); err != nil {
					p.logf("保存 HTML 失败 [%s]: %v", articleID, err)
				}
			}

			if p.opts.ExportMarkdown {
				publishStr := formatPublishTime(article.PublishTime)
				if err := svc.SaveMarkdown(paths, articleID, article.Title, publishStr, html); err != nil {
					p.logf("保存 Markdown 失败 [%s]: %v", articleID, err)
				}
			}

			if p.opts.ExportExcelDetails {
				if shouldCollectDetail(articleID, p.opts.DetailSampleRate) {
					detail, err := client.GetArticleDetails(ctx, token, &article, html, p.opts.FetchComments)
					if err == nil {
						detail["detail_sampled"] = 1
						detailRows.Store(detailIdx.Add(1), detail)
					}
				} else {
					detailRows.Store(detailIdx.Add(1), buildSkippedDetailRow(&article))
				}
			}

			repository.MarkStatus(articleID, "completed", "")
			completed.Add(1)

			cur := processed.Add(1)
			if cur%10 == 0 {
				p.logf("进度: %d/%d | 完成: %d | 失败: %d", cur, len(pendingArticles), completed.Load(), failed.Load())
			}
		}()
	}

	wg.Wait()

	// Phase 5: Write detail/error Excel
	var detailRowsList []map[string]interface{}
	detailRows.Range(func(_, v interface{}) bool {
		if row, ok := v.(map[string]interface{}); ok {
			detailRowsList = append(detailRowsList, row)
		}
		return true
	})

	var errorRowsList []map[string]interface{}
	errorRows.Range(func(_, v interface{}) bool {
		if row, ok := v.(map[string]interface{}); ok {
			errorRowsList = append(errorRowsList, row)
		}
		return true
	})

	if len(detailRowsList) > 0 {
		if err := excelExp.AppendRows(paths.ArticleDetailsXlsx(), detailRowsList); err != nil {
			p.logf("写入详情 Excel 失败: %v", err)
		}
	}

	if len(errorRowsList) > 0 {
		if err := excelExp.AppendRows(paths.ErrorLinksXlsx(), errorRowsList); err != nil {
			p.logf("写入失败列表 Excel 失败: %v", err)
		}
	}

	// Phase 6: Save manifest and summary
	finishedAt := time.Now()
	completedN := int(completed.Load())
	failedN := int(failed.Load())

	manifest := map[string]interface{}{
		"started_at":  startedAt.Format(time.RFC3339),
		"finished_at": finishedAt.Format(time.RFC3339),
		"resumed":     resumeRoot != "",
		"pages":       pages,
		"total":       len(allArticles),
		"completed":   completedN,
		"skipped":     skipped,
		"failed":      failedN,
		"output_root": paths.TaskRoot,
	}
	_ = svc.SaveManifest(paths, manifest)

	latestPath := ensureLatestShortcut(paths)
	summary := buildTaskSummary(taskName, token, paths, startedAt, finishedAt, pages, len(allArticles), completedN, skipped, failedN, p.opts, resumeRoot != "", allArticles, latestPath)
	_ = svc.SaveTaskSummary(paths, summary)

	p.logf("任务完成。完成: %d, 跳过: %d, 失败: %d", completedN, skipped, failedN)

	return &models.PipelineResult{
		OutputRoot: paths.TaskRoot,
		Total:      len(allArticles),
		Completed:  completedN,
		Skipped:    skipped,
		Failed:     failedN,
	}, nil
}

func (p *Pipeline) collectArticles(ctx context.Context, client *api.Client, token *models.TokenLink, pages int) ([]models.ArticleRecord, map[string]bool) {
	var allArticles []models.ArticleRecord
	idSet := make(map[string]bool)

	for page := 0; page < pages; page++ {
		select {
		case <-ctx.Done():
			return allArticles, idSet
		default:
		}

		p.logf("抓取列表页 %d/%d", page+1, pages)

		list, err := client.GetArticleList(ctx, token, page)
		if err != nil {
			p.logf("List page %d failed: %v. Skipped.", page+1, err)
			continue
		}

		for _, a := range list {
			id := a.ArticleID()
			if !idSet[id] {
				idSet[id] = true
				allArticles = append(allArticles, a)
			}
		}

		if p.opts.ListPageDelayMs > 0 && page < pages-1 {
			select {
			case <-ctx.Done():
				return allArticles, idSet
			case <-time.After(time.Duration(p.opts.ListPageDelayMs) * time.Millisecond):
			}
		}
	}

	return allArticles, idSet
}

func (p *Pipeline) logf(format string, args ...interface{}) {
	if p.log != nil {
		p.log(fmt.Sprintf(format, args...))
	}
}

func formatPublishTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format("2006-01-02 15:04:05")
	return &s
}

func shouldCollectDetail(articleID string, sampleRate float64) bool {
	if sampleRate >= 1.0 {
		return true
	}
	if sampleRate <= 0.0 {
		return false
	}
	h := stableHash(articleID)
	score := float64(h%10000) / 10000.0
	return score < sampleRate
}

func stableHash(input string) uint32 {
	var hash uint32 = 2166136261
	for _, ch := range input {
		hash ^= uint32(ch)
		hash *= 16777619
	}
	return hash & 0x7FFFFFFF
}

func buildSkippedDetailRow(article *models.ArticleRecord) map[string]interface{} {
	publishTime := ""
	if article.PublishTime != nil {
		publishTime = article.PublishTime.Format("2006-01-02 15:04:05")
	}
	return map[string]interface{}{
		"article_id":       article.ArticleID(),
		"title":            article.Title,
		"publish_time":     publishTime,
		"direct_url":       article.DirectURL,
		"source_url":       article.SourceURL,
		"read_num":         nil,
		"like_num":         nil,
		"share_num":        nil,
		"show_read":        nil,
		"comments":         "[]",
		"comment_like_nums": "[]",
		"detail_sampled":   0,
	}
}

func tryResolveResumeRoot(outputRoot, taskName string) string {
	root, _ := filepath.Abs(outputRoot)
	if _, err := os.Stat(root); err != nil {
		return ""
	}

	prefix := export.SanitizePathSegment(taskName) + "_"
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}

	var latestDir string
	var latestTime time.Time
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), prefix) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(latestTime) {
			latestTime = info.ModTime()
			latestDir = filepath.Join(root, e.Name())
		}
	}

	return latestDir
}

func ensureLatestShortcut(paths *export.Paths) string {
	pointerPath := filepath.Join(paths.OutputRoot, "latest.path.txt")
	_ = os.WriteFile(pointerPath, []byte(paths.TaskRoot), 0o644)
	return pointerPath
}

func buildTaskSummary(taskName string, token *models.TokenLink, paths *export.Paths, startedAt, finishedAt time.Time, pages, total, completed, skipped, failed int, opts models.TaskOptions, resumed bool, articles []models.ArticleRecord, latestPath string) string {
	var sb strings.Builder

	sb.WriteString("# 微信公众号抓取信息\n\n")
	sb.WriteString("## 基本信息\n")
	sb.WriteString(fmt.Sprintf("- 任务名称: %s\n", taskName))
	sb.WriteString(fmt.Sprintf("- 输出目录: `%s`\n", paths.TaskRoot))
	sb.WriteString(fmt.Sprintf("- 抓取开始: %s\n", startedAt.Format("2006-01-02 15:04:05 -07:00")))
	sb.WriteString(fmt.Sprintf("- 抓取结束: %s\n", finishedAt.Format("2006-01-02 15:04:05 -07:00")))
	sb.WriteString(fmt.Sprintf("- 抓取耗时: %.1f 秒\n", finishedAt.Sub(startedAt).Seconds()))
	sb.WriteString(fmt.Sprintf("- 公众号标识(__biz): %s\n", token.Biz))
	resumeText := "否（新建任务目录）"
	if resumed {
		resumeText = "是（复用历史任务目录）"
	}
	sb.WriteString(fmt.Sprintf("- 断点续传: %s\n\n", resumeText))

	sb.WriteString("## 抓取参数\n")
	sb.WriteString(fmt.Sprintf("- 抓取页数上限: %d\n", pages))
	maxArtText := "不限制"
	if opts.MaxArticles > 0 {
		maxArtText = fmt.Sprintf("%d", opts.MaxArticles)
	}
	sb.WriteString(fmt.Sprintf("- 抓取数量上限: %s\n", maxArtText))
	sb.WriteString(fmt.Sprintf("- 线程并发: %d\n", opts.MaxConcurrency))
	sb.WriteString(fmt.Sprintf("- 列表页间隔: %d ms\n", opts.ListPageDelayMs))
	sb.WriteString(fmt.Sprintf("- 单篇随机延时: %d-%d ms\n", opts.ArticleDelayMinMs, opts.ArticleDelayMaxMs))
	sb.WriteString(fmt.Sprintf("- detail sample rate: %.3f\n", opts.DetailSampleRate))
	fetchStr := "No"
	if opts.FetchComments {
		fetchStr = "Yes"
	}
	sb.WriteString(fmt.Sprintf("- fetch comments: %s\n\n", fetchStr))

	sb.WriteString("## 抓取结果\n")
	sb.WriteString(fmt.Sprintf("- 列表去重后总数: %d\n", total))
	sb.WriteString(fmt.Sprintf("- 本次完成: %d\n", completed))
	sb.WriteString(fmt.Sprintf("- 断点跳过: %d\n", skipped))
	sb.WriteString(fmt.Sprintf("- 失败数量: %d\n\n", failed))

	sb.WriteString("## 文章样本（最多 5 条）\n")
	count := min(5, len(articles))
	if count == 0 {
		sb.WriteString("- 无\n")
	} else {
		for i := 0; i < count; i++ {
			a := articles[i]
			pt := "未知"
			if a.PublishTime != nil {
				pt = a.PublishTime.Format("2006-01-02 15:04:05")
			}
			sb.WriteString(fmt.Sprintf("%d. %s（%s）\n", i+1, a.Title, pt))
		}
	}

	return sb.String()
}

