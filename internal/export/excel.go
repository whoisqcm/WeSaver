package export

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"
)

type ExcelExporter struct{}

func NewExcelExporter() *ExcelExporter {
	return &ExcelExporter{}
}

var defaultHeaders = []string{
	"article_id", "title", "publish_time", "direct_url", "source_url",
	"read_num", "like_num", "share_num", "show_read",
	"comments", "comment_like_nums",
}

func (e *ExcelExporter) AppendRows(filePath string, rows []map[string]interface{}) error {
	if len(rows) == 0 {
		return nil
	}

	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	var f *excelize.File
	var err error
	sheet := "data"

	if _, statErr := os.Stat(filePath); statErr == nil {
		f, err = excelize.OpenFile(filePath)
		if err != nil {
			return fmt.Errorf("open excel: %w", err)
		}
	} else {
		f = excelize.NewFile()
		idx, _ := f.NewSheet(sheet)
		f.SetActiveSheet(idx)
		// Delete default Sheet1 if different
		if sheet != "Sheet1" {
			f.DeleteSheet("Sheet1")
		}
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) > 0 {
		sheet = sheets[0]
	}

	headers := getOrCreateHeaders(f, sheet, rows)

	rows2, _ := f.GetRows(sheet)
	startRow := len(rows2) + 1
	if startRow < 2 {
		startRow = 2
	}

	for i, row := range rows {
		for j, h := range headers {
			col, _ := excelize.ColumnNumberToName(j + 1)
			cell := fmt.Sprintf("%s%d", col, startRow+i)
			val := ""
			if v, ok := row[h]; ok && v != nil {
				val = fmt.Sprintf("%v", v)
				if len(val) > 32767 {
					val = val[:32767]
				}
			}
			f.SetCellValue(sheet, cell, val)
		}
	}

	return f.SaveAs(filePath)
}

func (e *ExcelExporter) ReadColumnValues(filePath, columnName string) map[string]bool {
	result := make(map[string]bool)

	if _, err := os.Stat(filePath); err != nil {
		return result
	}

	f, err := excelize.OpenFile(filePath)
	if err != nil {
		return result
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return result
	}

	rows, err := f.GetRows(sheets[0])
	if err != nil || len(rows) == 0 {
		return result
	}

	targetCol := -1
	for i, h := range rows[0] {
		if strings.EqualFold(h, columnName) {
			targetCol = i
			break
		}
	}

	if targetCol < 0 {
		return result
	}

	for _, row := range rows[1:] {
		if targetCol < len(row) {
			val := strings.TrimSpace(row[targetCol])
			if val != "" {
				result[val] = true
			}
		}
	}

	return result
}

func getOrCreateHeaders(f *excelize.File, sheet string, rows []map[string]interface{}) []string {
	existingRows, _ := f.GetRows(sheet)
	if len(existingRows) > 0 && len(existingRows[0]) > 0 {
		var headers []string
		for _, h := range existingRows[0] {
			if h != "" {
				headers = append(headers, h)
			}
		}
		if len(headers) > 0 {
			return headers
		}
	}

	headers := make([]string, len(defaultHeaders))
	copy(headers, defaultHeaders)

	seen := make(map[string]bool)
	for _, h := range headers {
		seen[strings.ToLower(h)] = true
	}

	for _, row := range rows {
		for k := range row {
			if !seen[strings.ToLower(k)] {
				headers = append(headers, k)
				seen[strings.ToLower(k)] = true
			}
		}
	}

	for i, h := range headers {
		col, _ := excelize.ColumnNumberToName(i + 1)
		f.SetCellValue(sheet, col+"1", h)
	}

	return headers
}
