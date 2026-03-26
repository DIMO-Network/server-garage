package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	schemaDir := flag.String("schema", "", "directory containing .graphqls files")
	prefix := flag.String("prefix", "", "tool name prefix")
	out := flag.String("out", "", "output Go file path")
	pkg := flag.String("package", "", "Go package name for generated file")
	flag.Parse()

	if *schemaDir == "" || *out == "" || *pkg == "" {
		fmt.Fprintln(os.Stderr, "usage: mcpgen -schema <dir> -prefix <prefix> -out <file> -package <pkg>")
		os.Exit(1)
	}

	// Collect .graphqls files
	entries, err := os.ReadDir(*schemaDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reading schema directory: %v\n", err)
		os.Exit(1)
	}

	var paths []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".graphqls" {
			paths = append(paths, filepath.Join(*schemaDir, e.Name()))
		}
	}

	if len(paths) == 0 {
		fmt.Fprintln(os.Stderr, "no .graphqls files found in", *schemaDir)
		os.Exit(1)
	}

	tools, err := parseSchema(paths, *prefix)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parsing schema: %v\n", err)
		os.Exit(1)
	}

	output, err := generateGoFile(*pkg, tools)
	if err != nil {
		fmt.Fprintf(os.Stderr, "generating output: %v\n", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "creating output directory: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(*out, []byte(output), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "writing output: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Generated %d tool(s) in %s\n", len(tools), *out)
}
