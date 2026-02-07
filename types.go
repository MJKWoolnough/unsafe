package main

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"iter"
	"strconv"
	"strings"

	"vimagination.zapto.org/gotypes"
)

func (b *builder) getStruct(imps map[string]*types.Package, typename string) (types.Type, error) {
	pos := strings.LastIndexByte(typename, '.')
	if pos < 0 {
		return nil, fmt.Errorf("%w: %s", ErrNoModuleType, typename)
	}

	if isInternal(typename[:pos]) {
		return nil, ErrInternal
	}

	pkg, ok := imps[typename[:pos]]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNoModule, typename[:pos])
	}

	obj := pkg.Scope().Lookup(typename[pos+1:])
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

func (b *builder) conStruct(name string, str types.Type) *ast.GenDecl {
	var paramList *ast.FieldList

	switch typ := str.(type) {
	case *types.Named:
		if tp := typ.TypeParams(); tp != nil {
			paramList = new(ast.FieldList)

			for t := range tp.TypeParams() {
				paramList.List = append(paramList.List, &ast.Field{
					Names: []*ast.Ident{ast.NewIdent(t.Obj().Name())},
					Type:  b.fieldToType(t.Constraint()),
				})
			}
		}

		str = typ.Underlying()
	}

	return &ast.GenDecl{
		Tok: token.TYPE,
		Specs: []ast.Spec{
			&ast.TypeSpec{
				Name:       ast.NewIdent(typeName(name)),
				TypeParams: paramList,
				Type:       b.fieldToType(str),
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

func (b *builder) structFieldList(fieldsFn func() iter.Seq[*types.Var], variadic bool) []*ast.Field {
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

	if variadic && len(fields) > 0 {
		fields[len(fields)-1].Type = &ast.UnaryExpr{
			Op: token.ELLIPSIS,
			X:  fields[len(fields)-1].Type.(*ast.ArrayType).Elt,
		}
	}

	return fields
}

func (b *builder) requiredTypeName(namedType *types.Named) ast.Expr {
	b.required = append(b.required, named{namedType.Obj().Pkg().Path() + "." + namedType.Obj().Name(), namedType})

	return newTypeName(namedType.Obj())
}

func (b *builder) fieldToType(typ types.Type) ast.Expr {
	if expr := b.handleNamed(typ); expr != nil {
		return expr
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
		if namedType, isNamed := typ.(*types.Named); isNamed && gotypes.IsTypeRecursive(typ) {
			return b.requiredTypeName(namedType)
		}

		return &ast.StructType{
			Fields: &ast.FieldList{
				List: b.structFieldList(t.Fields, false),
			},
		}
	case *types.Signature:
		return &ast.FuncType{
			Params: &ast.FieldList{
				List: b.structFieldList(t.Params().Variables, t.Variadic()),
			},
			Results: &ast.FieldList{
				List: b.structFieldList(t.Results().Variables, false),
			},
		}
	case *types.Interface:
		if t.NumMethods() == 0 {
			return ast.NewIdent("any")
		}

		if namedType, isNamed := typ.(*types.Named); isNamed && (namedType.TypeArgs() != nil || gotypes.IsTypeRecursive(typ) || interfaceContainsUnexported(t)) {
			return b.requiredTypeName(typ.(*types.Named))
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

func interfaceContainsUnexported(t *types.Interface) bool {
	for method := range t.ExplicitMethods() {
		if !method.Exported() {
			return true
		}

		for v := range method.Signature().Params().Variables() {
			if named, ok := v.Type().(*types.Named); ok {
				if !named.Obj().Exported() {
					return true
				}
			}
		}

		for v := range method.Signature().Results().Variables() {
			if named, ok := v.Type().(*types.Named); ok {
				if !named.Obj().Exported() {
					return true
				}
			}
		}
	}

	return false
}

func (b *builder) handleNamed(typ types.Type) ast.Expr {
	switch namedType := typ.(type) {
	case *types.Named:
		var name ast.Expr

		if namedType.Obj().Exported() {
			name = &ast.SelectorExpr{
				X:   b.packageName(namedType.Obj().Pkg()),
				Sel: ast.NewIdent(namedType.Obj().Name()),
			}
		} else if namedType.Obj().Pkg() == nil {
			return ast.NewIdent(namedType.Obj().Name())
		}

		if namedType.TypeParams() != nil {
			if name == nil {
				name = b.requiredTypeName(namedType)
			}

			indicies := make([]ast.Expr, 0, namedType.TypeArgs().Len())

			for typ, param := range combineIters(namedType.Obj().Type().(*types.Named).TypeParams().TypeParams(), namedType.TypeArgs().Types()) {
				var name *types.TypeName

				switch p := param.(type) {
				case *types.Named:
					name = p.Obj()
				case *types.TypeParam:
					name = p.Obj()
				}

				fieldType := b.fieldToType(param)
				indicies = append(indicies, fieldType)
				b.implements[newTypeName(name).Name] = interfaceType{typ.Underlying().(*types.Interface), param}
			}

			return &ast.IndexListExpr{
				X:       name,
				Indices: indicies,
			}
		} else if name != nil && !isInternal(namedType.Obj().Pkg().Path()) {
			return name
		}
	case *types.TypeParam:
		return ast.NewIdent(namedType.Obj().Name())
	}

	return nil
}

func combineIters[V1, V2 any](iter1 iter.Seq[V1], iter2 iter.Seq[V2]) iter.Seq2[V1, V2] {
	return func(yield func(V1, V2) bool) {
		nextIter1, stopIter1 := iter.Pull(iter1)
		nextIter2, stopIter2 := iter.Pull(iter2)

		defer stopIter1()
		defer stopIter2()

		for {
			v1, ok := nextIter1()
			if !ok {
				return
			}

			v2, _ := nextIter2()

			if !yield(v1, v2) {
				return
			}
		}
	}
}

func (b *builder) packageName(pkg *types.Package) *ast.Ident {
	name, ok := b.imports[pkg.Path()]
	if !ok {
		name = &packageName{pkg, ast.NewIdent("")}
		b.imports[pkg.Path()] = name
	}

	return name.Ident
}

var (
	ErrNoModuleType = errors.New("module-less type")
	ErrNoModule     = errors.New("module not imported")
	ErrNoType       = errors.New("no type found")
	ErrNotStruct    = errors.New("not a struct type")
	ErrInternal     = errors.New("cannot process internal type")
)
