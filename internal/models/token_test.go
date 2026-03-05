package models

import "testing"

func TestParseTokenLink_HandlesEscapedSeparators(t *testing.T) {
	raw := `https://mp.weixin.qq.com/mp/profile_ext?__biz=MzA1&uin=123&key=abc&pass_ticket=pt%2Bv\u0026foo=bar`

	token, ok := ParseTokenLink(raw)
	if !ok || token == nil {
		t.Fatalf("expected token link to parse, got ok=%v", ok)
	}
	if token.Biz != "MzA1" || token.Uin != "123" || token.Key != "abc" || token.PassTicket != "pt+v" {
		t.Fatalf("unexpected parse result: %+v", token)
	}
}

func TestParseTokenLink_PreservesAmpSemicolonInValue(t *testing.T) {
	raw := "https://mp.weixin.qq.com/mp/profile_ext?__biz=biz&uin=u&key=amp;123&pass_ticket=pt"

	token, ok := ParseTokenLink(raw)
	if !ok || token == nil {
		t.Fatalf("expected token link to parse, got ok=%v", ok)
	}
	if token.Key != "amp;123" {
		t.Fatalf("key value was unexpectedly modified: %q", token.Key)
	}
}
