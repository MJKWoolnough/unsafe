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
)

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

func (b *builder) conStruct(name string, str types.Type) *ast.GenDecl {
	var paramList *ast.FieldList

	params := make(map[string]struct{})

	switch typ := str.(type) {
	case *types.Named:
		if tp := typ.TypeParams(); tp != nil {
			paramList = new(ast.FieldList)

			for t := range tp.TypeParams() {
				paramList.List = append(paramList.List, &ast.Field{
					Names: []*ast.Ident{ast.NewIdent(t.Obj().Name())},
					Type:  b.fieldToType(t.Constraint(), map[string]struct{}{}),
				})

				params[t.Obj().Name()] = struct{}{}
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
				Type:       b.fieldToType(str, params),
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

func (b *builder) structFieldList(fieldsFn func() iter.Seq[*types.Var], params map[string]struct{}) []*ast.Field {
	var fields []*ast.Field

	for field := range fieldsFn() {
		var name []*ast.Ident

		if n := field.Name(); n != "" {
			name = []*ast.Ident{ast.NewIdent(field.Name())}
		}

		fields = append(fields, &ast.Field{
			Names: name,
			Type:  b.fieldToType(field.Type(), params),
		})
	}

	return fields
}

func (b *builder) requiredTypeName(namedType *types.Named) ast.Expr {
	b.required = append(b.required, named{namedType.Obj().Pkg().Path() + "." + namedType.Obj().Name(), namedType})
	return newTypeName(namedType.Obj())
}

func (b *builder) fieldToType(typ types.Type, params map[string]struct{}) ast.Expr {
	if expr := b.handleNamed(typ, params); expr != nil {
		return expr
	}

	switch t := typ.Underlying().(type) {
	case *types.Pointer:
		return &ast.StarExpr{
			X: b.fieldToType(t.Elem(), params),
		}
	case *types.Map:
		return &ast.MapType{
			Key:   b.fieldToType(t.Key(), params),
			Value: b.fieldToType(t.Elem(), params),
		}
	case *types.Array:
		return &ast.ArrayType{
			Len: &ast.BasicLit{
				Value: strconv.FormatInt(t.Len(), 10),
			},
			Elt: b.fieldToType(t.Elem(), params),
		}
	case *types.Slice:
		return &ast.ArrayType{
			Elt: b.fieldToType(t.Elem(), params),
		}
	case *types.Struct:
		if isTypeRecursive(typ, map[types.Type]bool{}) {
			return b.requiredTypeName(typ.(*types.Named))
		}

		return &ast.StructType{
			Fields: &ast.FieldList{
				List: b.structFieldList(t.Fields, params),
			},
		}
	case *types.Signature:
		return &ast.FuncType{
			Params: &ast.FieldList{
				List: b.structFieldList(t.Params().Variables, params),
			},
			Results: &ast.FieldList{
				List: b.structFieldList(t.Results().Variables, params),
			},
		}
	case *types.Interface:
		if t.NumMethods() == 0 {
			return ast.NewIdent("any")
		}

		if namedType, isNamed := typ.(*types.Named); isNamed && (namedType.TypeArgs() != nil || isTypeRecursive(typ, map[types.Type]bool{})) {
			return b.requiredTypeName(typ.(*types.Named))
		}

		var fields []*ast.Field

		for f := range t.EmbeddedTypes() {
			fields = append(fields, &ast.Field{
				Type: b.fieldToType(f, params),
			})
		}

		for fn := range t.ExplicitMethods() {
			typ := b.fieldToType(fn.Signature(), params).(*ast.FuncType)

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

func (b *builder) handleNamed(typ types.Type, params map[string]struct{}) ast.Expr {
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

			for param := range namedType.TypeArgs().Types() {
				indicies = append(indicies, b.fieldToType(param, params))
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

var (
	ErrNoModuleType = errors.New("module-less type")
	ErrNoModule     = errors.New("module not imported")
	ErrNoType       = errors.New("no type found")
	ErrNotStruct    = errors.New("not a struct type")
	ErrInternal     = errors.New("cannot process internal type")
)
