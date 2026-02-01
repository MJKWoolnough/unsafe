package main

import (
	"go/ast"
	"go/types"
)

func (b *builder) buildFunc(typ types.Type) *ast.FuncDecl {
	namedType := typ.(*types.Named)
	obj := namedType.Obj()
	tname := typeName(obj.Pkg().Path() + "." + obj.Name())

	var (
		oname ast.Expr = &ast.SelectorExpr{
			X:   b.packageName(obj.Pkg()),
			Sel: ast.NewIdent(obj.Name()),
		}
		nname ast.Expr = ast.NewIdent(tname)

		paramList *ast.FieldList
	)

	if namedType.TypeParams() != nil {
		paramList = new(ast.FieldList)
		indicies := make([]ast.Expr, 0, namedType.TypeArgs().Len())

		for param := range namedType.TypeParams().TypeParams() {
			paramList.List = append(paramList.List, &ast.Field{
				Names: []*ast.Ident{ast.NewIdent(param.Obj().Name())},
				Type:  b.fieldToType(param.Constraint()),
			})
			indicies = append(indicies, b.fieldToType(param))
		}

		oname = &ast.IndexListExpr{
			X:       oname,
			Indices: indicies,
		}
		nname = &ast.IndexListExpr{
			X:       nname,
			Indices: indicies,
		}
	}

	return &ast.FuncDecl{
		Name: ast.NewIdent("make_" + tname),
		Type: &ast.FuncType{
			TypeParams: paramList,
			Params: &ast.FieldList{
				List: []*ast.Field{
					{
						Names: []*ast.Ident{ast.NewIdent("x")},
						Type: &ast.StarExpr{
							X: oname,
						},
					},
				},
			},
			Results: &ast.FieldList{
				List: []*ast.Field{
					{
						Type: &ast.StarExpr{
							X: nname,
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
									X: nname,
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

func (b *builder) addRequiredMethods(decls []ast.Decl) []ast.Decl {
	var ndecls []ast.Decl

	for _, decl := range decls {
		ndecls = append(ndecls, decl)
		typ := decl.(*ast.GenDecl).Specs[0].(*ast.TypeSpec)
		name := typ.Name.Name

		if intf, ok := b.implements[name]; ok {
			for method := range intf.Methods() {
				ndecls = append(ndecls, &ast.FuncDecl{
					Recv: &ast.FieldList{
						List: []*ast.Field{
							{
								Type: b.handleNamed(intf.Type),
							},
						},
					},
					Name: ast.NewIdent(method.Name()),
					Type: &ast.FuncType{
						Params: &ast.FieldList{
							List: setBlankNames(b.structFieldList(method.Signature().Params().Variables, method.Signature().Variadic())),
						},
						Results: &ast.FieldList{
							List: setBlankNames(b.structFieldList(method.Signature().Results().Variables, false)),
						},
					},
					Body: &ast.BlockStmt{
						List: []ast.Stmt{&ast.ReturnStmt{}},
					},
				})
			}
		}
	}

	return ndecls
}

var blankName = []*ast.Ident{ast.NewIdent("_")}

func setBlankNames(fields []*ast.Field) []*ast.Field {
	for n := range fields {
		fields[n].Names = blankName
	}

	return fields
}
