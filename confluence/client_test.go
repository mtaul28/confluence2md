// SPDX-License-Identifier: Apache-2.0

package confluence

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := NewClient(srv.URL+"/wiki", "secret", "")
	c.sleep = func(time.Duration) {}
	return c
}

func writeJSON(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprint(w, body)
}

func TestGetPage(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/wiki/rest/api/content/42" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("expand"); !strings.Contains(got, "body.export_view") {
			t.Errorf("expected export_view to be expanded, got %q", got)
		}
		writeJSON(w, `{
			"id": "42",
			"title": "Hello",
			"space": {"key": "ENG"},
			"body": {"export_view": {"value": "<p>hi</p>"}}
		}`)
	})

	page, err := c.GetPage(context.Background(), "42")
	if err != nil {
		t.Fatal(err)
	}
	if page.ID != "42" || page.Title != "Hello" || page.SpaceKey != "ENG" || page.HTML != "<p>hi</p>" {
		t.Errorf("unexpected page: %+v", page)
	}
}

func TestAuthHeaders(t *testing.T) {
	var got string
	handler := func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		writeJSON(w, `{"id":"1"}`)
	}

	c := newTestClient(t, handler)
	if _, err := c.GetPage(context.Background(), "1"); err != nil {
		t.Fatal(err)
	}
	if got != "Bearer secret" {
		t.Errorf("PAT auth: got %q, want bearer token", got)
	}

	c.User = "me@example.com"
	if _, err := c.GetPage(context.Background(), "1"); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "Basic ") {
		t.Errorf("Cloud auth: got %q, want basic auth", got)
	}
}

func TestChildPages_FollowsNextLink(t *testing.T) {
	calls := 0
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch r.URL.Query().Get("start") {
		case "":
			writeJSON(w, `{
				"results": [{"id":"2","title":"A"},{"id":"3","title":"B"}],
				"_links": {"next": "/rest/api/content/1/child/page?limit=2&start=2"}
			}`)
		case "2":
			writeJSON(w, `{"results": [{"id":"4","title":"C"}], "_links": {}}`)
		default:
			t.Errorf("unexpected request %s", r.URL)
		}
	})

	children, err := c.ChildPages(context.Background(), "1")
	if err != nil {
		t.Fatal(err)
	}
	if len(children) != 3 || children[2].Title != "C" {
		t.Errorf("unexpected children: %+v", children)
	}
	if calls != 2 {
		t.Errorf("expected 2 requests, got %d", calls)
	}
}

func TestFindPageID(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("spaceKey") != "OPS" || q.Get("title") != "Runbook & Notes" {
			t.Errorf("unexpected query %s", r.URL.RawQuery)
		}
		if q.Get("title") == "Runbook & Notes" {
			writeJSON(w, `{"results":[{"id":"77","title":"Runbook & Notes"}]}`)
			return
		}
		writeJSON(w, `{"results":[]}`)
	})

	id, err := c.ResolveID(context.Background(), PageRef{SpaceKey: "OPS", Title: "Runbook & Notes"})
	if err != nil {
		t.Fatal(err)
	}
	if id != "77" {
		t.Errorf("id = %q, want 77", id)
	}
}

func TestFindPageID_NoMatch(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"results":[]}`)
	})
	_, err := c.FindPageID(context.Background(), "OPS", "Missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestErrorStatuses(t *testing.T) {
	tests := []struct {
		status int
		want   string
	}{
		{http.StatusUnauthorized, "401"},
		{http.StatusForbidden, "403"},
		{http.StatusNotFound, "not found"},
		{http.StatusInternalServerError, "500"},
	}
	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "nope", tt.status)
			})
			_, err := c.GetPage(context.Background(), "1")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestHTMLResponseIsRejected(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, "<html>Please log in</html>")
	})
	_, err := c.GetPage(context.Background(), "1")
	if err == nil || !strings.Contains(err.Error(), "SSO") {
		t.Errorf("expected SSO hint, got %v", err)
	}
}

func TestRetriesOnRateLimit(t *testing.T) {
	calls := 0
	var waits []time.Duration
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		writeJSON(w, `{"id":"1","title":"ok"}`)
	})
	c.sleep = func(d time.Duration) { waits = append(waits, d) }

	page, err := c.GetPage(context.Background(), "1")
	if err != nil {
		t.Fatal(err)
	}
	if page.Title != "ok" {
		t.Errorf("unexpected page %+v", page)
	}
	if len(waits) != 2 || waits[0] != 2*time.Second {
		t.Errorf("unexpected waits %v", waits)
	}
}

func TestGivesUpAfterMaxRetries(t *testing.T) {
	calls := 0
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusTooManyRequests)
	})
	if _, err := c.GetPage(context.Background(), "1"); err == nil {
		t.Error("expected an error")
	}
	if calls != maxRetries+1 {
		t.Errorf("expected %d attempts, got %d", maxRetries+1, calls)
	}
}

func TestRetryAfter(t *testing.T) {
	if got := retryAfter("5", 0); got != 5*time.Second {
		t.Errorf("got %v, want 5s", got)
	}
	if got := retryAfter("", 2); got != 4*time.Second {
		t.Errorf("got %v, want 4s", got)
	}
	if got := retryAfter("Wed, 21 Oct 2015 07:28:00 GMT", 0); got != time.Second {
		t.Errorf("got %v, want 1s fallback", got)
	}
}
