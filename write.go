// Package unsafe is a program that localises a type from another package.
package main

import (
	"go/ast"
	"go/format"
	"go/token"
	"go/types"
	"io"
	"slices"
	"strconv"
	"strings"

	"vimagination.zapto.org/gotypes"
)

const generateComment = "//go:generate go run vimagination.zapto.org/unsafe@latest "

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
	mod        *gotypes.ModFile
	imports    map[string]*packageName
	structs    map[string]ast.Decl
	implements map[string]interfaceType
	required   []named
	functions  []ast.Decl
	args       []string
	pkg        *types.Package
	pos
}

type interfaceType struct {
	*types.Interface
	types.Type
}

func newBuilder(module string, args ...string) (*builder, error) {
	pkg, err := gotypes.ParsePackage(module)
	if err != nil {
		return nil, err
	}

	mod, err := gotypes.ParseModFile(module)
	if err != nil {
		return nil, err
	}

	return &builder{
		mod:  mod,
		pkg:  pkg,
		args: args,
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
	b.implements = make(map[string]interfaceType)
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

		if _, ok := b.structs[name]; ok {
			continue
		}

		b.structs[name] = b.conStruct(name, t.typ)

		if slices.Contains(typeNames, name) {
			b.functions = append(b.functions, b.buildFunc(t.typ))
		}
	}

	var doc *ast.CommentGroup

	if len(b.args) > 0 {
		doc = &ast.CommentGroup{
			List: []*ast.Comment{
				{
					Slash: b.newLine(),
					Text:  generateComment + encodeOpts(b.args),
				},
			},
		}
	}

	return &ast.File{
		Doc:     doc,
		Package: b.newLine(),
		Name:    ast.NewIdent(packageName),
		Decls:   append(append([]ast.Decl{b.genImports()}, b.addNewLines(b.addRequiredMethods(sortedValues(b.structs)))...), b.addNewLines(b.functions)...),
	}, nil
}

func encodeOpts(opts []string) string {
	var buf []byte

	for n, opt := range opts {
		if n > 0 {
			buf = append(buf, ' ')
		}

		if strings.Contains(opt, " ") {
			buf = strconv.AppendQuote(buf, opt)
		} else {
			buf = append(buf, opt...)
		}
	}

	return string(buf)
}

func (b *builder) addNewLines(decls []ast.Decl) []ast.Decl {
	for n := range decls {
		switch decl := decls[n].(type) {
		case *ast.GenDecl:
			decl.TokPos = b.newLine()
		case *ast.FuncDecl:
			decl.Type.Func = b.newLine()
		}
	}

	return decls
}
