# gijq

Interactive jq explorer for the terminal. Type a filter, see results instantly.

![demo](demo.gif)

```
gijq data.json
```

```
cat data.json | gijq
```

## Features

- **Live filtering** -- results update as you type any valid jq expression
- **Tab completion** -- press Tab to cycle through available keys at the current path, works through nested objects, arrays, and pipe expressions
- **Split-pane layout** -- JSON output on the left, available keys on the right
- **Query history** -- per-file history accessible via Ctrl+H, persisted across sessions
- **Clipboard support** -- copy output or filter to clipboard, with OSC52 fallback for SSH sessions
- **Syntax highlighting** -- keys, strings, numbers, booleans, and nulls are color-coded
- **Scrollable output** -- arrow keys and page up/down for large results
- **Pipeline-friendly** -- press Enter to output the current result to stdout and exit

## Install

Requires Go 1.24+.

```
go install github.com/dayangraham/gijq@latest
```

Or install a prebuilt binary from GitHub Releases.

### From GitHub Releases (macOS/Linux)

Install latest:

```sh
curl -fsSL https://raw.githubusercontent.com/dgrah50/gijq/main/scripts/install.sh | sh
```

Install a specific version:

```sh
curl -fsSL https://raw.githubusercontent.com/dgrah50/gijq/main/scripts/install.sh | sh -s -- v0.0.1
```

### From GitHub Releases (Windows PowerShell)

```powershell
$repo = "dgrah50/gijq"
$latest = Invoke-RestMethod -Uri "https://api.github.com/repos/$repo/releases/latest"
$tag = $latest.tag_name
$asset = "gijq_${tag}_windows_amd64.exe.zip"
$url = "https://github.com/$repo/releases/download/$tag/$asset"
Invoke-WebRequest -Uri $url -OutFile $asset
Expand-Archive $asset -DestinationPath .
Move-Item "gijq_${tag}_windows_amd64.exe" "gijq.exe"
```

Or build from source:

```
git clone https://github.com/dayangraham/gijq.git
cd gijq
go build -o gijq .
```

## Releases

This repo includes a GitHub Actions release workflow at `.github/workflows/release.yml`.
It runs `go test ./...`, builds binaries for Linux/macOS/Windows, and uploads them to a GitHub Release when you push a semver tag (`vX.Y.Z`).

Create and push the next release tag with:

```sh
scripts/release.sh          # defaults to patch bump
scripts/release.sh minor    # bump minor
scripts/release.sh major    # bump major
```

After pushing the tag, check the Actions tab, then see assets on the release page.

## Usage

```sh
# From a file
gijq api-response.json

# From a pipe
curl -s https://api.example.com/data | gijq
```

Once inside, type any jq expression in the filter bar:

```
.users[] | select(.active) | .name
```

## Keybindings

| Key | Action |
|---|---|
| *type* | Filter updates, results refresh live |
| `Tab` | Cycle autocomplete suggestions |
| `Enter` | Output current result to stdout and exit |
| `Ctrl+Y` | Copy JSON output to clipboard |
| `Ctrl+F` | Copy filter to clipboard |
| `Ctrl+H` | Show query history overlay |
| `Up/Down` | Scroll output or navigate suggestions |
| `PgUp/PgDn` | Scroll output half-page |
| `Shift+Left/Right` | Horizontal scroll output |
| `Esc` | Close overlay, or exit |
| `Ctrl+C` | Exit |

## Performance Benchmarks

Generate deterministic large test files:

```sh
go run ./scripts/generate_benchdata.go -out testdata/bench -sizes 10,55,100
```

Run the reproducible benchmark suite (query execution + typing replay):

```sh
go test ./internal/perf -run '^$' -bench . -benchmem
```

Capture CPU and memory profiles for analysis:

```sh
go test ./internal/perf -run '^$' -bench BenchmarkTypingReplay/55MB -cpuprofile cpu.out -memprofile mem.out
go tool pprof -http=:0 cpu.out
```

Capture interactive keypress latency from real TUI sessions:

```sh
GIJQ_TELEMETRY=1 gijq testdata/bench/synthetic-55mb.json
```

At exit, `gijq` prints p50/p95/p99 keypress-to-frame timings to stderr.

## License

MIT
