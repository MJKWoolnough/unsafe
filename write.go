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

type named struct {
	name string
	typ  types.Type
}

type pos []int

func (p *pos) newLine() token.Pos {
	l := len(*p)
	*p = append(*p, len(*p), len(*p)+1)

	return token.Pos(l + 1)
}

type packageName struct {
	*types.Package
	*ast.Ident
}

type builder struct {
	mod      *gotypes.ModFile
	imports  map[string]*packageName
	structs  map[string]ast.Decl
	required []named
	methods  []ast.Decl
	pkg      *types.Package
	pos
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
		structs: make(map[string]ast.Decl),
		pkg:     pkg,
	}, nil
}

func (b *builder) WriteType(w io.Writer, pkgName string, typeNames ...string) error {
	if pkgName == "" {
		pkgName = b.pkg.Name()
	}

	b.init()

	file, err := b.genAST(pkgName, typeNames)
	if err != nil {
		return err
	}

	fset := token.NewFileSet()
	wsfile := fset.AddFile("out.go", 1, len(b.pos))

	wsfile.SetLines(b.pos)

	return format.Node(w, fset, file)
}

func (b *builder) init() {
	b.pos = []int{0, 1}
	b.imports = map[string]*packageName{"unsafe": {types.NewPackage("unsafe", "unsafe"), ast.NewIdent("unsafe")}}
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

		switch typ := t.typ.Underlying().(type) {
		case *types.Struct:
			if _, ok := b.structs[name]; ok {
				continue
			}

			b.structs[name] = b.conStruct(name, typ)

			if slices.Contains(typeNames, name) {
				b.methods = append(b.methods, b.buildFunc(t.typ))
			}
		}
	}

	return &ast.File{
		Name:  ast.NewIdent(packageName),
		Decls: append(append([]ast.Decl{b.genImports()}, sortedValues(b.structs)...), b.methods...),
	}, nil
}

func (b *builder) getStruct(imps map[string]*types.Package, typename string) (types.Type, error) {
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

	if isInternal(typename[:pos]) {
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

	_, ok = obj.Type().Underlying().(*types.Struct)
	if !ok {
		return nil, ErrNotStruct
	}

	b.imports[typename[:pos]] = &packageName{pkg, ast.NewIdent("")}

	return obj.Type(), nil
}

func isInternal(path string) bool {
	return strings.Contains(path, "/internal/") || strings.HasSuffix(path, "/internal") || strings.HasPrefix(path, "internal/")
}

func (b *builder) genImports() *ast.GenDecl {
	names := map[string]struct{}{}

	specs := b.processImports(names, false)
	stdlib := len(specs)
	specs = append(specs, b.processImports(names, true)...)

	if len(specs) > stdlib {
		imp := specs[stdlib].(*ast.ImportSpec)

		if imp.Name != nil {
			specs[stdlib].(*ast.ImportSpec).Name.NamePos = b.newLine()
		} else {
			specs[stdlib].(*ast.ImportSpec).Path.ValuePos = b.newLine()
		}
	}

	return &ast.GenDecl{
		Tok:   token.IMPORT,
		Specs: specs,
	}
}

func (b *builder) processImports(names map[string]struct{}, ext bool) []ast.Spec {
	imps := b.buildImports(names, ext)

	return sortedValues(imps)
}

func (b *builder) buildImports(names map[string]struct{}, ext bool) map[string]ast.Spec {
	imps := map[string]ast.Spec{}

	for _, imp := range b.imports {
		if _, isExt := b.mod.Imports[imp.Path()]; isExt == ext {
			oname := imp.Package.Name()
			name := oname
			pos := 0

			for has(names, oname) {
				pos++
				name = oname + strconv.Itoa(pos)
			}

			names[name] = struct{}{}

			var aName *ast.Ident

			if pos > 0 {
				aName = ast.NewIdent(name)
			}

			imp.Ident.Name = name

			imps[imp.Path()] = &ast.ImportSpec{
				Name: aName,
				Path: &ast.BasicLit{
					Kind:  token.STRING,
					Value: strconv.Quote(imp.Path()),
				},
			}
		}
	}

	return imps
}

func has[K comparable, V any](m map[K]V, k K) bool {
	_, has := m[k]

	return has
}

func sortedValues[K cmp.Ordered, V any](m map[K]V) []V {
	keys := slices.Collect(maps.Keys(m))

	slices.Sort(keys)

	var specs []V

	for _, key := range keys {
		specs = append(specs, m[key])
	}

	return specs
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
	if isNamed && namedType.Obj().Exported() && !isInternal(namedType.Obj().Pkg().Path()) {
		return &ast.SelectorExpr{
			X:   b.packageName(namedType.Obj().Pkg()),
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
			b.required = append(b.required, named{namedType.Obj().Pkg().Path() + "." + namedType.Obj().Name(), namedType})

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
			b.required = append(b.required, named{namedType.Obj().Pkg().Path() + "." + namedType.Obj().Name(), namedType})

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

func (b *builder) packageName(pkg *types.Package) *ast.Ident {
	name, ok := b.imports[pkg.Path()]
	if !ok {
		name = &packageName{pkg, ast.NewIdent("")}
		b.imports[pkg.Path()] = name
	}

	return name.Ident
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

func (b *builder) buildFunc(typ types.Type) *ast.FuncDecl {
	named := typ.(*types.Named).Obj()
	tname := typeName(named.Pkg().Path() + "." + named.Name())

	return &ast.FuncDecl{
		Name: ast.NewIdent("make" + tname),
		Type: &ast.FuncType{
			Params: &ast.FieldList{
				List: []*ast.Field{
					{
						Names: []*ast.Ident{ast.NewIdent("x")},
						Type: &ast.StarExpr{
							X: &ast.SelectorExpr{
								X:   b.packageName(named.Pkg()),
								Sel: ast.NewIdent(named.Name()),
							},
						},
					},
				},
			},
			Results: &ast.FieldList{
				List: []*ast.Field{
					{
						Type: &ast.StarExpr{
							X: ast.NewIdent(tname),
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
									X: ast.NewIdent(tname),
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
	ErrNotStruct    = errors.New("not a struct type")
	ErrInternal     = errors.New("cannot process internal type")
)
