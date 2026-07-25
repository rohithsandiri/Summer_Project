// tools/depscan/main.go
//
// CLI for the static dependency-graph scanner. Two modes:
//
//	go run ./tools/depscan                  regenerate the seed file
//	go run ./tools/depscan --check           verify it's already up to date (CI)
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"

	"github.com/rohithsandiri/Summer_Project/tools/depscan/depscan"
)

func main() {
	root := flag.String("root", ".", "repository root to scan (expects cmd/*/main.go under it)")
	out := flag.String("out", "internal/operator/dependency/generated_seed.go", "output path for the generated seed, relative to root")
	check := flag.Bool("check", false, "verify the output file matches a fresh scan instead of writing it; exits 1 on drift")
	flag.Parse()

	edges, err := depscan.ScanRoot(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "depscan:", err)
		os.Exit(1)
	}

	generated := depscan.Generate(edges)
	outPath := *out
	if !os.IsPathSeparator(outPath[0]) {
		outPath = *root + string(os.PathSeparator) + outPath
	}

	if *check {
		existing, err := os.ReadFile(outPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "depscan --check: reading %s: %v\n", outPath, err)
			os.Exit(1)
		}
		if !bytes.Equal(existing, generated) {
			fmt.Fprintf(os.Stderr, "depscan --check: %s is out of date with the current cmd/*/main.go sources.\n"+
				"Run `make depscan` and commit the result.\n", outPath)
			os.Exit(1)
		}
		fmt.Println("depscan --check: seed is up to date")
		return
	}

	if err := os.WriteFile(outPath, generated, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "depscan: writing %s: %v\n", outPath, err)
		os.Exit(1)
	}
	fmt.Printf("depscan: wrote %d edges to %s\n", len(edges), outPath)
}
