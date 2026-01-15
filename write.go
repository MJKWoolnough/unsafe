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
	"iter"
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

	return genAST(w, structs, packagename, typeNames)
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
			if named, ok := field.Type().(*types.Named); ok && named.Obj().Exported() {
				continue
			} else if err := processField(imps, structs, field.Type()); err != nil {
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

func genAST(w io.Writer, structs map[string]types.Object, packageName string, types []string) error {
	file := &ast.File{
		Name:  ast.NewIdent(packageName),
		Decls: append(append([]ast.Decl{determineImports(structs)}, determineStructs(structs)...), determineMethods(types)...),
	}
	fset := token.NewFileSet()

	return format.Node(w, fset, file)
}

func determineImports(structs map[string]types.Object) *ast.GenDecl {
	imports := map[string]struct{}{"unsafe": {}}

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
	return strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(name, "_", "__"), ".", "_"), "/", "_")
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
	named, isNamed := typ.(*types.Named)
	if isNamed && named.Obj().Exported() {
		return &ast.SelectorExpr{
			X:   ast.NewIdent(named.Obj().Pkg().Name()),
			Sel: ast.NewIdent(named.Obj().Name()),
		}
	}

	switch t := typ.Underlying().(type) {
	case *types.Pointer:
		return &ast.StarExpr{
			X: fieldToType(t.Elem()),
		}
	case *types.Map:
		return &ast.MapType{
			Key:   fieldToType(t.Key()),
			Value: fieldToType(t.Elem()),
		}
	case *types.Array:
		return &ast.ArrayType{
			Len: &ast.BasicLit{
				Value: strconv.FormatInt(t.Len(), 10),
			},
			Elt: fieldToType(t.Elem()),
		}
	case *types.Slice:
		return &ast.ArrayType{
			Elt: fieldToType(t.Elem()),
		}
	case *types.Struct:
		if isStructRecursive(t, map[*types.Struct]bool{t: true}) {
			return ast.NewIdent(typeName(named.Obj().Pkg().Path() + "." + named.Obj().Name()))
		}

		return &ast.StructType{
			Fields: &ast.FieldList{
				List: structFieldList(t),
			},
		}
	case *types.Basic:
		return ast.NewIdent(t.Name())
	}

	return nil
}

func isStructRecursive(str *types.Struct, found map[*types.Struct]bool) bool {
	for field := range str.Fields() {
		for str := range getStructsFromType(field.Type()) {
			if recursive, done := found[str]; recursive {
				return true
			} else if !done {
				found[str] = false

				if isStructRecursive(str, found) {
					return true
				}
			}
		}
	}

	return false
}

func getStructsFromType(typ types.Type) iter.Seq[*types.Struct] {
	return func(yield func(*types.Struct) bool) {
		var elem types.Type

		switch t := typ.Underlying().(type) {
		case *types.Struct:
			yield(t)

			return
		case *types.Pointer:
			elem = t.Elem()
		case *types.Map:
			for str := range getStructsFromType(t.Key()) {
				if !yield(str) {
					return
				}
			}

			elem = t.Elem()
		case *types.Array:
			elem = t.Elem()
		case *types.Slice:
			elem = t.Elem()
		default:
			return
		}

		getStructsFromType(elem)(yield)
	}
}

func determineMethods(types []string) []ast.Decl {
	var decls []ast.Decl

	for _, typ := range types {
		decls = append(decls, buildFunc(typ))
	}

	return decls
}

func buildFunc(typ string) *ast.FuncDecl {
	return &ast.FuncDecl{
		Name: ast.NewIdent("make" + typeName(typ)),
		Type: &ast.FuncType{
			Params: &ast.FieldList{
				List: []*ast.Field{
					{
						Names: []*ast.Ident{ast.NewIdent("x")},
						Type: &ast.StarExpr{
							X: ast.NewIdent(typ),
						},
					},
				},
			},
			Results: &ast.FieldList{
				List: []*ast.Field{
					{
						Type: &ast.StarExpr{
							X: ast.NewIdent(typeName(typ)),
						},
					},
				},
			},
		},
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.ReturnStmt{
					Results: []ast.Expr{
						&ast.CallExpr{
							Fun: &ast.ParenExpr{
								X: &ast.StarExpr{
									X: ast.NewIdent(typeName(typ)),
								},
							},
							Args: []ast.Expr{
								&ast.CallExpr{
									Fun: &ast.SelectorExpr{
										X:   ast.NewIdent("unsafe"),
										Sel: ast.NewIdent("Pointer"),
									},
									Args: []ast.Expr{
										ast.NewIdent("x"),
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

var (
	ErrNoModuleType = errors.New("module-less type")
	ErrNoModule     = errors.New("module not imported")
	ErrNoType       = errors.New("no type found")
	ErrNoTypes      = errors.New("no non-internal types found")
)
