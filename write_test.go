package main

import (
	"testing"

	"vimagination.zapto.org/gotypes"
)

func TestGetAllImports(t *testing.T) {
	pkg, err := gotypes.ParsePackage(".")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	imps := map[string]string{pkg.Path(): pkg.Name()}

	getAllImports(pkg.Imports(), imps)

	for path, name := range map[string]string{
		"vimagination.zapto.org/unsafe":       "main",
		"vimagination.zapto.org/httpreaderat": "httpreaderat",
		"archive/zip":                         "zip",
		"bytes":                               "bytes",
	} {
		if n := imps[path]; n != name {
			t.Errorf("expecting package %q to have name %q, got %q", path, name, n)
		}
	}
}
