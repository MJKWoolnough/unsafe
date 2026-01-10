package main

import (
	"go/types"
	"io"
)

func WriteType(w io.Writer, pkg *types.Package, types ...string) error {
	return nil
}

func getAllImports(imports []*types.Package, imps map[string]string) {
	for _, imp := range imports {
		if _, ok := imps[imp.Path()]; ok {
			continue
		}

		imps[imp.Path()] = imp.Name()

		getAllImports(imp.Imports(), imps)
	}
}
