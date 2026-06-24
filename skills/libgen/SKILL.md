---
name: libgen
description: Search Library Genesis and download books, ebooks, papers, or PDFs via the libgen-cli tool. Use whenever the user wants to find, look up, search for, or download a book / ebook / textbook / paper / PDF.
---

# libgen — search & download from Library Genesis

Drive the `libgen-cli` command-line tool to search Library Genesis and download
books on the user's behalf. The flow is always the same: **bootstrap → search →
parse MD5 → download by MD5**.

## 1. Bootstrap (do this first, before any search)

Make sure the binary is available. Run:

```bash
command -v libgen-cli || command -v libgen
```

If a path prints, use that command name for everything below (prefer `libgen-cli`).

If nothing prints, install it with `go install`:

```bash
go install github.com/chunyoupeng/libgen-cli@latest
export PATH="$(go env GOPATH)/bin:$PATH"   # GOPATH defaults to ~/go
command -v libgen-cli
```

Fallback if `go install` fails (no Go toolchain, or module resolution error):

```bash
git clone https://github.com/chunyoupeng/libgen-cli /tmp/libgen-cli
cd /tmp/libgen-cli && make build      # produces ./libgen
```

then invoke it as `/tmp/libgen-cli/libgen`.

**The binary needs internet.** On startup it pings Google and exits immediately if
offline — so if it dies with no output, check connectivity first.

## 2. Search + filter

Non-interactive search prints results to stdout (it does NOT download):

```bash
libgen-cli search "<query>" -r <count> [filters]
```

Filter flags (same set for `download-all`):

| flag | meaning | example |
|------|---------|---------|
| `-r, --results` | max results (default 10) | `-r 5` |
| `-e, --extension` | file type; repeat for multiple | `-e pdf -e epub` |
| `-a, --require-author` | drop results with no author | `-a` |
| `-y, --year` | exact year | `-y 2020` |
| `-p, --publisher` | publisher substring | `-p "O'Reilly"` |
| `-l, --language` | language | `-l English` |
| `-s, --sort-by` | id, title, author, pub, year, lang, size, ext | `-s year` |
| `--sort-asc` | ascending (default true); `--sort-asc=false` for desc | `--sort-asc=false` |
| `-o, --output` | download directory | `-o ~/Books` |
| `-m, --mirror` | pin a specific search mirror by host (skip auto-select) | `-m libgen.li` |

Example:

```bash
libgen-cli search "deep learning" -r 5 -e pdf -y 2020 -l English -s year
```

**Search the title only.** Never put the author's name in the query string — filter
by author with `-a` (or `-p` for publisher) instead. Mixing the author into the title
usually returns nothing.

**Pinning a mirror** — `-m, --mirror <host>` is available on `search`, `download`,
`download-all`, and `link`. By default a random working mirror is chosen; pass `-m` to
force a specific one (it pins the **search** mirror, which is also the primary source for
download links). Useful when one mirror is flaky. Run `libgen-cli status -m search` first
to see which hosts are `[OK]`, then e.g. `-m libgen.li`. An unknown host errors out and
lists the valid hosts. The `library.lol`/`libgen.pm` download fallback stays automatic.

**Output format** — each result is two lines:

```
MD5: a1b2c3d4e5f6...<32 hex>  Some Book Title
    ++ @author Jane Doe  @year 2020  @size 12 MB  @type pdf
```

Parse the **MD5 with the regex `[a-f0-9]{32}`** (one per result line). Color is
auto-disabled when output is piped, so it's clean ASCII — no ANSI stripping needed.

Present results to the user as a numbered list (title / author / year / size / type).
Keep each row's MD5 internally so you can download the one they pick.

- `-t, --interactive` exists but is an arrow-key TUI menu for humans — **never use it
  from the agent**, it needs a real TTY.

## 3. Download

Once the user picks (or there's a single obvious match), download by MD5:

```bash
libgen-cli download <md5> -o <dir>
```

Just want the direct URL without downloading? Use:

```bash
libgen-cli link <md5>
```

Download every match for a query (use only when the user explicitly asks for "all"):

```bash
libgen-cli download-all "<query>" <filters> -o <dir>
```

Confirm the destination directory with the user before a bulk `download-all`.

## 4. Other commands

- `libgen-cli status [-m search|download]` — check which mirrors are alive (`[OK]`/`[FAIL]`).
- `libgen-cli dbdumps` — interactive DB-dump downloader (TUI; humans only, don't drive it).
- `libgen-cli version` — print version.

## 5. Troubleshooting

- **Mirrors are flaky — retry before giving up.** Up to ~3 attempts per book is reasonable
  before reporting failure to the user.
- **"Please provide a valid MD5 hash"** — you passed a non-MD5 (e.g. a numeric ID or
  title). Re-grab the 32-hex MD5 from the search output.
- **Search/download fails or times out** — Library Genesis mirrors are community-run and
  frequently go down. Retry, or run `libgen-cli status` to find a live mirror. Transient
  failure here is expected, not a bug.
- **Binary exits instantly with no output** — almost always offline; check the connection.
- **IPFS** — pass `-i` on `search`/`download`/`download-all`/`link` to use IPFS gateways
  instead of HTTP mirrors when normal mirrors are down.
