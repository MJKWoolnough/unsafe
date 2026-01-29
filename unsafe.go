package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	var module, output, packageName string

	flag.StringVar(&module, "-m", "", "path to module")
	flag.StringVar(&output, "-o", "", "output file")
	flag.StringVar(&packageName, "-p", "", "package name")

	flag.Parse()

	b, err := newBuilder(module)
	if err != nil {
		return err
	}

	f, err := os.Create(output)
	if err != nil {
		return err
	}

	return b.WriteType(f, packageName, flag.Args()...)
}
