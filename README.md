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

### Pulling pages directly from Confluence

Skip the manual "Export to Word" step and point it at a page URL instead:

```bash
export CONFLUENCE_TOKEN=your-token

# Fetch a single page
confluence2md --url "https://confluence.example.com/pages/viewpage.action?pageId=12345"

# Fetch a page and everything under it into ./docs
confluence2md --url "https://confluence.example.com/display/ENG/Team+Home" -r --out-dir ./docs

# Only go two levels deep, and preview first
confluence2md --url "<page url>" -r --depth 2 --dry-run
```

**Authentication**

- **Server / Data Center:** create a [Personal Access Token](https://confluence.atlassian.com/enterprise/using-personal-access-tokens-1026032365.html) and set `CONFLUENCE_TOKEN` (or pass `--token`). It's sent as a bearer token.
- **Cloud:** create an [API token](https://id.atlassian.com/manage-profile/security/api-tokens) and also set `CONFLUENCE_USER` (or `--user`) to your account email. Cloud API tokens are sent as basic auth.

Using the environment variables keeps the token out of your shell history.

**Supported URL formats**

- `https://<site>.atlassian.net/wiki/spaces/KEY/pages/12345/Title`
- `https://<host>/pages/viewpage.action?pageId=12345`
- `https://<host>/display/KEY/Page+Title`

Short links (`/wiki/x/AbCd`) aren't supported yet; open the page and copy the full URL instead.

**Output layout**

With `--recursive`, pages are written as a folder tree that mirrors Confluence. A page that has children gets a folder with the same name next to its `.md` file:

```
docs/
├── Team-Home.md
└── Team-Home/
    ├── Getting-Started.md
    ├── Getting-Started/
    │   └── Setup.md
    └── API-Reference.md
```

Pages are fetched using Confluence's `export_view` rendering, which is the same HTML used by the Word export, so the output matches what you'd get from converting a `.doc` export. Attachments and images aren't downloaded; image links will still point at your Confluence server.

## Flags

| Flag | Description |
|------|-------------|
| `-o, --output` | Output file path (default: input with `.md` extension) |
| `--dir` | Convert all `.doc` files in directory |
| `-v, --verbose` | Show detailed processing info |
| `--dry-run` | Show what would be converted without writing |
| `--version` | Show version |
| `--url` | Confluence page URL to fetch and convert |
| `--token` | Personal access token or API token (default: `$CONFLUENCE_TOKEN`) |
| `--user` | Account email, only needed for Confluence Cloud (default: `$CONFLUENCE_USER`) |
| `-r, --recursive` | With `--url`, also fetch every page under it |
| `--depth` | With `--recursive`, how many levels of children to fetch (`0` = all) |
| `--out-dir` | With `--url`, directory to write pages into (default: current directory) |

## What it converts

This tool specifically handles **Confluence MIME exports** - files that look like `.doc` but are actually MIME-encoded HTML. These are created when exporting pages from Confluence to Word format.

It does **not** handle:
- Binary Microsoft Word `.doc` files
- `.docx` files (use pandoc directly for these)

## How it works

1. **MIME parsing**: Extracts HTML content from the multipart MIME message (or, with `--url`, fetches the page's rendered HTML from the Confluence REST API)
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
