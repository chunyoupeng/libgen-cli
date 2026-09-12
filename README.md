# libgen-cli

[![Go Version](https://img.shields.io/badge/go-1.20%2B-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/license-Apache%202.0-green.svg)](LICENSE)
[![Agent Ready](https://img.shields.io/badge/AI%20Agent-Native%20Skill-blueviolet.svg)](#ai-agent-skills--tooling-integration)

A resilient, high-concurrency CLI tool and reusable Go library designed for querying, resolving, and downloading resources from Library Genesis. Engineered for both **human terminal workflows** and **autonomous AI Agent toolkits**.

---

## Highlights

- **AI Agent Native**: Emits deterministic, structured stdout by default. No interactive prompts hijack the agent's context or stdin unless explicitly requested (`-t / --interactive`).
- **Concurrent Search Pipeline**: Replaces legacy sequential `1 + 2N` HTTP lookups with bounded concurrency worker pools, slashing multi-item queries from 15+ seconds down to ~1 second while preserving rank order.
- **Resilient Fallback Orchestrator**: Multi-tier mirror probing and deterministic gateway fallback ensure high availability even when community mirrors experience intermittent downtime.
- **Safe & Atomic Storage**: Employs atomic write-to-temp and rename mechanics to prevent truncated files on dropped connections, alongside strict filename sanitization and UTF-8 multi-byte protection.
- **IPFS & Gateway Routing**: Seamless fallback between direct mirror links and decentralized IPFS gateways.

---

## AI Agent Skills & Tooling Integration

Modern autonomous agents (Claude Code, Pi, OpenAI Swarm, AutoGPT, MCP servers) require CLI tools that are **predictable**, **non-blocking**, and **context-efficient**.

This repository ships with a ready-to-use Agent Skill manifest located at [`skills/libgen/SKILL.md`](skills/libgen/SKILL.md). You can drop it directly into your agent harness to enable self-driving search, link extraction, and downloading.

### 1. Zero-Friction Execution (Non-Interactive Default)
Traditional CLI tools with interactive selector prompts (`promptui` / `fzf`) hang headless LLM workers indefinitely. `libgen-cli` runs in **pure print mode** by default:

```bash
# Agent queries catalog without getting blocked on stdin
libgen search "distributed systems" -r 3
```

Standard Output stream provides dense, clean metadata:
```text
++ Searching for: distributed systems
--------------------------------------------------------------------------------
MD5: 5a8a1c93a0ef6a26df0f9cf8290bc5e8 Designing Data-Intensive Applications...
    ++ @author Martin Kleppmann           @year 2017  @size  12 MB  @type  pdf
--------------------------------------------------------------------------------
MD5: 29a8d9fcb0f021ad5e8841a0b67bb401 Distributed Systems: Principles and Paradigms...
    ++ @author Maarten van Steen          @year 2023  @size 8.5 MB  @type  pdf
```

### 2. Autonomous Action Chain (Search → Direct Link → Download)
Agents can compose simple, deterministic command pipelines:

```bash
# Step 1: Search query with filters
libgen search "kubernetes" -e pdf -r 1

# Step 2: Extract direct URL (no file I/O overhead)
libgen link 2501e060c27cadf959fc0ff4bfb9d55f

# Step 3: Fetch file to designated directory
libgen download 2501e060c27cadf959fc0ff4bfb9d55f -o ./downloads/
```

### 3. Agent Tool Manifest Example (JSON / Function Calling)
When exposing `libgen-cli` as a tool definition to an LLM harness (e.g., Anthropic Tool Use or Model Context Protocol):

```json
{
  "name": "search_books",
  "description": "Searches Library Genesis for academic books, papers, and manuals.",
  "parameters": {
    "type": "object",
    "properties": {
      "query": { "type": "string", "description": "Keywords or book title" },
      "extension": { "type": "string", "enum": ["pdf", "epub", "mobi"], "description": "Optional format filter" },
      "limit": { "type": "integer", "default": 5, "description": "Number of items to retrieve (1-100)" }
    },
    "required": ["query"]
  }
}
```

---

## Installation

### From Source (Go 1.20+)
```bash
go install github.com/chunyoupeng/libgen-cli@latest
```

### Local Build
```bash
git clone https://github.com/chunyoupeng/libgen-cli.git
cd libgen-cli
make build
# Binary is produced at ./libgen
```

---

## Command Reference

### `search`
Searches Library Genesis by keyword, ISBN, title, or author.

```bash
# Basic query
libgen search "clean code"

# Human Interactive Mode (renders interactive arrow-key selector)
libgen search "clean code" -t

# Filter by format and limit count
libgen search "quantum computing" -e "pdf,epub" -r 5

# Sort results (options: id, title, author, pub, year, lang, size, ext)
libgen search "compiler design" -s year --sort-asc=false

# Pin a verified search mirror
libgen search "linear algebra" -m libgen.li
```

### `download`
Downloads a resource using its known 32-character MD5 hash.

```bash
# Download by hash
libgen download 2F2DBA2A621B693BB95601C16ED680F8

# Output to specific folder
libgen download 2F2DBA2A621B693BB95601C16ED680F8 -o /data/library/

# Download via IPFS gateway
libgen download 2F2DBA2A621B693BB95601C16ED680F8 --ipfs-mirrors

# Batch download from pipe
cat hashes.txt | xargs libgen download
```

### `download-all`
Fetches all query matches sequentially without terminal progress tearing.

```bash
libgen download-all "rust programming" -r 5 -o ./rust_books/
```

### `link`
Resolves and outputs the resolved raw direct download URL without initiating the transfer. Ideal for passing URLs to `aria2c`, `wget`, or remote download workers.

```bash
libgen link 2F2DBA2A621B693BB95601C16ED680F8
```

### `status`
Runs asynchronous, non-blocking concurrent health probes against all configured search and download mirrors.

```bash
# Probe all mirrors concurrently (~1s turnaround)
libgen status

# Probe specific pool
libgen status -m search
libgen status -m download
```

---

## Architecture & Reliability Model

```
[User / AI Agent]
       │
       ▼
 ┌───────────┐      Concurrent Probing
 │ libgen-cli│ ───────────────────────────► [Live Search Mirrors]
 └─────┬─────┘                               (libgen.li / libgen.vg / etc.)
       │
       ▼
 ┌────────────────────────────────────────────────────────┐
 │ Search Pipeline                                        │
 │ 1. Parse index page -> Deduplicated MD5 hashes         │
 │ 2. Parallel Worker Pool (8 workers) -> json.php (f + e)│
 │ 3. Order-preserving assembly & Unicode sanitization   │
 └─────────────────────────┬──────────────────────────────┘
                           │
                           ▼
 ┌────────────────────────────────────────────────────────┐
 │ Safe Download Pipeline                                 │
 │ 1. Deterministic Multi-tier Fallback (HTTP / IPFS)     │
 │ 2. Streaming download -> <filename>.tmp                │
 │ 3. Integrity verification -> Atomic os.Rename          │
 └────────────────────────────────────────────────────────┘
```

---

## Disclaimer

This project is developed for educational and research purposes only. The maintainers take no responsibility for how this tool is utilized. Please comply with your local copyright laws and institutional regulations.

## License

Licensed under the Apache License, Version 2.0 (the "License"). See [LICENSE](LICENSE) for details.
