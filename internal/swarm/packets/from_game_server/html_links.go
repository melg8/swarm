// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import "strings"

// htmlLinkCap bounds the plausible link count of a dialog page: the
// longest quest and teleport pages carry a few dozen links, so 128
// is far past every honest page while a pathological page cannot
// grow the result without end.
const htmlLinkCap = 128

// HTMLLink is one dialog action of a server html page: the bypass
// command the link carries (the optional "-h " prefix stripped and
// the command trimmed - exactly the form RequestBypassToServer must
// send back) and the visible link text the dialog walker matches
// pages by ("Change profession to an Elven Knight").
type HTMLLink struct {
	Command string
	Text    string
}

// ParseHTMLLinks extracts the bypass links of a server dialog page,
// mirroring the server side extraction of
// HtmlUtil.buildHtmlBypassCache: the attribute values starting with
// "bypass " are matched case-insensitively on the lowercased html
// (the original casing is preserved in the result), the command
// runs to the closing quote, the optional "-h " prefix is stripped
// and the command trimmed. The link text is the content between the
// anchor tag end and the next tag. A page without links (or with an
// unterminated action attribute) yields nil.
func ParseHTMLLinks(html string) []HTMLLink {
	if html == "" {
		return nil
	}
	lower := strings.ToLower(html)
	links := make([]HTMLLink, 0, 8)

	searchFrom := 0
	for len(links) < htmlLinkCap {
		match := strings.Index(lower[searchFrom:], `="bypass `)
		if match == -1 {
			break
		}
		commandStart := searchFrom + match + len(`="bypass `)
		quoteEnd := strings.IndexByte(lower[commandStart:], '"')
		if quoteEnd == -1 {
			// An unterminated action attribute: the page is broken,
			// the links found so far stand.
			break
		}
		quoteEnd += commandStart

		command := html[commandStart:quoteEnd]
		command = strings.TrimPrefix(command, "-h ")
		command = strings.TrimSpace(command)

		links = append(links, HTMLLink{
			Command: command,
			Text:    anchorText(html, quoteEnd),
		})
		searchFrom = textEndOfAnchor(html, quoteEnd)
	}
	if len(links) == 0 {
		return nil
	}

	return links
}

// anchorText returns the visible text of the anchor whose action
// attribute ends at quoteEnd: the content between the tag closing
// bracket and the next tag.
func anchorText(html string, quoteEnd int) string {
	tagEnd := strings.IndexByte(html[quoteEnd:], '>')
	if tagEnd == -1 {
		return ""
	}
	textStart := quoteEnd + tagEnd + 1
	textEnd := strings.IndexByte(html[textStart:], '<')
	if textEnd == -1 {
		textEnd = len(html)
	} else {
		textEnd += textStart
	}

	return strings.TrimSpace(html[textStart:textEnd])
}

// textEndOfAnchor returns the scan position after the anchor of the
// link whose action attribute ends at quoteEnd: the next tag open
// after the anchor text (the closing </a> or whatever follows), so
// the scan skips the text it already consumed.
func textEndOfAnchor(html string, quoteEnd int) int {
	tagEnd := strings.IndexByte(html[quoteEnd:], '>')
	if tagEnd == -1 {
		return len(html)
	}
	textStart := quoteEnd + tagEnd + 1
	textEnd := strings.IndexByte(html[textStart:], '<')
	if textEnd == -1 {
		return len(html)
	}

	return textStart + textEnd
}
