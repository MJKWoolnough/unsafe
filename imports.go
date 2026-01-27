package main

import (
	"cmp"
	"go/ast"
	"go/token"
	"maps"
	"slices"
	"strconv"
	"strings"
)

func isInternal(path string) bool {
	return strings.Contains(path, "/internal/") || strings.HasSuffix(path, "/internal") || strings.HasPrefix(path, "internal/")
}

func (b *builder) genImports() *ast.GenDecl {
	names := map[string]struct{}{}
	specs := b.processImports(names, false)
	stdlib := len(specs)
	specs = append(specs, b.processImports(names, true)...)

	if len(specs) > stdlib {
		if specs[stdlib].(*ast.ImportSpec).Name != nil {
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

	for _, imp := range sortedValues(b.imports) {
		if _, isExt := b.mod.Imports[imp.Path()]; isExt == ext {
			oname := imp.Package.Name()
			name := oname
			pos := 0

			for has(names, name) {
				pos++
				name = oname + strconv.Itoa(pos)
			}

			var aName *ast.Ident

			if pos > 0 {
				aName = ast.NewIdent(name)
			}

			names[name] = struct{}{}
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
