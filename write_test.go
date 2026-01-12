package main

import (
	"errors"
	"go/types"
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
