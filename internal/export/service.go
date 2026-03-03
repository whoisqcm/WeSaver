package export

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	md "github.com/JohannesKaufmann/html-to-markdown/v2"
)

type Service struct{}

func NewService() *Service {
	return &Service{}
}

func (s *Service) SaveHTML(paths *Paths, articleID, title string, publishTime *string, html string) error {
	cleaned := CleanHTMLForOffline(html)
	target := buildArticleOutputPath(paths.HTMLDir, title, publishTime, articleID, ".html")
	return os.WriteFile(target, []byte(cleaned), 0o644)
}

func (s *Service) SaveMarkdown(paths *Paths, articleID, title string, publishTime *string, html string) error {
	markdown, err := md.ConvertString(html)
	if err != nil {
		markdown = html
	}
	target := buildArticleOutputPath(paths.MdDir, title, publishTime, articleID, ".md")
	return os.WriteFile(target, []byte(markdown), 0o644)
}

func (s *Service) SaveManifest(paths *Paths, manifest interface{}) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(paths.ManifestJSON(), data, 0o644)
}

func (s *Service) SaveTaskSummary(paths *Paths, content string) error {
	return os.WriteFile(paths.TaskSummaryMd(), []byte(content), 0o644)
}

func buildArticleOutputPath(folder, title string, publishTime *string, articleID, ext string) string {
	stem := buildArticleFileStem(title, publishTime)
	target := filepath.Join(folder, stem+ext)

	if _, err := os.Stat(target); err == nil {
		safeSuffix := SanitizePathSegment(articleID)
		target = filepath.Join(folder, stem+"_"+safeSuffix+ext)
	}

	return target
}

func buildArticleFileStem(title string, publishTime *string) string {
	safeTitle := SanitizePathSegment(title)
	if safeTitle == "" {
		safeTitle = "Untitled"
	}

	publishText := "UnknownDate"
	if publishTime != nil && *publishTime != "" {
		publishText = strings.ReplaceAll(*publishTime, ":", "")
		publishText = strings.ReplaceAll(publishText, " ", "_")
		publishText = strings.ReplaceAll(publishText, "-", "")
	}

	stem := fmt.Sprintf("%s_%s", safeTitle, publishText)
	if len(stem) > 150 {
		stem = stem[:150]
	}
	return stem
}
