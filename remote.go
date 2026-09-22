// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/aqueeb/confluence2md/confluence"
	"github.com/aqueeb/confluence2md/converter"
)

// maxFileNameLength keeps generated names comfortably under filesystem
// limits once the .md extension and a page ID suffix are added.
const maxFileNameLength = 100

// pullFromConfluence fetches the page at cfg.pageURL (and optionally
// everything under it) and writes each page out as Markdown.
func pullFromConfluence(ctx context.Context, cfg *config) error {
	baseURL, ref, err := confluence.ParsePageURL(cfg.pageURL)
	if err != nil {
		return err
	}

	client := confluence.NewClient(baseURL, cfg.token, cfg.user)
	id, err := client.ResolveID(ctx, ref)
	if err != nil {
		return err
	}

	p := &puller{
		client:   client,
		verbose:  cfg.verbose,
		dryRun:   cfg.dryRun,
		maxDepth: cfg.depth,
		used:     make(map[string]bool),
	}

	if !cfg.recursive {
		page, err := client.GetPage(ctx, id)
		if err != nil {
			return err
		}
		out := cfg.outputPath
		if out == "" {
			out = filepath.Join(cfg.outDir, pageFileName(page.Title, page.ID)+".md")
		}
		return p.writePage(page, out)
	}

	root, err := client.GetPage(ctx, id)
	if err != nil {
		return err
	}
	p.pullTree(ctx, root, cfg.outDir, 0)
	if err := ctx.Err(); err != nil {
		return err
	}

	fmt.Printf("\nConverted %d/%d pages\n", p.converted, p.seen)
	if p.converted < p.seen {
		return fmt.Errorf("%d page(s) failed, see warnings above", p.seen-p.converted)
	}
	return nil
}

// puller walks a page tree and writes it to disk.
type puller struct {
	client   *confluence.Client
	verbose  bool
	dryRun   bool
	maxDepth int // 0 means no limit

	// used tracks output paths already written in this run, so two pages
	// whose titles sanitize to the same name don't overwrite each other.
	used map[string]bool

	seen      int
	converted int
}

// pullTree writes page into dir, then recurses into its children. A page
// with children gets a directory of the same name next to its .md file:
//
//	Parent.md
//	Parent/Child.md
//	Parent/Child/Grandchild.md
//
// Failures on individual pages are reported and skipped so one bad page
// doesn't stop the whole export.
func (p *puller) pullTree(ctx context.Context, page *confluence.Page, dir string, depth int) {
	name := p.uniqueName(dir, page)
	if err := p.writePage(page, filepath.Join(dir, name+".md")); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to convert %q: %v\n", page.Title, err)
	}

	if p.maxDepth > 0 && depth >= p.maxDepth {
		return
	}

	children, err := p.client.ChildPages(ctx, page.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: skipping children of %q: %v\n", page.Title, err)
		return
	}

	for _, child := range children {
		if ctx.Err() != nil {
			return
		}
		childPage, err := p.client.GetPage(ctx, child.ID)
		if err != nil {
			p.seen++
			fmt.Fprintf(os.Stderr, "Warning: failed to fetch %q: %v\n", child.Title, err)
			continue
		}
		p.pullTree(ctx, childPage, filepath.Join(dir, name), depth+1)
	}
}

// writePage converts a fetched page and writes it to outputPath.
func (p *puller) writePage(page *confluence.Page, outputPath string) error {
	p.seen++

	if p.dryRun {
		fmt.Printf("[dry-run] Would write: %s -> %s\n", page.Title, outputPath)
		p.converted++
		return nil
	}

	if p.verbose {
		fmt.Printf("Converting: %s (id %s) -> %s\n", page.Title, page.ID, outputPath)
	}

	markdown, err := converter.ConvertHTMLToMarkdown(page.HTML)
	if err != nil {
		return fmt.Errorf("failed to convert to Markdown: %w", err)
	}

	// export_view doesn't include the page title, so add it back as the H1
	if page.Title != "" {
		markdown = "# " + page.Title + "\n\n" + markdown
	}

	if dir := filepath.Dir(outputPath); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}
	}
	if err := os.WriteFile(outputPath, []byte(markdown), 0644); err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}

	if !p.verbose {
		fmt.Printf("Converted: %s -> %s\n", page.Title, outputPath)
	}
	p.converted++
	return nil
}

// uniqueName picks a file name for page within dir that hasn't been used yet
// in this run. Comparison is case-insensitive since macOS and Windows are.
func (p *puller) uniqueName(dir string, page *confluence.Page) string {
	name := pageFileName(page.Title, page.ID)
	key := strings.ToLower(filepath.Join(dir, name))
	if p.used[key] {
		name = name + "-" + page.ID
		key = strings.ToLower(filepath.Join(dir, name))
	}
	p.used[key] = true
	return name
}

// pageFileName turns a page title into something safe to use as a file or
// directory name on any OS. Spaces become dashes to match the naming used
// for converted .doc exports.
func pageFileName(title, id string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.TrimSpace(title) {
		switch {
		case strings.ContainsRune(`/\:*?"<>|+`, r), unicode.IsSpace(r), unicode.IsControl(r):
			if !lastDash {
				b.WriteRune('-')
				lastDash = true
			}
		default:
			b.WriteRune(r)
			lastDash = false
		}
	}

	name := strings.Trim(b.String(), "-.")
	if runes := []rune(name); len(runes) > maxFileNameLength {
		name = strings.TrimRight(string(runes[:maxFileNameLength]), "-.")
	}
	if name == "" {
		name = "page-" + id
	}
	return name
}
