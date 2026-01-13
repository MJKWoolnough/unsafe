package main

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"reflect"
	"testing"

	"vimagination.zapto.org/gotypes"
)

func TestGetAllStructs(t *testing.T) {
	pkg, err := gotypes.ParsePackage(".")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	imps := gotypes.Imports(pkg)

	structs := make(map[string]types.Object)

	if err := getAllStructs(imps, structs, "Request"); !errors.Is(err, ErrNoModuleType) {
		t.Errorf("expected error %v, got %v", ErrNoModuleType, err)
	}

	if err := getAllStructs(imps, structs, "unknown.Request"); !errors.Is(err, ErrNoModule) {
		t.Errorf("expected error %v, got %v", ErrNoModule, err)
	}

	if err := getAllStructs(imps, structs, "vimagination.zapto.org/httpreaderat.Requested"); !errors.Is(err, ErrNoType) {
		t.Errorf("expected error %v, got %v", ErrNoType, err)
	}

	if err := getAllStructs(imps, structs, "internal/sync.Mutex"); err != nil {
		t.Errorf("unexpected error %v", err)
	} else if len(structs) > 0 {
		t.Errorf("expected no types, got %v", structs)
	}

	if err := getAllStructs(imps, structs, "vimagination.zapto.org/httpreaderat.Request"); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	for typeName, structStr := range map[string]string{
		"vimagination.zapto.org/httpreaderat.Request": "struct{url string; length int64; blockSize int64; cache *vimagination.zapto.org/cache.LRU[int64, string]}",
		"vimagination.zapto.org/cache.LRU":            "struct{vimagination.zapto.org/cache.cache[T, U, vimagination.zapto.org/cache.lru[T, U]]}",
		"sync.Mutex":                                  "struct{_ sync.noCopy; mu internal/sync.Mutex}",
	} {
		if s := structs[typeName].Type().Underlying().String(); s != structStr {
			t.Errorf("expecting struct %q to have type %q, got %q", typeName, structStr, s)
		}
	}
}

func TestDetermineImports(t *testing.T) {
	pkg, err := gotypes.ParsePackage(".")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	imps := gotypes.Imports(pkg)
	structs := make(map[string]types.Object)

	if err := getAllStructs(imps, structs, "strings.Reader"); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	expected := &ast.GenDecl{
		Tok: token.IMPORT,
		Specs: []ast.Spec{
			&ast.ImportSpec{
				Path: &ast.BasicLit{
					Value: `"strings"`,
				},
			},
		},
	}

	if imp := determineImports(structs); !reflect.DeepEqual(imp, expected) {
		fmt.Println(imp.Specs[0].(*ast.ImportSpec).Path.Value)
		t.Errorf("expecting imports %v, got %v", expected, imp)
	}
}

func TestTypeName(t *testing.T) {
	for n, test := range [...][2]string{
		{
			"A", "A",
		},
		{
			"a.A", "a·A",
		},
		{
			"a·A", "a··A",
		},
	} {
		if name := typeName(test[0]); name != test[1] {
			t.Errorf("test %d: expecting name %q, got %q", n+1, test[1], name)
		}
	}
}
