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
	return writeUniqueArticleFile(paths.HTMLDir, title, publishTime, articleID, ".html", []byte(cleaned))
}

func (s *Service) SaveMarkdown(paths *Paths, articleID, title string, publishTime *string, html string) error {
	markdown, err := md.ConvertString(html)
	if err != nil {
		markdown = html
	}
	return writeUniqueArticleFile(paths.MdDir, title, publishTime, articleID, ".md", []byte(markdown))
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
	for attempt := 0; ; attempt++ {
		target := articleOutputPathCandidate(folder, title, publishTime, articleID, ext, attempt)
		if _, err := os.Stat(target); os.IsNotExist(err) {
			return target
		}
	}
}

func articleOutputPathCandidate(folder, title string, publishTime *string, articleID, ext string, attempt int) string {
	stem := buildArticleFileStem(title, publishTime)
	if attempt == 0 {
		return filepath.Join(folder, stem+ext)
	}

	safeSuffix := SanitizePathSegment(articleID)
	if safeSuffix == "" {
		safeSuffix = "dup"
	}
	if attempt == 1 {
		return filepath.Join(folder, stem+"_"+safeSuffix+ext)
	}

	return filepath.Join(folder, fmt.Sprintf("%s_%s_%d%s", stem, safeSuffix, attempt, ext))
}

func writeUniqueArticleFile(folder, title string, publishTime *string, articleID, ext string, data []byte) error {
	for attempt := 0; ; attempt++ {
		target := articleOutputPathCandidate(folder, title, publishTime, articleID, ext, attempt)
		f, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if _, err := f.Write(data); err != nil {
			_ = f.Close()
			_ = os.Remove(target)
			return err
		}
		if err := f.Close(); err != nil {
			_ = os.Remove(target)
			return err
		}
		return nil
	}
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
	stemRunes := []rune(stem)
	if len(stemRunes) > 150 {
		stem = string(stemRunes[:150])
	}
	return stem
}
