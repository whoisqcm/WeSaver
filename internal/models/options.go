package models

import "time"

type TaskOptions struct {
	OutputRoot         string        `json:"output_root"`
	ExportHTML         bool          `json:"export_html"`
	ExportMarkdown     bool          `json:"export_markdown"`
	ExportExcelDetails bool          `json:"export_excel_details"`
	MaxRetries         int           `json:"max_retries"`
	HTTPTimeout        time.Duration `json:"http_timeout"`
	MaxConcurrency     int           `json:"max_concurrency"`
	ListPageDelayMs    int           `json:"list_page_delay_ms"`
	ArticleDelayMinMs  int           `json:"article_delay_min_ms"`
	ArticleDelayMaxMs  int           `json:"article_delay_max_ms"`
	FetchComments      bool          `json:"fetch_comments"`
	DetailSampleRate   float64       `json:"detail_sample_rate"`
	ExportQueueCap     int           `json:"export_queue_capacity"`
	ExportWorkers      int           `json:"export_workers"`
	MaxArticles        int           `json:"max_articles"`
	ResumeByDefault    bool          `json:"resume_by_default"`
	OverwriteExisting  bool          `json:"overwrite_existing"`
}

func DefaultTaskOptions() TaskOptions {
	return TaskOptions{
		OutputRoot:         "output",
		ExportHTML:         true,
		ExportMarkdown:     true,
		ExportExcelDetails: true,
		MaxRetries:         2,
		HTTPTimeout:        25 * time.Second,
		MaxConcurrency:     2,
		ListPageDelayMs:    1800,
		ArticleDelayMinMs:  1800,
		ArticleDelayMaxMs:  3200,
		FetchComments:      false,
		DetailSampleRate:   0.25,
		ExportQueueCap:     1024,
		ExportWorkers:      2,
		MaxArticles:        0,
		ResumeByDefault:    true,
		OverwriteExisting:  false,
	}
}

type PipelineResult struct {
	OutputRoot string `json:"output_root"`
	Total      int    `json:"total"`
	Completed  int    `json:"completed"`
	Skipped    int    `json:"skipped"`
	Failed     int    `json:"failed"`
}

type SpeedProfile struct {
	Name              string
	ListPageDelayMs   int
	ArticleDelayMinMs int
	ArticleDelayMaxMs int
}

func SpeedProfiles() []SpeedProfile {
	return []SpeedProfile{
		{Name: "慢速（默认，防封）", ListPageDelayMs: 1800, ArticleDelayMinMs: 1800, ArticleDelayMaxMs: 3200},
		{Name: "标准", ListPageDelayMs: 900, ArticleDelayMinMs: 700, ArticleDelayMaxMs: 1300},
		{Name: "高速（风险更高）", ListPageDelayMs: 400, ArticleDelayMinMs: 150, ArticleDelayMaxMs: 450},
	}
}
