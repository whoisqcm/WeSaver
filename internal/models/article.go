package models

import "time"

type ArticleRecord struct {
	Biz         string
	Mid         string
	Idx         string
	Sn          string
	Title       string
	PublishTime *time.Time
	SourceURL   string
	DirectURL   string
	CoverURL    string
}

func (a *ArticleRecord) ArticleID() string {
	return a.Biz + "_" + a.Mid + "_" + a.Idx + "_" + a.Sn
}
