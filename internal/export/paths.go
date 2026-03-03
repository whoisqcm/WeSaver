package export

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var illegalChars = regexp.MustCompile(`[\\/*?:"<>|]`)

type Paths struct {
	OutputRoot    string
	PublisherName string
	StartedAt     time.Time
	TaskRoot      string
	DataDir       string
	RawDir        string
	HTMLDir       string
	MdDir         string
	AssetsDir     string
}

func NewPaths(outputRoot, publisherName string, startedAt time.Time, taskRootOverride string) *Paths {
	absRoot, err := filepath.Abs(outputRoot)
	if err != nil {
		absRoot = outputRoot
	}
	safeName := SanitizePathSegment(publisherName)

	taskRoot := taskRootOverride
	if taskRoot == "" {
		taskRoot = filepath.Join(absRoot, fmt.Sprintf("%s_%s", safeName, startedAt.Format("20060102_150405")))
	} else {
		absTask, err := filepath.Abs(taskRoot)
		if err == nil {
			taskRoot = absTask
		}
	}

	return &Paths{
		OutputRoot:    absRoot,
		PublisherName: safeName,
		StartedAt:     startedAt,
		TaskRoot:      taskRoot,
		DataDir:       filepath.Join(taskRoot, "data"),
		RawDir:        filepath.Join(taskRoot, "raw"),
		HTMLDir:       filepath.Join(taskRoot, "raw", "html"),
		MdDir:         filepath.Join(taskRoot, "raw", "md"),
		AssetsDir:     filepath.Join(taskRoot, "raw", "assets"),
	}
}

func (p *Paths) ArticleListXlsx() string    { return filepath.Join(p.DataDir, "article_list.xlsx") }
func (p *Paths) ArticleDetailsXlsx() string { return filepath.Join(p.DataDir, "article_details.xlsx") }
func (p *Paths) ErrorLinksXlsx() string     { return filepath.Join(p.DataDir, "error_links.xlsx") }
func (p *Paths) ManifestJSON() string       { return filepath.Join(p.TaskRoot, "manifest.json") }
func (p *Paths) TaskSummaryMd() string      { return filepath.Join(p.TaskRoot, "公众号抓取信息.md") }

func (p *Paths) EnsureFolders() error {
	dirs := []string{p.TaskRoot, p.DataDir, p.RawDir, p.HTMLDir, p.MdDir, p.AssetsDir}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func SanitizePathSegment(input string) string {
	s := strings.TrimSpace(input)
	if s == "" {
		s = "Unknown"
	}
	s = illegalChars.ReplaceAllString(s, "_")
	s = strings.ReplaceAll(s, ".", "_")
	if len(s) > 80 {
		s = s[:80]
	}
	return s
}
