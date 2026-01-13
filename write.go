// Package unsafe is a program that localises a type from another package.
package main

import (
	"cmp"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"go/types"
	"io"
	"maps"
	"slices"
	"strconv"
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

	if len(structs) == 0 {
		return ErrNoTypes
	}

	return genAST(w, structs, packagename)
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

	if strings.Contains(typename[:pos], "/internal/") || strings.HasSuffix(typename[:pos], "/internal") || strings.HasPrefix(typename, "internal/") {
		return nil
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

func genAST(w io.Writer, structs map[string]types.Object, packageName string) error {
	file := &ast.File{
		Name:  ast.NewIdent(packageName),
		Decls: append([]ast.Decl{determineImports(structs)}, determineStructs(structs)...),
	}
	fset := token.NewFileSet()

	return format.Node(w, fset, file)
}

func determineImports(structs map[string]types.Object) *ast.GenDecl {
	imports := make(map[string]struct{})

	for _, str := range structs {
		imports[str.Pkg().Path()] = struct{}{}
	}

	importPaths := slices.Collect(maps.Keys(imports))

	slices.Sort(importPaths)

	specs := make([]ast.Spec, len(importPaths))

	for n, ip := range importPaths {
		specs[n] = &ast.ImportSpec{
			Path: &ast.BasicLit{
				Value: strconv.Quote(ip),
			},
		}
	}

	return &ast.GenDecl{
		Tok:   token.IMPORT,
		Specs: specs,
	}
}

func determineStructs(structs map[string]types.Object) []ast.Decl {
	var decls []ast.Decl

	for name, obj := range structs {
		decls = append(decls, conStruct(name, obj.Type().Underlying().(*types.Struct)))
	}

	return decls
}

func conStruct(name string, str *types.Struct) *ast.GenDecl {
	return &ast.GenDecl{
		Tok: token.TYPE,
		Specs: []ast.Spec{
			&ast.TypeSpec{
				Name: ast.NewIdent(typeName(name)),
				Type: &ast.StructType{
					Fields: &ast.FieldList{
						List: structFieldList(str),
					},
				},
			},
		},
	}
}

func typeName(name string) string {
	return strings.ReplaceAll(strings.ReplaceAll(name, "·", "··"), ".", "·")
}

func structFieldList(str *types.Struct) []*ast.Field {
	var fields []*ast.Field

	for field := range str.Fields() {
		fields = append(fields, &ast.Field{
			Names: []*ast.Ident{ast.NewIdent(field.Name())},
			Type:  fieldToType(field.Type()),
		})
	}

	return fields
}

func fieldToType(typ types.Type) ast.Expr {
	return nil
}

var (
	ErrNoModuleType = errors.New("module-less type")
	ErrNoModule     = errors.New("module not imported")
	ErrNoType       = errors.New("no type found")
	ErrNoTypes      = errors.New("no non-internal types found")
)
