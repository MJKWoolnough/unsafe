package main

import (
	"flag"
	"fmt"
	"os"

	"vimagination.zapto.org/gotypes"
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

	pkg, err := gotypes.ParsePackage(module)
	if err != nil {
		return err
	}

	f, err := os.Create(output)
	if err != nil {
		return err
	}

	return WriteType(f, pkg, packageName, flag.Args()...)
}
