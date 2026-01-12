// Package unsafe is a program that localises a type from another package.
package main

import (
	"cmp"
	"errors"
	"fmt"
	"go/types"
	"io"
	"strings"

	"vimagination.zapto.org/gotypes"
)

func WriteType(w io.Writer, pkg *types.Package, packagename string, typeNames ...string) error {
	imps := gotypes.Imports(pkg)
	structs := make(map[string]types.Object)

	for _, typename := range typeNames {
		if err := getAllStructs(imps, structs, typename); err != nil {
			return err
		}
	}

	return genAST(w, imps, structs, packagename)
}

func getAllStructs(imps map[string]*types.Package, structs map[string]types.Object, typename string) error {
	genPos := strings.IndexByte(typename, '[')
	if genPos == -1 {
		genPos = len(typename)
	}

	if _, ok := structs[typename[:genPos]]; ok {
		return nil
	}

	pos := strings.LastIndexByte(typename[:genPos], '.')
	if pos < 0 {
		return fmt.Errorf("%w: %s", ErrNoModuleType, typename)
	}

	pkg, ok := imps[typename[:pos]]
	if !ok {
		return fmt.Errorf("%w: %s", ErrNoModule, typename)
	}

	obj := pkg.Scope().Lookup(typename[pos+1 : genPos])
	if obj == nil {
		return fmt.Errorf("%w: %s", ErrNoType, typename)
	}

	if s, ok := obj.Type().Underlying().(*types.Struct); ok {
		structs[typename[:genPos]] = obj

		for field := range s.Fields() {
			if err := processField(imps, structs, field.Type()); err != nil {
				return err
			}
		}
	}

	return nil
}

func processField(imps map[string]*types.Package, structs map[string]types.Object, field types.Type) error {
	switch t := field.Underlying().(type) {
	case *types.Pointer:
		return processField(imps, structs, t.Elem())
	case *types.Map:
		return cmp.Or(processField(imps, structs, t.Key()), processField(imps, structs, t.Elem()))
	case *types.Array:
		return processField(imps, structs, t.Elem())
	case *types.Slice:
		return processField(imps, structs, t.Elem())
	case *types.Struct:
		return getAllStructs(imps, structs, field.String())
	}

	return nil
}

func genAST(w io.Writer, imps map[string]*types.Package, structs map[string]types.Object, packageName string) error {
	return nil
}

var (
	ErrNoModuleType = errors.New("module-less type")
	ErrNoModule     = errors.New("module not imported")
	ErrNoType       = errors.New("no type found")
)
