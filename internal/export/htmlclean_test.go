package export

import (
	"strings"
	"testing"
)

func TestRemoveExternalScripts(t *testing.T) {
	input := `<html><head>
<script src="https://res.wx.qq.com/mmbizappmsg/zh_CN/htmledition/js/assets/appmsg.mma8ahx160e29b33.js"></script>
<script>var inline = true;</script>
</head><body>hello</body></html>`

	result := removeExternalScripts(input)

	if strings.Contains(result, "appmsg.mma8ahx160e29b33.js") {
		t.Error("external script was not removed")
	}
	if !strings.Contains(result, "var inline = true") {
		t.Error("inline script was incorrectly removed")
	}
	if !strings.Contains(result, "hello") {
		t.Error("body content was removed")
	}
}

func TestRemoveExternalStylesheets(t *testing.T) {
	input := `<link rel="stylesheet" href="https://res.wx.qq.com/some.css" />
<link href="https://cdn.example.com/style.css" rel="stylesheet">
<style>.inline { color: red; }</style>`

	result := removeExternalStylesheets(input)

	if strings.Contains(result, "res.wx.qq.com") {
		t.Error("external stylesheet from wx was not removed")
	}
	if strings.Contains(result, "cdn.example.com") {
		t.Error("external stylesheet from cdn was not removed")
	}
	if !strings.Contains(result, ".inline { color: red; }") {
		t.Error("inline style was incorrectly removed")
	}
}

func TestFixLazyImages(t *testing.T) {
	input := `<img data-src="https://mmbiz.qpic.cn/real-image.jpg" />`
	result := fixLazyImages(input)

	if !strings.Contains(result, `src="https://mmbiz.qpic.cn/real-image.jpg"`) {
		t.Errorf("data-src not converted to src, got: %s", result)
	}
	if strings.Contains(result, "data-src") {
		t.Error("data-src attribute still present")
	}
}

func TestFixLazyImagesWithPlaceholder(t *testing.T) {
	input := `<img src="data:image/gif;base64,R0lGODl" data-src="https://mmbiz.qpic.cn/real.jpg" />`
	result := fixLazyImages(input)

	if !strings.Contains(result, `src="https://mmbiz.qpic.cn/real.jpg"`) {
		t.Errorf("data-src not properly converted, got: %s", result)
	}
}

func TestInjectOfflineStyles(t *testing.T) {
	input := `<html><head><title>Test</title></head><body>content</body></html>`
	result := injectOfflineStyles(input)

	if !strings.Contains(result, `id="wesaver-offline"`) {
		t.Error("offline styles not injected")
	}

	headIdx := strings.Index(result, "</head>")
	styleIdx := strings.Index(result, "wesaver-offline")
	if styleIdx > headIdx {
		t.Error("styles should be injected before </head>")
	}
}

func TestCleanHTMLForOffline_Integration(t *testing.T) {
	input := `<!DOCTYPE html><html><head>
<meta charset="utf-8">
<link rel="stylesheet" href="https://res.wx.qq.com/style.css">
<script src="https://res.wx.qq.com/mmbizappmsg/zh_CN/htmledition/js/assets/appmsg.js"></script>
</head><body>
<div id="js_content" style="visibility:hidden">
<p>文章正文</p>
<img data-src="https://mmbiz.qpic.cn/article-image.jpg">
</div>
<script>var config = {};</script>
</body></html>`

	result := CleanHTMLForOffline(input)

	// External scripts removed
	if strings.Contains(result, "res.wx.qq.com/mmbizappmsg") {
		t.Error("external script not removed")
	}
	// External stylesheets removed
	if strings.Contains(result, `href="https://res.wx.qq.com/style.css"`) {
		t.Error("external stylesheet not removed")
	}
	// Inline scripts preserved
	if !strings.Contains(result, "var config = {}") {
		t.Error("inline script incorrectly removed")
	}
	// Article content preserved
	if !strings.Contains(result, "文章正文") {
		t.Error("article content lost")
	}
	// Lazy images fixed
	if !strings.Contains(result, `src="https://mmbiz.qpic.cn/article-image.jpg"`) {
		t.Error("lazy image not fixed")
	}
	// Offline styles injected
	if !strings.Contains(result, "wesaver-offline") {
		t.Error("offline styles not injected")
	}
	// js_content visibility fix
	if !strings.Contains(result, "visibility: visible") {
		t.Error("js_content visibility fix not present")
	}
}
