package main

import (
	"go/ast"
	"go/types"
)

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
