package main

import (
	"go/ast"
	"go/token"
	"reflect"
	"testing"

	"vimagination.zapto.org/gotypes"
)

func TestGenImports(t *testing.T) {
	for n, test := range [...]struct {
		imports  []string
		expected *ast.GenDecl
	}{
		{
			[]string{"strings"},
			&ast.GenDecl{
				Tok: token.IMPORT,
				Specs: []ast.Spec{
					&ast.ImportSpec{
						Path: &ast.BasicLit{
							Kind:  token.STRING,
							Value: `"strings"`,
						},
					},
					&ast.ImportSpec{
						Path: &ast.BasicLit{
							Kind:  token.STRING,
							Value: `"unsafe"`,
						},
					},
				},
			},
		},
		{
			[]string{"strings", "vimagination.zapto.org/httpreaderat"},
			&ast.GenDecl{
				Tok: token.IMPORT,
				Specs: []ast.Spec{
					&ast.ImportSpec{
						Path: &ast.BasicLit{
							Kind:  token.STRING,
							Value: `"strings"`,
						},
					},
					&ast.ImportSpec{
						Path: &ast.BasicLit{
							Kind:  token.STRING,
							Value: `"unsafe"`,
						},
					},
					&ast.ImportSpec{
						Path: &ast.BasicLit{
							ValuePos: 3,
							Kind:     token.STRING,
							Value:    `"vimagination.zapto.org/httpreaderat"`,
						},
					},
				},
			},
		},
		{
			[]string{"strings", "vimagination.zapto.org/httpreaderat", "vimagination.zapto.org/cache", "io"},
			&ast.GenDecl{
				Tok: token.IMPORT,
				Specs: []ast.Spec{
					&ast.ImportSpec{
						Path: &ast.BasicLit{
							Kind:  token.STRING,
							Value: `"io"`,
						},
					},
					&ast.ImportSpec{
						Path: &ast.BasicLit{
							Kind:  token.STRING,
							Value: `"strings"`,
						},
					},
					&ast.ImportSpec{
						Path: &ast.BasicLit{
							Kind:  token.STRING,
							Value: `"unsafe"`,
						},
					},
					&ast.ImportSpec{
						Path: &ast.BasicLit{
							ValuePos: 3,
							Kind:     token.STRING,
							Value:    `"vimagination.zapto.org/cache"`,
						},
					},
					&ast.ImportSpec{
						Path: &ast.BasicLit{
							Kind:  token.STRING,
							Value: `"vimagination.zapto.org/httpreaderat"`,
						},
					},
				},
			},
		},
	} {
		b, err := newBuilder(".")
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		imps := gotypes.Imports(b.pkg)
		b.init()

		for _, imp := range test.imports {
			b.packageName(imps[imp])
		}

		if imp := b.genImports(); !reflect.DeepEqual(imp, test.expected) {
			t.Errorf("test %d: expecting imports %v, got %v", n+1, test.expected, imp)
		}
	}
}
