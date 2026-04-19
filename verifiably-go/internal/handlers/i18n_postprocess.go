package handlers

import (
	"bytes"
	"context"
	"io"
	"strings"
	"unicode"

	"golang.org/x/net/html"
)

// translateHTML walks the rendered HTML in-place and translates every visible
// text node to the target language. Short strings, punctuation-only strings,
// and content inside <script>/<style>/<code>/<pre>/<textarea> are skipped.
// Elements (or ancestors) marked translate="no" or class containing "mono" /
// "notr" are skipped too — `.mono` spans hold identifiers/URLs/IDs that must
// render verbatim.
//
// Falls back to the original body on any parse error — translation should
// never break the page.
func translateHTML(ctx context.Context, body []byte, lang string, tr Translator) []byte {
	if tr == nil || lang == "" || lang == "en" || len(body) == 0 {
		return body
	}
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return body
	}
	walkAndTranslate(ctx, doc, lang, tr, false)
	var buf bytes.Buffer
	if err := html.Render(&buf, doc); err != nil {
		return body
	}
	return buf.Bytes()
}

var skipTags = map[string]bool{
	"script": true, "style": true, "code": true, "pre": true,
	"textarea": true, "svg": true, "noscript": true,
}

// attrsThatHoldUserText lists the attributes whose VALUES are shown to the
// user (as tooltips, placeholders, button labels, etc.) and are worth
// translating alongside text nodes.
var attrsThatHoldUserText = []string{"title", "placeholder", "alt", "aria-label"}

func walkAndTranslate(ctx context.Context, n *html.Node, lang string, tr Translator, insideSkip bool) {
	if n == nil {
		return
	}
	if n.Type == html.ElementNode {
		if skipTags[n.Data] {
			insideSkip = true
		}
		if !insideSkip && elementIsNoTranslate(n) {
			insideSkip = true
		}
		if !insideSkip {
			// Translate user-facing attributes in place.
			for i := range n.Attr {
				a := &n.Attr[i]
				if !attrIsUserFacing(a.Key) {
					continue
				}
				if translated := translateIfWorth(ctx, a.Val, lang, tr); translated != "" {
					a.Val = translated
				}
			}
		}
	}
	if n.Type == html.TextNode && !insideSkip {
		if translated := translateIfWorth(ctx, n.Data, lang, tr); translated != "" {
			n.Data = translated
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walkAndTranslate(ctx, c, lang, tr, insideSkip)
	}
}

// elementIsNoTranslate checks an element's translate= and class= for opt-outs.
// We honor the standard `translate="no"` HTML attribute and treat the project's
// `.mono` class as a no-translate marker (it holds URLs, IDs, and other
// identifiers that must render verbatim).
func elementIsNoTranslate(n *html.Node) bool {
	for _, a := range n.Attr {
		switch a.Key {
		case "translate":
			if a.Val == "no" {
				return true
			}
		case "class":
			classes := strings.Fields(a.Val)
			for _, c := range classes {
				if c == "mono" || c == "notr" {
					return true
				}
			}
		}
	}
	return false
}

func attrIsUserFacing(key string) bool {
	for _, k := range attrsThatHoldUserText {
		if k == key {
			return true
		}
	}
	return false
}

// translateIfWorth returns a translated replacement for s, or "" if the text
// should be left alone (empty, whitespace-only, purely punctuation/symbols,
// or fewer than 2 letters).
func translateIfWorth(ctx context.Context, s, lang string, tr Translator) string {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return ""
	}
	// Skip strings with no letters — punctuation, arrows, numbers, IDs.
	hasLetters := 0
	for _, r := range trimmed {
		if unicode.IsLetter(r) {
			hasLetters++
			if hasLetters >= 2 {
				break
			}
		}
	}
	if hasLetters < 2 {
		return ""
	}
	// Preserve the original's leading/trailing whitespace around the translated
	// core, so span layout doesn't collapse.
	lead, core, trail := splitWhitespace(s)
	translated := tr.Translate(ctx, core, lang)
	if translated == "" || translated == core {
		return ""
	}
	return lead + translated + trail
}

func splitWhitespace(s string) (lead, core, trail string) {
	i := 0
	for i < len(s) && isSpace(s[i]) {
		i++
	}
	j := len(s)
	for j > i && isSpace(s[j-1]) {
		j--
	}
	return s[:i], s[i:j], s[j:]
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

// interface assertion — compile-time check that io.Writer usage elsewhere
// isn't accidentally broken by this file.
var _ io.Writer = (*bytes.Buffer)(nil)
