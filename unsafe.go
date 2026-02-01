package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	var (
		output, packageName string
		excludeComment      bool
	)

	flag.StringVar(&output, "o", "", "output file")
	flag.StringVar(&packageName, "p", "", "package name")
	flag.BoolVar(&excludeComment, "x", false, "don't include go:generate comment")

	flag.Parse()

	var args []string

	if !excludeComment {
		args = os.Args[1:]
	}

	b, err := newBuilder(filepath.Dir(output), args...)
	if err != nil {
		return err
	}

	f, err := os.Create(output)
	if err != nil {
		return err
	}

	return b.WriteType(f, packageName, flag.Args()...)
}
