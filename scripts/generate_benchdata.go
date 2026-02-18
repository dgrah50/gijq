package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func main() {
	var (
		outDir = flag.String("out", "testdata/bench", "output directory")
		sizes  = flag.String("sizes", "10,55,100", "comma-separated sizes in MB")
		force  = flag.Bool("force", false, "overwrite existing files")
	)
	flag.Parse()

	sizeList, err := parseSizes(*sizes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "error creating %s: %v\n", *outDir, err)
		os.Exit(1)
	}

	for _, mb := range sizeList {
		filename := filepath.Join(*outDir, fmt.Sprintf("synthetic-%dmb.json", mb))
		if !*force {
			if _, err := os.Stat(filename); err == nil {
				fmt.Printf("skip %s (already exists)\n", filename)
				continue
			}
		}

		data := syntheticJSON(mb)
		if err := os.WriteFile(filename, data, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "error writing %s: %v\n", filename, err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s (%d bytes)\n", filename, len(data))
	}
}

func parseSizes(input string) ([]int, error) {
	parts := strings.Split(input, ",")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		v, err := strconv.Atoi(p)
		if err != nil || v <= 0 {
			return nil, fmt.Errorf("invalid size %q", p)
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no valid sizes provided")
	}
	return out, nil
}

func syntheticJSON(sizeMB int) []byte {
	targetBytes := sizeMB * 1024 * 1024
	if targetBytes < 1024 {
		targetBytes = 1024
	}

	payload := strings.Repeat("abcdefghijklmnopqrstuvwxyz0123456789", 4)
	statuses := []string{"ok", "warn", "error"}

	var b strings.Builder
	b.Grow(targetBytes + 4096)
	b.WriteString(`{"items":[`)

	for i := 0; b.Len() < targetBytes-1024; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(
			&b,
			`{"id":%d,"name":"item-%06d","status":"%s","metrics":{"count":%d,"ratio":%.3f},"tags":["alpha","beta","gamma"],"payload":"%s"}`,
			i,
			i,
			statuses[i%len(statuses)],
			i%1000,
			float64(i%1000)/1000.0,
			payload,
		)
	}

	b.WriteString(`],"meta":{"generated":true,"seed":"deterministic-v1"}}`)
	return []byte(b.String())
}
