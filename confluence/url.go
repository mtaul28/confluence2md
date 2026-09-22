// SPDX-License-Identifier: Apache-2.0

package confluence

import (
	"fmt"
	"net/url"
	"strings"
)

// PageRef identifies a page from a browser URL. Either ID is set, or
// SpaceKey and Title are set (for old-style /display/ links).
type PageRef struct {
	ID       string
	SpaceKey string
	Title    string
}

// ParsePageURL splits a Confluence page URL into the site's base URL (the
// part the REST API hangs off of) and a reference to the page itself.
//
// Supported forms:
//
//	https://example.atlassian.net/wiki/spaces/KEY/pages/12345/Page+Title
//	https://confluence.example.com/pages/viewpage.action?pageId=12345
//	https://confluence.example.com/display/KEY/Page+Title
//
// Any context path in front of those (e.g. /confluence) is kept as part of
// the base URL.
func ParsePageURL(raw string) (string, PageRef, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", PageRef{}, fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" || u.Host == "" {
		return "", PageRef{}, fmt.Errorf("not an http(s) URL: %q", raw)
	}

	site := u.Scheme + "://" + u.Host
	path := u.EscapedPath()

	// ?pageId= works on every Server/DC URL that has it, regardless of path
	if id := u.Query().Get("pageId"); id != "" {
		prefix := path
		if i := strings.Index(path, "/pages/"); i >= 0 {
			prefix = path[:i]
		}
		return site + prefix, PageRef{ID: id}, nil
	}

	// Cloud (and newer DC): /spaces/KEY/pages/ID[/Title]
	if i := strings.Index(path, "/spaces/"); i >= 0 {
		parts := strings.Split(strings.Trim(path[i:], "/"), "/")
		if len(parts) >= 4 && parts[2] == "pages" && isNumeric(parts[3]) {
			return site + path[:i], PageRef{ID: parts[3]}, nil
		}
		// Some Cloud edit URLs look like /spaces/KEY/pages/edit-v2/ID
		if len(parts) >= 5 && parts[2] == "pages" && isNumeric(parts[4]) {
			return site + path[:i], PageRef{ID: parts[4]}, nil
		}
	}

	// Server/DC pretty links: /display/KEY/Page+Title
	if i := strings.Index(path, "/display/"); i >= 0 {
		parts := strings.SplitN(strings.Trim(path[i+len("/display/"):], "/"), "/", 2)
		if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
			title, err := url.QueryUnescape(parts[1])
			if err != nil {
				return "", PageRef{}, fmt.Errorf("could not decode page title in %q: %w", raw, err)
			}
			return site + path[:i], PageRef{SpaceKey: parts[0], Title: title}, nil
		}
	}

	return "", PageRef{}, fmt.Errorf("could not find a page ID in %q (expected a /spaces/KEY/pages/ID, viewpage.action?pageId=ID or /display/KEY/Title link)", raw)
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
