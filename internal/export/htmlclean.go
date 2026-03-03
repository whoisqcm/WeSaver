package export

import (
	"regexp"
	"strings"
)

// CleanHTMLForOffline prepares a WeChat article HTML page for offline viewing:
//   - Removes external <script> tags (WeChat JS bundles that fail under file:// protocol)
//   - Removes external <link rel="stylesheet"> pointing to remote CDNs
//   - Converts lazy-loaded images (data-src) to standard src attributes
//   - Injects minimal CSS to keep the article readable without WeChat styles
func CleanHTMLForOffline(html string) string {
	html = removeExternalScripts(html)
	html = removeExternalStylesheets(html)
	html = fixLazyImages(html)
	html = injectOfflineStyles(html)
	return html
}

// removeExternalScripts strips <script> tags that load from external URLs.
// Inline <script>...</script> blocks without a src attribute are preserved.
var reExternalScript = regexp.MustCompile(`(?is)<script\b[^>]*\bsrc\s*=\s*["'][^"']*["'][^>]*>\s*</script>`)

func removeExternalScripts(html string) string {
	return reExternalScript.ReplaceAllString(html, "")
}

// removeExternalStylesheets strips <link rel="stylesheet"> tags pointing to
// external resources (res.wx.qq.com, etc.) that would fail offline.
var reExternalStylesheet = regexp.MustCompile(`(?is)<link\b[^>]*\brel\s*=\s*["']stylesheet["'][^>]*\bhref\s*=\s*["']https?://[^"']*["'][^>]*/?>`)
var reExternalStylesheet2 = regexp.MustCompile(`(?is)<link\b[^>]*\bhref\s*=\s*["']https?://[^"']*["'][^>]*\brel\s*=\s*["']stylesheet["'][^>]*/?>`)

func removeExternalStylesheets(html string) string {
	html = reExternalStylesheet.ReplaceAllString(html, "")
	html = reExternalStylesheet2.ReplaceAllString(html, "")
	return html
}

// fixLazyImages converts data-src attributes to src for images.
// WeChat articles use data-src for lazy loading; without JavaScript these
// images would remain invisible.
var reLazyImg = regexp.MustCompile(`(?is)(<img\b[^>]*?)\bdata-src\s*=\s*`)

func fixLazyImages(html string) string {
	// If an img already has both src and data-src, we replace data-src with
	// a data attribute to avoid duplicate src. But the common case in WeChat
	// is that img tags only have data-src and no src.

	// Step 1: For img tags that have data-src but no src=, convert data-src → src
	// Step 2: For img tags that have both, leave as-is (src already works)

	// Simple approach: rename all data-src to src. If there is already a src,
	// the browser will use whichever appears first; but in WeChat HTML the
	// existing src is typically a 1x1 transparent placeholder, so we remove it.
	html = removePlaceholderSrc(html)
	html = reLazyImg.ReplaceAllString(html, "${1}src=")
	return html
}

// removePlaceholderSrc removes src="data:image/..." or src="" placeholder
// attributes from img tags that also have data-src, so the data-src→src
// conversion produces a clean result.
var rePlaceholderSrc = regexp.MustCompile(`(?is)(<img\b[^>]*?)\bsrc\s*=\s*["'](?:data:image[^"']*|about:blank|)["']([^>]*\bdata-src\s*=)`)

func removePlaceholderSrc(html string) string {
	return rePlaceholderSrc.ReplaceAllString(html, "${1}${2}")
}

const offlineCSS = `<style id="wesaver-offline">
/* WeSaver offline reading styles */
body { max-width: 720px; margin: 0 auto; padding: 20px; font-family: -apple-system, "Segoe UI", sans-serif; line-height: 1.8; color: #333; background: #fff; }
img { max-width: 100% !important; height: auto !important; display: block; margin: 12px auto; }
#js_content { visibility: visible !important; }
.rich_media_content { visibility: visible !important; overflow: visible !important; }
pre, code { background: #f5f5f5; padding: 2px 6px; border-radius: 3px; overflow-x: auto; }
pre code { padding: 0; }
blockquote { border-left: 4px solid #ddd; margin: 16px 0; padding: 8px 16px; color: #666; }
a { color: #576b95; }
</style>`

func injectOfflineStyles(html string) string {
	// Try to inject before </head>
	if idx := strings.Index(strings.ToLower(html), "</head>"); idx >= 0 {
		return html[:idx] + offlineCSS + "\n" + html[idx:]
	}
	// Fallback: prepend
	return offlineCSS + "\n" + html
}
