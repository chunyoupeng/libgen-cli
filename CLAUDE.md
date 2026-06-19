# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
make build        # go build -o libgen  → produces ./libgen
make install      # go install .        → installs binary as `libgen-cli` in $GOPATH/bin
make build-travis # cross-compile linux/macos/windows/freebsd into artifacts/
make test         # go test -v -race ./...  (also rm's stray `libgen` binaries left in package dirs)

go run . search kubernetes            # run without building during dev
go test -v -race ./libgen -run TestSearch   # run a single test
```

Note: the binary name the user types depends on how it was built — `libgen` (via `make build`) or `libgen-cli` (via `make install`). The Cobra `Use: "libgen"` field is only the help-text display name, not the real command name. Module path is `github.com/chunyoupeng/libgen-cli` (a fork of `ciehanski/libgen-cli`).

**Tests hit live Library Genesis mirrors over the network** — they are inherently flaky and will fail offline or when mirrors are down. This is expected, not a code regression.

## Architecture

Three layers, two packages:

- `main.go` (package `main`) — entry point. Does a connectivity check (pings Google) and **exits if offline**, then calls `libgen_cli.Execute()`. The compiled binary will not run without internet.
- `cmd/libgen-cli/` (package `libgen_cli`) — the Cobra CLI layer. `root.go` holds `rootCmd`; `Execute()` registers each subcommand via `AddCommand`, and Cobra routes `os.Args[1]` to the matching command's `Use` name. One file per subcommand (`search.go`, `download.go`, `download_all.go`, `dbdumps.go`, `link.go`, `status.go`). Each `Run` func parses flags + positional args, calls into `libgen`, and signals failure with `os.Exit` (Run has no return value). This layer is CLI-only — printing, prompts, exit codes.
- `libgen/` (package `libgen`) — the reusable library with no CLI concerns. All operations revolve around the `Book` struct (`api.go`), which is the data carrier threaded through the whole pipeline.

### The Book pipeline

`search` and `download` differ only in *how they obtain* `[]*Book`, then share the rest:

1. **Obtain books**: `Search()` (keyword → scrape `index.php` → `parseHashes` extracts MD5s) or `GetDetails()` (known MD5 hashes). `GetDetails` makes **two `json.php` calls per hash** — `object=f` for file info (size, ext, edition ID) then `object=e` for edition info (title, author, year) — then applies flag filters (author/extension/year/etc.).
2. **Resolve download URL**: `GetDownloadURL()` fills `book.DownloadURL`.
3. **Download**: `DownloadBook()` (or `DownloadBookIPFS()`) writes the file to disk.

### Mirrors are the fragile core

Library Genesis has **no stable API** — everything is HTML/JSON scraping of community mirrors that frequently go down or change markup. This is where breakage almost always originates.

- Mirror lists live in `mirrors.go`: `SearchMirrors`, `DownloadMirrors`, `UploadMirrors`, `DbdumpsMirrors` (each a `[]url.URL`).
- `FindWorkingMirror()` / `GetWorkingMirror()` (`api.go`) probe the list to pick a live host before any request.
- Download-link extraction relies on **regexes in `const.go`** (`libraryLolReg`, `libgenPMReg`, IPFS gateway regexes, `SearchMD5`, etc.) run against scraped pages. If downloads break, suspect mirror HTML drift or a dead host first.
- `GetDownloadURL()` (`download.go`) is a multi-tier fallback: it first tries the search mirror's `ads.php`/`get.php` link, then rotates through `library.lol` ↔ `libgen.pm` ↔ IPFS gateways with retries, because any given mirror may be unavailable.

When adding/debugging download support, the work is usually: update a mirror host in `mirrors.go`, adjust a regex in `const.go`, or add a new `get*URL` resolver function in `download.go` and wire it into the `GetDownloadURL` switch.
