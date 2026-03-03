package ui

import (
	"context"
	"time"

	"wesaver/internal/api"
	"wesaver/internal/models"
)

func validateTokenImpl(tokenURL string) (valid bool, message string, count int, taskName string) {
	token, ok := models.ParseTokenLink(tokenURL)
	if !ok {
		return false, "token 格式无效", 0, ""
	}

	opts := models.TaskOptions{
		MaxRetries:  0,
		HTTPTimeout: 12 * time.Second,
	}
	client := api.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	list, err := client.GetArticleList(ctx, token, 0)
	if err != nil {
		return false, "校验失败: " + err.Error(), 0, ""
	}

	if len(list) > 0 {
		return true, "有效", len(list), "mp_" + token.Biz
	}

	return false, "token 疑似失效/风控", 0, ""
}
