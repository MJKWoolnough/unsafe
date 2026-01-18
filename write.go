// Package unsafe is a program that localises a type from another package.
package main

import (
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

type builder struct {
	mod     *gotypes.ModFile
	imports map[string]*types.Package
	structs map[string]ast.Decl
	pkg     *types.Package
	fset    *token.FileSet
}

func newBuilder(module string) (*builder, error) {
	pkg, err := gotypes.ParsePackage(module)
	if err != nil {
		return nil, err
	}

	mod, err := gotypes.ParseModFile(module)
	if err != nil {
		return nil, err
	}

	return &builder{
		mod:     mod,
		imports: make(map[string]*types.Package),
		structs: make(map[string]ast.Decl),
		pkg:     pkg,
		fset:    token.NewFileSet(),
	}, nil
}

func (b *builder) WriteType(w io.Writer, packagename string, typeNames ...string) error {
	file, err := b.genAST(packagename, typeNames)
	if err != nil {
		return err
	}

	return format.Node(w, b.fset, file)
}

func (b *builder) genAST(packageName string, types []string) (*ast.File, error) {
	imps := gotypes.Imports(b.pkg)

	for _, typeName := range types {
		str, err := b.getStruct(imps, typeName)
		if err != nil {
			return nil, err
		}

		b.structs[typeName] = b.conStruct(typeName, str)
	}

	return &ast.File{
		Name:  ast.NewIdent(packageName),
		Decls: append(append([]ast.Decl{b.genImports()}, slices.Collect(maps.Values(b.structs))...), determineMethods(types)...),
	}, nil
}

func (b *builder) getStruct(imps map[string]*types.Package, typename string) (*types.Struct, error) {
	genPos := strings.IndexByte(typename, '[')
	if genPos == -1 {
		genPos = len(typename)
	}

	if _, ok := b.structs[typename[:genPos]]; ok {
		return nil, nil
	}

	pos := strings.LastIndexByte(typename[:genPos], '.')
	if pos < 0 {
		return nil, fmt.Errorf("%w: %s", ErrNoModuleType, typename)
	}

	if strings.Contains(typename[:pos], "/internal/") || strings.HasSuffix(typename[:pos], "/internal") || strings.HasPrefix(typename, "internal/") {
		return nil, ErrInternal
	}

	pkg, ok := imps[typename[:pos]]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNoModule, typename)
	}

	obj := pkg.Scope().Lookup(typename[pos+1 : genPos])
	if obj == nil {
		return nil, fmt.Errorf("%w: %s", ErrNoType, typename)
	}

	s, ok := obj.Type().Underlying().(*types.Struct)
	if !ok {
		return nil, ErrNotStruct
	}

	b.imports[typename[:pos]] = pkg

	return s, nil
}

func (b *builder) genImports() *ast.GenDecl {
	names := map[string]struct{}{}

	specs := b.processImports(names, false)
	specs = append(specs, b.processImports(names, true)...)

	return &ast.GenDecl{
		Tok:   token.IMPORT,
		Specs: specs,
	}
}

func (b *builder) processImports(names map[string]struct{}, ext bool) []ast.Spec {
	imps := map[string]*ast.ImportSpec{}

	for _, imp := range b.imports {
		if _, isExt := b.mod.Imports[imp.Path()]; isExt == ext {
			oname := imp.Name()
			name := oname
			pos := 0

			for has(names, oname) {
				pos++
				name = oname + strconv.Itoa(pos)
			}

			var aName *ast.Ident

			if pos > 0 {
				aName = ast.NewIdent(name)
			}

			imps[imp.Path()] = &ast.ImportSpec{
				Name: aName,
				Path: &ast.BasicLit{
					Kind:  token.STRING,
					Value: strconv.Quote(imp.Path()),
				},
			}
		}
	}

	if !ext && !has(imps, "unsafe") {
		imps["unsafe"] = &ast.ImportSpec{
			Path: &ast.BasicLit{
				Kind:  token.STRING,
				Value: `"unsafe"`,
			},
		}
	}

	keys := slices.Collect(maps.Keys(imps))

	slices.Sort(keys)

	var specs []ast.Spec

	for _, key := range keys {
		specs = append(specs, imps[key])
	}

	return specs
}

func has[K comparable, V any](m map[K]V, k K) bool {
	_, has := m[k]

	return has
}

func (b *builder) determineStructs(structs map[string]types.Object) []ast.Decl {
	var decls []ast.Decl

	for name, obj := range structs {
		decls = append(decls, b.conStruct(name, obj.Type().Underlying().(*types.Struct)))
	}

	return decls
}

func (b *builder) conStruct(name string, str *types.Struct) *ast.GenDecl {
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
	ErrNotStruct    = errors.New("not a struct type")
	ErrInternal     = errors.New("cannot process internal type")
)
