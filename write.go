// Package unsafe is a program that localises a type from another package.
package main

import (
	"errors"
	"go/ast"
	"go/format"
	"go/token"
	"go/types"
	"io"
	"slices"

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
		mod: mod,
		pkg: pkg,
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
	b.structs = make(map[string]ast.Decl)
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

var (
	ErrNoModuleType = errors.New("module-less type")
	ErrNoModule     = errors.New("module not imported")
	ErrNoType       = errors.New("no type found")
	ErrNotStruct    = errors.New("not a struct type")
	ErrInternal     = errors.New("cannot process internal type")
)
