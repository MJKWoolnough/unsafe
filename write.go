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

type named struct {
	name string
	typ  types.Type
}

type builder struct {
	mod      *gotypes.ModFile
	imports  map[string]*types.Package
	structs  map[string]ast.Decl
	required []named
	pkg      *types.Package
	fset     *token.FileSet
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

func (b *builder) genAST(packageName string, typeNames []string) (*ast.File, error) {
	imps := gotypes.Imports(b.pkg)

	for _, typeName := range typeNames {
		str, err := b.getStruct(imps, typeName)
		if err != nil {
			return nil, err
		}

		b.required = append(b.required, named{typeName, str})
	}

	for len(b.required) > 0 {
		t := b.required[0]
		b.required = b.required[1:]
		name := t.name

		switch t := t.typ.(type) {
		case *types.Struct:
			if _, ok := b.structs[name]; ok {
				continue
			}

			b.structs[name] = b.conStruct(name, t)
		}
	}

	return &ast.File{
		Name:  ast.NewIdent(packageName),
		Decls: append(append([]ast.Decl{b.genImports()}, slices.Collect(maps.Values(b.structs))...), determineMethods(typeNames)...),
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
						List: b.structFieldList(str.Fields),
					},
				},
			},
		},
	}
}

func typeName(name string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(name, "_", "__"), ".", "_"), "/", "_")
}

func newTypeName(name *types.TypeName) *ast.Ident {
	return ast.NewIdent(typeName(name.Pkg().Path() + "." + name.Name()))
}

func (b *builder) structFieldList(fieldsFn func() iter.Seq[*types.Var]) []*ast.Field {
	var fields []*ast.Field

	for field := range fieldsFn() {
		var name []*ast.Ident

		if n := field.Name(); n != "" {
			name = []*ast.Ident{ast.NewIdent(field.Name())}
		}

		fields = append(fields, &ast.Field{
			Names: name,
			Type:  b.fieldToType(field.Type()),
		})
	}

	return fields
}

func (b *builder) fieldToType(typ types.Type) ast.Expr {
	namedType, isNamed := typ.(*types.Named)
	if isNamed && namedType.Obj().Exported() {
		return &ast.SelectorExpr{
			X:   ast.NewIdent(namedType.Obj().Pkg().Name()),
			Sel: ast.NewIdent(namedType.Obj().Name()),
		}
	}

	switch t := typ.Underlying().(type) {
	case *types.Pointer:
		return &ast.StarExpr{
			X: b.fieldToType(t.Elem()),
		}
	case *types.Map:
		return &ast.MapType{
			Key:   b.fieldToType(t.Key()),
			Value: b.fieldToType(t.Elem()),
		}
	case *types.Array:
		return &ast.ArrayType{
			Len: &ast.BasicLit{
				Value: strconv.FormatInt(t.Len(), 10),
			},
			Elt: b.fieldToType(t.Elem()),
		}
	case *types.Slice:
		return &ast.ArrayType{
			Elt: b.fieldToType(t.Elem()),
		}
	case *types.Struct:
		if isTypeRecursive(typ, map[types.Type]bool{}) {
			b.required = append(b.required, named{namedType.Obj().Pkg().Path() + "." + namedType.Obj().Name(), t})

			return newTypeName(namedType.Obj())
		}

		return &ast.StructType{
			Fields: &ast.FieldList{
				List: b.structFieldList(t.Fields),
			},
		}
	case *types.Signature:
		return &ast.FuncType{
			Params: &ast.FieldList{
				List: b.structFieldList(t.Params().Variables),
			},
			Results: &ast.FieldList{
				List: b.structFieldList(t.Results().Variables),
			},
		}
	case *types.Interface:
		if isTypeRecursive(typ, map[types.Type]bool{}) {
			b.required = append(b.required, named{namedType.Obj().Pkg().Path() + "." + namedType.Obj().Name(), t})

			return newTypeName(namedType.Obj())
		}

		var fields []*ast.Field

		for f := range t.EmbeddedTypes() {
			fields = append(fields, &ast.Field{
				Type: b.fieldToType(f),
			})
		}

		for fn := range t.ExplicitMethods() {
			typ := b.fieldToType(fn.Signature()).(*ast.FuncType)

			typ.Func = token.NoPos

			fields = append(fields, &ast.Field{
				Names: []*ast.Ident{ast.NewIdent(fn.Name())},
				Type:  typ,
			})
		}

		return &ast.InterfaceType{
			Methods: &ast.FieldList{
				List: fields,
			},
		}
	case *types.Basic:
		return ast.NewIdent(t.Name())
	}

	return nil
}

func isTypeRecursive(typ types.Type, found map[types.Type]bool) bool {
	f, ok := found[typ]
	if ok {
		return f
	}

	found[typ] = len(found) == 0

	switch t := typ.Underlying().(type) {
	case *types.Struct:
		for field := range t.Fields() {
			if isTypeRecursive(field.Type(), found) {
				return true
			}
		}
	case *types.Pointer:
		return isTypeRecursive(t.Elem(), found)
	case *types.Map:
		if isTypeRecursive(t.Key(), found) {
			return true
		}

		return isTypeRecursive(t.Elem(), found)
	case *types.Array:
		return isTypeRecursive(t.Elem(), found)
	case *types.Slice:
		return isTypeRecursive(t.Elem(), found)
	case *types.Signature:
		for typ := range t.Params().Variables() {
			if isTypeRecursive(typ.Type(), found) {
				return true
			}
		}

		for typ := range t.Results().Variables() {
			if isTypeRecursive(typ.Type(), found) {
				return true
			}
		}
	case *types.Interface:
		for typ := range t.EmbeddedTypes() {
			if isTypeRecursive(typ, found) {
				return true
			}
		}

		for fn := range t.ExplicitMethods() {
			if isTypeRecursive(fn.Signature(), found) {
				return true
			}
		}
	}

	return false
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
