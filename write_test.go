package main

import (
	"go/types"
	"testing"

	"vimagination.zapto.org/gotypes"
)

func TestGetAllImports(t *testing.T) {
	pkg, err := gotypes.ParsePackage(".")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	imps := map[string]*types.Package{pkg.Path(): pkg}

	getAllImports(pkg.Imports(), imps)

	for path, name := range map[string]string{
		"vimagination.zapto.org/unsafe":       "main",
		"vimagination.zapto.org/httpreaderat": "httpreaderat",
		"archive/zip":                         "zip",
		"bytes":                               "bytes",
	} {
		if n := imps[path].Name(); n != name {
			t.Errorf("expecting package %q to have name %q, got %q", path, name, n)
		}
	}
}
