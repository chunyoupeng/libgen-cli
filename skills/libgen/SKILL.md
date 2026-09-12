---
name: libgen
description: Search Library Genesis and download books, textbooks, research papers, or PDFs via libgen-cli. Trigger whenever the user asks to find, search for, look up, or download any book, ebook, manual, or academic document.
---

# libgen — Search & Download from Library Genesis

Drive the `libgen-cli` tool to query Library Genesis and retrieve documents on the user's behalf.
The operational flow is strictly decoupled: **Bootstrap → Search & Parse → Select & Download**.

```
[Agent Action Pipeline]
   ┌─────────────┐       Title Only       ┌───────────────┐
   │ User Query  │ ─────────────────────► │ libgen search │
   └─────────────┘                        └───────┬───────┘
                                                  │ Clean stdout (MD5 + Meta)
                                                  ▼
   ┌─────────────┐       MD5 Hash         ┌───────────────┐
   │ File on Disk│ ◄───────────────────── │libgen download│
   └─────────────┘                        └───────────────┘
```

---

## 1. Bootstrap Check (Execute First)

Confirm the CLI executable exists in the local environment:

```bash
command -v libgen-cli || command -v libgen || echo "NOT_INSTALLED"
```

If neither is available, install via Go:

```bash
go install github.com/chunyoupeng/libgen-cli@latest
export PATH="$(go env GOPATH)/bin:$PATH"
```

Fallback build from source:

```bash
git clone https://github.com/chunyoupeng/libgen-cli /tmp/libgen-cli
cd /tmp/libgen-cli && make build
# Binary is located at /tmp/libgen-cli/libgen
```

> **Network Note**: The tool probes Google, Cloudflare, and primary mirrors upon start. If running behind an institutional firewall or proxy, ensure `HTTP_PROXY` / `HTTPS_PROXY` is exported.

---

## 2. Search & Metadata Extraction

`libgen search` runs in **non-blocking print mode** by default. It never opens a TUI selector unless `-t / --interactive` is passed. **Never pass `-t` in autonomous workflows.**

```bash
libgen-cli search "<title_query>" -r <limit> [flags]
```

### Search Flags Reference

| Flag | Description | Recommended Usage |
|------|-------------|-------------------|
| `-r, --results` | Result limit (1-100, default 10) | `-r 5` (keeps context compact) |
| `-e, --extension` | File format filter | `-e pdf` or `-e pdf,epub` (leading dots ignored) |
| `-a, --require-author` | Discard entries without listed author | `-a` (reduces noise) |
| `-y, --year` | Filter by publication year | `-y 2023` |
| `-p, --publisher` | Case-insensitive publisher substring | `-p "O'Reilly"` |
| `-l, --language` | Case-insensitive language filter | `-l English` |
| `-s, --sort-by` | Sort key: `id`, `title`, `author`, `year`, `size`, `ext` | `-s year` |
| `--sort-asc` | Sort direction (default `true`) | `--sort-asc=false` (newest first) |
| `-m, --mirror` | Pin search mirror host (bypasses auto-probe) | `-m libgen.li` |

### Critical Search Guidelines
1. **Query Title Only**: Never combine author names into the query string (e.g., `"Martin Kleppmann Designing Data-Intensive"` often yields 0 matches). Query `"Designing Data-Intensive"` and filter with `-a` or inspect the author in the output.
2. **Output Stream Structure**:
   ```text
   MD5: 5a8a1c93a0ef6a26df0f9cf8290bc5e8 Designing Data-Intensive Applications
       ++ @author Martin Kleppmann           @year 2017  @size  12 MB  @type  pdf
   ```
3. **Parse Regex**:
   - MD5: `(?i)\b[a-f0-9]{32}\b`
   - Capture the 32-character hexadecimal hash to drive subsequent download or link retrieval.

---

## 3. Link & Download Execution

### Option A: Retrieve Direct URL (Zero File I/O)
When the user only needs the download link, or to delegate the download to an external downloader (`curl`, `wget`, `aria2`):

```bash
libgen-cli link <md5>
```

### Option B: Download to Local Disk
Download by target MD5 hash:

```bash
libgen-cli download <md5> -o <destination_dir>
```

- **Atomic File Writes**: Downloads stream into a temporary `<file>.tmp` and perform an atomic `os.Rename` only after full transfer. Incomplete files are deleted automatically on interruption.
- **Path Sanitization**: Book titles with path delimiters (`/`, `\`, `:`) are converted to `_` automatically, ensuring safe flat storage without unintentional directory creation.

### Option C: Bulk Download
Download all matches from a search query (only when explicitly requested by user):

```bash
libgen-cli download-all "<query>" [flags] -o <destination_dir>
```

Downloads execute sequentially with isolated progress reporting to maintain clean terminal logs.

---

## 4. Self-Healing & Troubleshooting

Autonomous agents should apply this standard retry loop when encountering transient failures:

```
Search / Download Fails
         │
         ▼
Run `libgen-cli status -m search` (~1s concurrent check)
         │
         ├─► Find first host marked [OK] (e.g. libgen.li)
         │   Retry with: `libgen-cli search ... -m libgen.li`
         │
         └─► If HTTP mirrors fail, fallback to IPFS:
             Retry with: `libgen-cli download ... -i`
```

1. **"Please provide a valid MD5 hash"**: Verify the argument passed to `download` or `link` matches `^[a-fA-F0-9]{32}$`. Do not pass numeric IDs or titles.
2. **Connection Timeout**: Library Genesis community mirrors frequently rotate or suffer outages. Execute `libgen-cli status` to detect live nodes before retrying.
3. **IPFS Fallback**: Passing `-i / --ipfs-mirrors` routes downloads through decentralized IPFS gateways, bypassing blocked HTTP mirrors.
