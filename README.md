<p align="center">
  <img src="logo.png" alt="confluence2md logo" width="256">
</p>

<h1 align="center">confluence2md</h1>

<p align="center">
  <a href="https://goreportcard.com/report/github.com/aqueeb/confluence2md"><img src="https://goreportcard.com/badge/github.com/aqueeb/confluence2md?v=2" alt="Go Report Card"></a>
  <a href="https://codecov.io/gh/aqueeb/confluence2md"><img src="https://codecov.io/gh/aqueeb/confluence2md/branch/main/graph/badge.svg?token=unused" alt="Coverage"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-Apache%202.0-blue.svg" alt="License: Apache 2.0"></a>
  <a href="https://github.com/aqueeb/confluence2md/releases"><img src="https://img.shields.io/github/v/release/aqueeb/confluence2md" alt="Release"></a>
  <a href="https://buymeacoffee.com/aqueeb"><img src="https://img.shields.io/badge/Buy%20Me%20A%20Coffee-support-yellow?logo=buymeacoffee" alt="Buy Me A Coffee"></a>
</p>

A CLI tool to convert Confluence MIME-encoded `.doc` exports to clean Markdown.

## The Problem

Confluence's "Export to Word" feature doesn't create real Word documents—it creates **MIME-encoded HTML files** with a `.doc` extension. Only Microsoft Word can open them. This has been [a known issue for over 10 years](https://community.atlassian.com/forums/Confluence-questions/Why-is-confluence-cloud-s-export-to-Word-feature-creating-an/qaq-p/2325894).

**What doesn't work:**
- LibreOffice, Google Docs, and other word processors
- Programmatic document parsers (python-docx, mammoth, etc.)
- Any tool expecting a real `.doc` or `.docx` file

**Why this matters:**
You can't convert Confluence exports to Markdown for version control, static site generators, or LLM/RAG pipelines—until now.

## Features

- **Zero dependencies** - release binaries include embedded pandoc
- **LLM/RAG-ready output** - clean Markdown optimized for chunking and embedding
- Parses MIME-encoded Confluence exports (not binary `.doc` files)
- Uses pandoc for high-quality HTML-to-Markdown conversion
- Cleans up Confluence-specific HTML artifacts
- Converts emoji images to Unicode (✅ ❌ 🚧 ⚠️)
- Converts info/tip/warning boxes to blockquotes
- Handles collapsible sections, code blocks, and tables
- Batch convert entire directories
- Pull pages straight from Confluence with a URL and token, including whole page trees

## Use Cases

- **Migrate to Git-based docs** — Move Confluence content to GitBook, Docusaurus, MkDocs, or any static site generator
- **Build RAG/LLM knowledge bases** — Feed your Confluence docs to LangChain, LlamaIndex, or custom embedding pipelines
- **Create portable backups** — Store documentation in a format that doesn't require Confluence or MS Word to read
- **Power AI coding assistants** — Add your team's documentation context to Copilot, Cursor, or Claude

## Installation

### From releases (recommended)

Download the binary for your platform from [Releases](https://github.com/aqueeb/confluence2md/releases). Release binaries include an embedded pandoc, so there are **no external dependencies**.

> [!IMPORTANT]
> **macOS users:** If you see "Apple could not verify" warning, either:
> - Run `xattr -d com.apple.quarantine /path/to/confluence2md` in Terminal, or
> - Go to **System Settings → Privacy & Security** and click "Open Anyway"

### From source

```bash
go install github.com/aqueeb/confluence2md@latest
```

> **Note:** Building from source requires [pandoc](https://pandoc.org/installing.html) to be installed on your system.

## Usage

```bash
# Convert a single file
confluence2md document.doc

# Convert with custom output path
confluence2md -o output.md document.doc

# Convert all .doc files in a directory
confluence2md --dir /path/to/docs

# Preview what would be converted (dry run)
confluence2md --dir /path/to/docs --dry-run

# Verbose output
confluence2md -v document.doc
```

### Fetching pages directly from Confluence

Instead of exporting each page to Word by hand, you can give confluence2md a page URL and an access token. It downloads the page (and, if you want, every page below it) and converts it in one step.

**1. Create a token**

| Your Confluence | Create this | Then set |
|-----------------|-------------|----------|
| Server / Data Center | A [Personal Access Token](https://confluence.atlassian.com/enterprise/using-personal-access-tokens-1026032365.html) (your avatar → Settings → Personal Access Tokens) | `CONFLUENCE_TOKEN` |
| Cloud (`*.atlassian.net`) | An [API token](https://id.atlassian.com/manage-profile/security/api-tokens) | `CONFLUENCE_TOKEN` **and** `CONFLUENCE_USER` (your Atlassian account email) |

confluence2md only reads from Confluence, so a read-only token is fine.

**2. Put the token in your environment**

```bash
export CONFLUENCE_TOKEN=your-token
export CONFLUENCE_USER=you@example.com   # Cloud only
```

`--token` and `--user` work too, but environment variables keep the token out of your shell history.

**3. Copy the page URL from your browser and run**

Put the URL in quotes. Characters like `?` and `&` confuse the shell otherwise.

```bash
# A single page, saved as ./Page-Title.md
confluence2md --url "https://example.atlassian.net/wiki/spaces/ENG/pages/12345/Team+Home"

# A single page, saved to a file you choose
confluence2md --url "<page url>" -o team-home.md

# A page and every page below it, saved under ./docs
confluence2md --url "<page url>" --recursive --out-dir ./docs

# Only the page and its direct children
confluence2md --url "<page url>" --recursive --depth 1 --out-dir ./docs

# List what would be written without writing any files
confluence2md --url "<page url>" --recursive --dry-run
```

#### Supported URLs

| Format | Example |
|--------|---------|
| Cloud / newer Data Center | `https://example.atlassian.net/wiki/spaces/ENG/pages/12345/Team+Home` |
| Server / Data Center page ID | `https://confluence.example.com/pages/viewpage.action?pageId=12345` |
| Server / Data Center title link | `https://confluence.example.com/display/ENG/Team+Home` |

Confluence installed under a sub-path (such as `https://intranet.example.com/confluence/...`) works as well. Short links like `/wiki/x/AbCd` aren't supported; open the page and copy the full URL from the address bar.

#### What you get

Each page becomes one Markdown file that starts with the page title as a `#` heading. With `--recursive`, the folders mirror the page tree in Confluence: a page with children gets a folder of the same name next to its `.md` file.

```
docs/
├── Team-Home.md
└── Team-Home/
    ├── Getting-Started.md
    ├── Getting-Started/
    │   └── Setup.md
    └── API-Reference.md
```

- File names come from page titles. Spaces and characters that aren't allowed in file names (`/ \ : * ? " < > | +`) are replaced with `-`.
- If two sibling pages end up with the same file name, the second one gets its page ID added (`A-B-67890.md`).
- If a page in the tree fails, confluence2md prints a warning, carries on with the rest, and exits with an error at the end so scripts can tell.
- Images and attachments aren't downloaded. Image links still point at your Confluence server.

Pages are fetched in Confluence's `export_view` format. That's the same HTML the Word export uses, so the Markdown matches what you'd get from converting an exported `.doc` file.

#### Flags for fetching

| Flag | Description |
|------|-------------|
| `--url <url>` | Page to fetch, copied from your browser |
| `--token <token>` | Personal Access Token (Server/DC) or API token (Cloud). Defaults to `$CONFLUENCE_TOKEN` |
| `--user <email>` | Your Atlassian account email. **Cloud only.** Defaults to `$CONFLUENCE_USER` |
| `-r, --recursive` | Also fetch every page below the given page |
| `--depth <n>` | With `--recursive`, how many levels below the page to fetch. `0` (the default) means no limit |
| `--out-dir <dir>` | Folder to write pages into. Defaults to the current directory |
| `-o, --output <file>` | Save a single page (no `--recursive`) to this file instead of `<out-dir>/<Page-Title>.md` |

`--verbose` and `--dry-run` work the same as for exported files. `--url` can't be combined with `--dir` or an input file.

#### Troubleshooting

| Error | What it usually means |
|-------|-----------------------|
| `401 unauthorized` | The token is wrong or expired. On Cloud, check that `CONFLUENCE_USER` is set to the email that owns the token. |
| `403 forbidden` | The token works, but your account can't view that page. |
| `page not found or not visible with this token` | Either the URL is wrong, or you don't have permission. Confluence answers "not found" in both cases. |
| `expected JSON but got "text/html"` | The request was redirected to a login page, usually an SSO proxy in front of Confluence. Ask your admin whether token access is allowed through it. |
| `could not find a page ID in ...` | The URL isn't one of the supported formats above. |

## Flags

| Flag | Description |
|------|-------------|
| `-o, --output` | Output file path (default: input with `.md` extension) |
| `--dir` | Convert all `.doc` files in directory |
| `-v, --verbose` | Show detailed processing info |
| `--dry-run` | Show what would be converted without writing |
| `--version` | Show version |

## What it converts

This tool specifically handles **Confluence MIME exports** - files that look like `.doc` but are actually MIME-encoded HTML. These are created when exporting pages from Confluence to Word format.

It does **not** handle:
- Binary Microsoft Word `.doc` files
- `.docx` files (use pandoc directly for these)

## How it works

1. **MIME parsing**: Extracts HTML content from the multipart MIME message
2. **Pandoc conversion**: Converts HTML to GitHub-flavored Markdown
3. **Post-processing**: Cleans up Confluence-specific artifacts:
   - Removes wrapper divs (`Section1`, `toc-macro`)
   - Converts info boxes to blockquotes (`> **Tip:**`, `> **Note:**`)
   - Replaces emoji images with Unicode characters
   - Fixes code block language hints
   - Balances orphaned HTML tags

## Support

If this tool saved you time, consider buying me a coffee:

[!["Buy Me A Coffee"](https://www.buymeacoffee.com/assets/img/custom_images/orange_img.png)](https://www.buymeacoffee.com/aqueeb)

Or just star the repo — it helps others discover this tool!

## Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

confluence2md is licensed under the [Apache License 2.0](LICENSE).

### Third-Party Components

This software bundles [Pandoc](https://pandoc.org/), a universal document
converter licensed under the GNU General Public License v2.0 or later.
Pandoc is distributed as a separate executable and invoked via process
execution (not linked).

For complete third-party licensing information, see
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
