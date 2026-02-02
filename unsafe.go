package main

import (
	"errors"
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

	if output == "" {
		return ErrNoOutput
	}

	var args []string

	if !excludeComment {
		args = []string{"-o", filepath.Base(output)}

		if packageName != "" {
			args = append(args, "-p", packageName)
		}
	}

	absPath, err := filepath.Abs(filepath.Dir(output))
	if err != nil {
		return err
	}

	b, err := newBuilder(absPath, args...)
	if err != nil {
		return err
	}

	f := fileWriter{path: output}

	if err := b.WriteType(&f, packageName, flag.Args()...); err != nil {
		return err
	}

	return f.Close()
}

type fileWriter struct {
	path string
	*os.File
}

func (f *fileWriter) Write(p []byte) (int, error) {
	if f.File == nil {
		var err error

		f.File, err = os.Create(f.path)
		if err != nil {
			return 0, err
		}
	}

	return f.File.Write(p)
}

var ErrNoOutput = errors.New("no output file specified")
