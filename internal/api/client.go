package api

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"
	"time"

	"wesaver/internal/models"
)

type Client struct {
	http       *http.Client
	maxRetries int
}

func NewClient(opts models.TaskOptions) *Client {
	return &Client{
		http: &http.Client{
			Timeout: opts.HTTPTimeout,
			Transport: &http.Transport{
				MaxIdleConns:        64,
				MaxIdleConnsPerHost: 64,
				IdleConnTimeout:     2 * time.Minute,
			},
		},
		maxRetries: opts.MaxRetries,
	}
}

func (c *Client) doGet(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(300*(1<<(attempt-1))) * time.Millisecond
			jitter := time.Duration(rand.IntN(100)) * time.Millisecond
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay + jitter):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, fmt.Errorf("build request: %w", err)
		}
		setHeaders(req)

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode >= 400 {
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body[:min(len(body), 200)]))
			continue
		}
		return body, nil
	}
	return nil, lastErr
}

func (c *Client) doPost(ctx context.Context, rawURL string, form url.Values) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(300*(1<<(attempt-1))) * time.Millisecond
			jitter := time.Duration(rand.IntN(100)) * time.Millisecond
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay + jitter):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(form.Encode()))
		if err != nil {
			return nil, fmt.Errorf("build request: %w", err)
		}
		setHeaders(req)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode >= 400 {
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body[:min(len(body), 200)]))
			continue
		}
		return body, nil
	}
	return nil, lastErr
}

func setHeaders(req *http.Request) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/126.0 Safari/537.36")
	req.Header.Set("Accept", "application/json, text/html")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
}

func (c *Client) DownloadHTML(ctx context.Context, rawURL string) (string, error) {
	body, err := c.doGet(ctx, rawURL)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (c *Client) GetArticleList(ctx context.Context, token *models.TokenLink, page int) ([]models.ArticleRecord, error) {
	offset := max(page, 0) * 10
	u := fmt.Sprintf(
		"https://mp.weixin.qq.com/mp/profile_ext?action=getmsg&__biz=%s&offset=%d&count=10&is_ok=1&scene=124&uin=%s&key=%s&pass_ticket=%s&wxtoken=&appmsg_token=&x5=0&f=json",
		token.Biz, offset, token.Uin, token.Key, url.QueryEscape(token.PassTicket),
	)

	body, err := c.doGet(ctx, u)
	if err != nil {
		return nil, err
	}

	var outer map[string]json.RawMessage
	if err := json.Unmarshal(body, &outer); err != nil {
		return nil, fmt.Errorf("parse outer: %w", err)
	}
	if ret := readRawInt(outer["ret"]); ret != 0 {
		msg := readRawString(outer["errmsg"])
		if msg == "" {
			msg = readRawString(outer["msg"])
		}
		return nil, fmt.Errorf("wechat ret=%d msg=%s", ret, msg)
	}

	listRawJSON, ok := outer["general_msg_list"]
	if !ok {
		return []models.ArticleRecord{}, nil
	}

	var listRawStr string
	if err := json.Unmarshal(listRawJSON, &listRawStr); err != nil {
		return nil, fmt.Errorf("parse list string: %w", err)
	}
	if strings.TrimSpace(listRawStr) == "" {
		return []models.ArticleRecord{}, nil
	}

	var listDoc struct {
		List []json.RawMessage `json:"list"`
	}
	if err := json.Unmarshal([]byte(listRawStr), &listDoc); err != nil {
		return nil, fmt.Errorf("parse list doc: %w", err)
	}

	var result []models.ArticleRecord
	for _, item := range listDoc.List {
		var entry struct {
			CommMsgInfo struct {
				DateTime int64 `json:"datetime"`
			} `json:"comm_msg_info"`
			AppMsgExtInfo json.RawMessage `json:"app_msg_ext_info"`
		}
		if err := json.Unmarshal(item, &entry); err != nil {
			continue
		}

		publishAt := time.Now()
		if entry.CommMsgInfo.DateTime > 0 {
			publishAt = time.Unix(entry.CommMsgInfo.DateTime, 0)
		}

		if entry.AppMsgExtInfo == nil {
			continue
		}

		var extInfo struct {
			Title              string            `json:"title"`
			ContentURL         string            `json:"content_url"`
			Cover              string            `json:"cover"`
			MultiAppMsgItemList []json.RawMessage `json:"multi_app_msg_item_list"`
		}
		if err := json.Unmarshal(entry.AppMsgExtInfo, &extInfo); err != nil {
			continue
		}

		if rec := buildRecord(extInfo.Title, extInfo.ContentURL, extInfo.Cover, publishAt); rec != nil {
			result = append(result, *rec)
		}

		for _, child := range extInfo.MultiAppMsgItemList {
			var sub struct {
				Title      string `json:"title"`
				ContentURL string `json:"content_url"`
				Cover      string `json:"cover"`
			}
			if err := json.Unmarshal(child, &sub); err != nil {
				continue
			}
			if rec := buildRecord(sub.Title, sub.ContentURL, sub.Cover, publishAt); rec != nil {
				result = append(result, *rec)
			}
		}
	}

	return result, nil
}

func (c *Client) GetArticleDetails(ctx context.Context, token *models.TokenLink, article *models.ArticleRecord, html string, fetchComments bool) (map[string]interface{}, error) {
	detailURL := fmt.Sprintf(
		"https://mp.weixin.qq.com/mp/getappmsgext?f=json&mock=&fasttmplajax=1&uin=%s&key=%s&pass_ticket=%s&__biz=%s",
		token.Uin, token.Key, url.QueryEscape(token.PassTicket), token.Biz,
	)

	reqID := extractBetween(html, "var req_id = ", ";")
	reqID = strings.NewReplacer("'", "", "\"", "").Replace(reqID)
	commentID := extractBetween(html, "var comment_id = '", "'")

	form := url.Values{
		"r":            {fmt.Sprintf("0.%d", rand.Int64N(9000000000000000)+1000000000000000)},
		"sn":           {article.Sn},
		"mid":          {article.Mid},
		"idx":          {article.Idx},
		"req_id":       {reqID},
		"title":        {article.Title},
		"comment_id":   {commentID},
		"appmsg_type":  {"9"},
		"__biz":        {token.Biz},
		"pass_ticket":  {token.PassTicket},
		"is_only_read": {"1"},
		"scene":        {"38"},
	}

	body, err := c.doPost(ctx, detailURL, form)
	if err != nil {
		return nil, err
	}

	var detailDoc map[string]interface{}
	if err := json.Unmarshal(body, &detailDoc); err != nil {
		return nil, fmt.Errorf("parse detail: %w", err)
	}

	publishTimeStr := ""
	if article.PublishTime != nil {
		publishTimeStr = article.PublishTime.Format("2006-01-02 15:04:05")
	}

	result := map[string]interface{}{
		"article_id":       article.ArticleID(),
		"title":            article.Title,
		"publish_time":     publishTimeStr,
		"direct_url":       article.DirectURL,
		"source_url":       article.SourceURL,
		"read_num":         findInt(detailDoc, "read_num"),
		"like_num":         findInt(detailDoc, "like_num"),
		"share_num":        findInt(detailDoc, "share_num"),
		"show_read":        findInt(detailDoc, "show_read"),
		"comments":         "[]",
		"comment_like_nums": "[]",
	}

	if fetchComments && commentID != "" {
		commentURL := fmt.Sprintf(
			"https://mp.weixin.qq.com/mp/appmsg_comment?action=getcomment&__biz=%s&appmsgid=0&idx=1&comment_id=%s&offset=0&limit=100&uin=%s&key=%s&pass_ticket=%s",
			token.Biz, commentID, token.Uin, token.Key, url.QueryEscape(token.PassTicket),
		)
		if commentBody, err := c.doGet(ctx, commentURL); err == nil {
			var commentDoc map[string]interface{}
			if json.Unmarshal(commentBody, &commentDoc) == nil {
				comments := findStringArray(commentDoc, "content")
				likes := findIntArray(commentDoc, "like_num")
				if cj, err := json.Marshal(comments); err == nil {
					result["comments"] = string(cj)
				}
				if lj, err := json.Marshal(likes); err == nil {
					result["comment_like_nums"] = string(lj)
				}
			}
		}
	}

	return result, nil
}

func buildRecord(title, contentURL, cover string, publishAt time.Time) *models.ArticleRecord {
	if strings.TrimSpace(contentURL) == "" {
		return nil
	}

	directURL := html.UnescapeString(contentURL)
	directURL = strings.ReplaceAll(directURL, "#wechat_redirect", "")

	u, err := url.Parse(directURL)
	if err != nil {
		return nil
	}

	q := u.Query()
	biz := q.Get("__biz")
	mid := q.Get("mid")
	idx := q.Get("idx")
	sn := q.Get("sn")

	if biz == "" || mid == "" || idx == "" || sn == "" {
		return nil
	}

	t := publishAt
	return &models.ArticleRecord{
		Biz:         biz,
		Mid:         mid,
		Idx:         idx,
		Sn:          sn,
		Title:       title,
		PublishTime: &t,
		SourceURL:   contentURL,
		DirectURL:   directURL,
		CoverURL:    cover,
	}
}

func readRawInt(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}
	var i int
	if err := json.Unmarshal(raw, &i); err == nil {
		return i
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return int(f)
	}
	return 0
}

func readRawString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(string(raw))
}

func extractBetween(source, startMarker, endMarker string) string {
	idx := strings.Index(source, startMarker)
	if idx < 0 {
		return ""
	}
	start := idx + len(startMarker)
	end := strings.Index(source[start:], endMarker)
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(source[start : start+end])
}

func findInt(data map[string]interface{}, key string) interface{} {
	for k, v := range data {
		if strings.EqualFold(k, key) {
			return v
		}
		if sub, ok := v.(map[string]interface{}); ok {
			if r := findInt(sub, key); r != nil {
				return r
			}
		}
	}
	return nil
}

func findStringArray(data map[string]interface{}, key string) []string {
	var result []string
	walkMap(data, func(k string, v interface{}) {
		if strings.EqualFold(k, key) {
			if s, ok := v.(string); ok && s != "" {
				result = append(result, s)
			}
		}
	})
	return result
}

func findIntArray(data map[string]interface{}, key string) []int {
	var result []int
	walkMap(data, func(k string, v interface{}) {
		if strings.EqualFold(k, key) {
			if f, ok := v.(float64); ok {
				result = append(result, int(f))
			}
		}
	})
	return result
}

func walkMap(data map[string]interface{}, fn func(string, interface{})) {
	for k, v := range data {
		fn(k, v)
		switch vv := v.(type) {
		case map[string]interface{}:
			walkMap(vv, fn)
		case []interface{}:
			for _, item := range vv {
				if sub, ok := item.(map[string]interface{}); ok {
					walkMap(sub, fn)
				}
			}
		}
	}
}
