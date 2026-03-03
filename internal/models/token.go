package models

import (
	"net/url"
	"strings"
)

type TokenLink struct {
	Biz        string
	Uin        string
	Key        string
	PassTicket string
}

func ParseTokenLink(raw string) (*TokenLink, bool) {
	if strings.TrimSpace(raw) == "" {
		return nil, false
	}

	normalized := normalize(raw)
	if normalized == "" {
		return nil, false
	}

	params := parseQuery(normalized)
	biz := params["__biz"]
	uin := params["uin"]
	key := params["key"]
	passTicket := params["pass_ticket"]

	if biz == "" || uin == "" || key == "" || passTicket == "" {
		return nil, false
	}

	return &TokenLink{
		Biz:        biz,
		Uin:        uin,
		Key:        key,
		PassTicket: passTicket,
	}, true
}

func normalize(input string) string {
	s := strings.TrimSpace(input)
	s = strings.Trim(s, "\"'`")
	if s == "" {
		return ""
	}
	s = strings.NewReplacer("&amp;", "&", "amp;", "").Replace(s)
	return s
}

func parseQuery(raw string) map[string]string {
	result := make(map[string]string)

	idx := strings.Index(raw, "?")
	query := raw
	if idx >= 0 {
		query = raw[idx+1:]
	}

	if hi := strings.Index(query, "#"); hi >= 0 {
		query = query[:hi]
	}

	if bi := strings.Index(strings.ToLower(query), "__biz="); bi > 0 {
		query = query[bi:]
	}

	for _, part := range strings.Split(query, "&") {
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		key := safeDecode(kv[0])
		if key == "" {
			continue
		}
		value := ""
		if len(kv) > 1 {
			value = safeDecode(kv[1])
		}
		result[strings.ToLower(key)] = value
	}

	return result
}

func safeDecode(s string) string {
	if s == "" {
		return ""
	}
	decoded, err := url.QueryUnescape(s)
	if err != nil {
		return s
	}
	return decoded
}
