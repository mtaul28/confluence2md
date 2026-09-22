// SPDX-License-Identifier: Apache-2.0

// Package confluence is a minimal client for the bits of the Confluence REST
// API needed to pull pages down and hand them to the converter. It uses the
// v1 content API, which is available on both Cloud and Server/Data Center.
package confluence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultTimeout = 60 * time.Second

	// childPageLimit is how many children we ask for per request. Confluence
	// caps this server-side anyway, so we follow the "next" link regardless.
	childPageLimit = 100

	// maxRetries is how many times a rate-limited (429) or temporarily
	// unavailable (503) request is retried before giving up.
	maxRetries = 3
)

// ErrNotFound is returned when the requested page doesn't exist or the token
// can't see it (Confluence returns 404 for both).
var ErrNotFound = errors.New("page not found or not visible with this token")

// Client talks to a single Confluence site.
type Client struct {
	// BaseURL is the site root the REST API lives under, e.g.
	// https://example.atlassian.net/wiki or https://confluence.example.com.
	BaseURL string

	// Token is a Personal Access Token (Server/DC) or API token (Cloud).
	Token string

	// User is only needed for Cloud, where API tokens are sent as basic auth
	// with your account email. Leave empty to send Token as a bearer token.
	User string

	HTTPClient *http.Client

	// sleep is swapped out in tests so retries don't actually wait.
	sleep func(time.Duration)
}

// NewClient returns a client for the given site.
func NewClient(baseURL, token, user string) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		Token:      token,
		User:       user,
		HTTPClient: &http.Client{Timeout: defaultTimeout},
		sleep:      time.Sleep,
	}
}

// Page is a single Confluence page with its rendered body.
type Page struct {
	ID       string
	Title    string
	SpaceKey string
	// HTML is the page's export_view body, which is the same rendering
	// Confluence uses for its Word/PDF exports.
	HTML string
}

// PageSummary is the lightweight form returned when listing children.
type PageSummary struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type contentResponse struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Space struct {
		Key string `json:"key"`
	} `json:"space"`
	Body struct {
		ExportView struct {
			Value string `json:"value"`
		} `json:"export_view"`
	} `json:"body"`
}

type listResponse struct {
	Results []PageSummary `json:"results"`
	Links   struct {
		Next string `json:"next"`
	} `json:"_links"`
}

// GetPage fetches a page and its rendered body.
func (c *Client) GetPage(ctx context.Context, id string) (*Page, error) {
	var resp contentResponse
	path := "/rest/api/content/" + url.PathEscape(id) + "?expand=body.export_view,space"
	if err := c.getJSON(ctx, path, &resp); err != nil {
		return nil, fmt.Errorf("fetching page %s: %w", id, err)
	}
	return &Page{
		ID:       resp.ID,
		Title:    resp.Title,
		SpaceKey: resp.Space.Key,
		HTML:     resp.Body.ExportView.Value,
	}, nil
}

// ChildPages returns the direct children of a page, following pagination.
func (c *Client) ChildPages(ctx context.Context, id string) ([]PageSummary, error) {
	var children []PageSummary
	next := fmt.Sprintf("/rest/api/content/%s/child/page?limit=%d", url.PathEscape(id), childPageLimit)

	for next != "" {
		var resp listResponse
		if err := c.getJSON(ctx, next, &resp); err != nil {
			return nil, fmt.Errorf("listing children of %s: %w", id, err)
		}
		children = append(children, resp.Results...)
		next = resp.Links.Next
	}
	return children, nil
}

// FindPageID looks up a page ID by space key and exact title. This is only
// needed for /display/KEY/Title style links.
func (c *Client) FindPageID(ctx context.Context, spaceKey, title string) (string, error) {
	q := url.Values{}
	q.Set("type", "page")
	q.Set("spaceKey", spaceKey)
	q.Set("title", title)

	var resp listResponse
	if err := c.getJSON(ctx, "/rest/api/content?"+q.Encode(), &resp); err != nil {
		return "", fmt.Errorf("looking up %q in space %s: %w", title, spaceKey, err)
	}
	if len(resp.Results) == 0 {
		return "", fmt.Errorf("looking up %q in space %s: %w", title, spaceKey, ErrNotFound)
	}
	return resp.Results[0].ID, nil
}

// ResolveID turns a PageRef into a page ID, doing a title lookup if needed.
func (c *Client) ResolveID(ctx context.Context, ref PageRef) (string, error) {
	if ref.ID != "" {
		return ref.ID, nil
	}
	return c.FindPageID(ctx, ref.SpaceKey, ref.Title)
}

// getJSON GETs a path relative to BaseURL and decodes the JSON response.
// Paths returned in Confluence's "_links.next" are relative to the same base,
// so they can be passed straight back in here.
func (c *Client) getJSON(ctx context.Context, path string, out interface{}) error {
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Accept", "application/json")
		if c.User != "" {
			req.SetBasicAuth(c.User, c.Token)
		} else if c.Token != "" {
			req.Header.Set("Authorization", "Bearer "+c.Token)
		}

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return err
		}

		if (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable) && attempt < maxRetries {
			wait := retryAfter(resp.Header.Get("Retry-After"), attempt)
			_ = resp.Body.Close()
			c.sleep(wait)
			continue
		}

		err = decodeResponse(resp, out)
		_ = resp.Body.Close()
		return err
	}
}

func decodeResponse(resp *http.Response, out interface{}) error {
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return errors.New("401 unauthorized: check your token (Cloud API tokens also need --user set to your account email)")
	case resp.StatusCode == http.StatusForbidden:
		return errors.New("403 forbidden: the token doesn't have access to this page")
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("unexpected status %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	// A login page coming back as 200 is a common failure mode behind SSO
	// proxies, and produces a confusing JSON error without this check.
	if ct := resp.Header.Get("Content-Type"); ct != "" && !strings.Contains(ct, "json") {
		return fmt.Errorf("expected JSON but got %q (is the URL behind an SSO login page?)", ct)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}
	return nil
}

// retryAfter works out how long to wait before retrying. It honours a
// Retry-After header in seconds and otherwise backs off 1s, 2s, 4s...
func retryAfter(header string, attempt int) time.Duration {
	if secs, err := strconv.Atoi(strings.TrimSpace(header)); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second
	}
	return time.Second << attempt
}
