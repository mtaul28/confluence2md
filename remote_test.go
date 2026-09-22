// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/aqueeb/confluence2md/converter"
)

type fakePage struct {
	title    string
	body     string
	children []string
}

// fakeConfluence serves a small page tree over the v1 REST API:
//
//	1 Team Home
//	├── 2 Getting Started
//	│   └── 4 Setup
//	├── 3 API: v2/beta
//	├── 5 A/B          (sanitizes to the same name as 6)
//	└── 6 A:B
func fakeConfluence(t *testing.T) *httptest.Server {
	t.Helper()
	pages := map[string]fakePage{
		"1": {title: "Team Home", body: "<p>Welcome</p>", children: []string{"2", "3", "5", "6"}},
		"2": {title: "Getting Started", body: "<h2>Intro</h2><p>Read this first.</p>", children: []string{"4"}},
		"3": {title: "API: v2/beta", body: "<ul><li>one</li><li>two</li></ul>"},
		"4": {title: "Setup", body: "<p>Install things.</p>"},
		"5": {title: "A/B", body: "<p>slash</p>"},
		"6": {title: "A:B", body: "<p>colon</p>"},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		rest := strings.TrimPrefix(r.URL.Path, "/wiki/rest/api/content/")
		parts := strings.Split(rest, "/")
		page, ok := pages[parts[0]]
		if !ok {
			http.NotFound(w, r)
			return
		}

		if len(parts) == 3 && parts[1] == "child" && parts[2] == "page" {
			var results []map[string]string
			for _, id := range page.children {
				results = append(results, map[string]string{"id": id, "title": pages[id].title})
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"results": results})
			return
		}

		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":    parts[0],
			"title": page.title,
			"space": map[string]string{"key": "ENG"},
			"body":  map[string]interface{}{"export_view": map[string]string{"value": page.body}},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func pageURL(srv *httptest.Server, id string) string {
	return srv.URL + "/wiki/spaces/ENG/pages/" + id + "/whatever"
}

// listFiles returns every file under dir as a sorted, slash-separated list.
func listFiles(t *testing.T, dir string) []string {
	t.Helper()
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			rel, _ := filepath.Rel(dir, path)
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	return files
}

func quietly(t *testing.T) {
	t.Helper()
	oldOut, oldErr := os.Stdout, os.Stderr
	devNull, _ := os.Open(os.DevNull)
	os.Stdout, os.Stderr = devNull, devNull
	t.Cleanup(func() {
		os.Stdout, os.Stderr = oldOut, oldErr
		_ = devNull.Close()
	})
}

func skipWithoutPandoc(t *testing.T) {
	t.Helper()
	if err := converter.CheckPandoc(); err != nil {
		t.Skipf("Pandoc not available: %v", err)
	}
}

func TestPullFromConfluence_SinglePage(t *testing.T) {
	skipWithoutPandoc(t)
	quietly(t)
	srv := fakeConfluence(t)
	outDir := t.TempDir()

	cfg := &config{pageURL: pageURL(srv, "2"), outDir: outDir}
	if err := pullFromConfluence(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}

	got := listFiles(t, outDir)
	if len(got) != 1 || got[0] != "Getting-Started.md" {
		t.Fatalf("unexpected files: %v", got)
	}

	md, _ := os.ReadFile(filepath.Join(outDir, "Getting-Started.md"))
	if !strings.HasPrefix(string(md), "# Getting Started\n") {
		t.Errorf("expected title heading, got:\n%s", md)
	}
	if !strings.Contains(string(md), "Read this first.") {
		t.Errorf("expected body content, got:\n%s", md)
	}
}

func TestPullFromConfluence_CustomOutput(t *testing.T) {
	skipWithoutPandoc(t)
	quietly(t)
	srv := fakeConfluence(t)
	out := filepath.Join(t.TempDir(), "nested", "custom.md")

	cfg := &config{pageURL: pageURL(srv, "4"), outputPath: out, outDir: "."}
	if err := pullFromConfluence(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Errorf("expected %s to exist: %v", out, err)
	}
}

func TestPullFromConfluence_Tree(t *testing.T) {
	skipWithoutPandoc(t)
	quietly(t)
	srv := fakeConfluence(t)
	outDir := t.TempDir()

	cfg := &config{pageURL: pageURL(srv, "1"), recursive: true, outDir: outDir}
	if err := pullFromConfluence(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"Team-Home.md",
		"Team-Home/A-B-6.md",
		"Team-Home/A-B.md",
		"Team-Home/API-v2-beta.md",
		"Team-Home/Getting-Started.md",
		"Team-Home/Getting-Started/Setup.md",
	}
	got := listFiles(t, outDir)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("files = %v\nwant %v", got, want)
	}
}

func TestPullFromConfluence_Depth(t *testing.T) {
	skipWithoutPandoc(t)
	quietly(t)
	srv := fakeConfluence(t)
	outDir := t.TempDir()

	cfg := &config{pageURL: pageURL(srv, "1"), recursive: true, depth: 1, outDir: outDir}
	if err := pullFromConfluence(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}

	for _, f := range listFiles(t, outDir) {
		if strings.Count(f, "/") > 1 {
			t.Errorf("--depth 1 should not fetch grandchildren, got %s", f)
		}
	}
	if _, err := os.Stat(filepath.Join(outDir, "Team-Home", "Getting-Started.md")); err != nil {
		t.Errorf("expected direct children to be fetched: %v", err)
	}
}

func TestPullFromConfluence_DryRun(t *testing.T) {
	srv := fakeConfluence(t)
	outDir := t.TempDir()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cfg := &config{pageURL: pageURL(srv, "1"), recursive: true, dryRun: true, outDir: outDir}
	err := pullFromConfluence(context.Background(), cfg)

	_ = w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)

	if err != nil {
		t.Fatal(err)
	}
	if files := listFiles(t, outDir); len(files) != 0 {
		t.Errorf("dry run wrote files: %v", files)
	}
	if !strings.Contains(string(out), "[dry-run] Would write: Setup") {
		t.Errorf("expected dry-run output for nested page, got:\n%s", out)
	}
	if !strings.Contains(string(out), "Converted 6/6 pages") {
		t.Errorf("expected summary line, got:\n%s", out)
	}
}

func TestPullFromConfluence_MissingPage(t *testing.T) {
	srv := fakeConfluence(t)
	cfg := &config{pageURL: pageURL(srv, "999"), outDir: t.TempDir()}
	err := pullFromConfluence(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not found error, got %v", err)
	}
}

func TestPullFromConfluence_BadURL(t *testing.T) {
	cfg := &config{pageURL: "https://example.com/nothing/here", outDir: t.TempDir()}
	if err := pullFromConfluence(context.Background(), cfg); err == nil {
		t.Error("expected an error for a URL with no page ID")
	}
}

func TestPageFileName(t *testing.T) {
	tests := []struct {
		title string
		want  string
	}{
		{"Getting Started", "Getting-Started"},
		{"API: v2/beta", "API-v2-beta"},
		{"  lots   of   space  ", "lots-of-space"},
		{`what? "quotes" <and> |pipes|`, "what-quotes-and-pipes"},
		{"C++ Style Guide", "C-Style-Guide"},
		{"Ends with a dot.", "Ends-with-a-dot"},
		{"Café résumé", "Café-résumé"},
		{"???", "page-123"},
		{"", "page-123"},
		{strings.Repeat("a", 150), strings.Repeat("a", maxFileNameLength)},
	}
	for _, tt := range tests {
		if got := pageFileName(tt.title, "123"); got != tt.want {
			t.Errorf("pageFileName(%q) = %q, want %q", tt.title, got, tt.want)
		}
	}
}

func TestParseFlags_Confluence(t *testing.T) {
	t.Setenv("CONFLUENCE_TOKEN", "from-env")
	t.Setenv("CONFLUENCE_USER", "")

	cfg, err := parseFlags([]string{"--url", "https://x/pages/viewpage.action?pageId=1", "-r", "--depth", "2", "--out-dir", "docs"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.recursive || cfg.depth != 2 || cfg.outDir != "docs" {
		t.Errorf("unexpected config: %+v", cfg)
	}
	if cfg.token != "from-env" {
		t.Errorf("expected token from env, got %q", cfg.token)
	}

	cfg, err = parseFlags([]string{"--url", "u", "--token", "from-flag", "--user", "me@example.com"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.token != "from-flag" || cfg.user != "me@example.com" {
		t.Errorf("flags should win over env: %+v", cfg)
	}
}

func TestParseFlags_ConfluenceConflicts(t *testing.T) {
	bad := [][]string{
		{"--recursive", "input.doc"},
		{"--url", "u", "--dir", "docs"},
		{"--url", "u", "input.doc"},
		{"--url", "u", "-r", "-o", "out.md"},
		{"--url", "u", "--depth", "-1"},
	}
	for _, args := range bad {
		if _, err := parseFlags(args, io.Discard); err == nil {
			t.Errorf("parseFlags(%v) expected an error", args)
		}
	}
}
