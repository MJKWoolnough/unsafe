package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/mod/module"
	"vimagination.zapto.org/gotypes"
)

func TestTypeName(t *testing.T) {
	for n, test := range [...][2]string{
		{
			"A", "A",
		},
		{
			"a.A", "a_A",
		},
		{
			"a_A", "a__A",
		},
		{
			"vimagination.zapto.org/httpreaderat.Request", "vimagination_zapto_org_httpreaderat_Request",
		},
	} {
		if name := typeName(test[0]); name != test[1] {
			t.Errorf("test %d: expecting name %q, got %q", n+1, test[1], name)
		}
	}
}

func TestFieldToType(t *testing.T) {
	for n, test := range [...]struct {
		typ types.Type
		res string
	}{
		{
			typ: types.Typ[types.Int8],
			res: "int8",
		},
		{
			typ: types.Typ[types.Bool],
			res: "bool",
		},
		{
			typ: types.NewPointer(types.Typ[types.Float32]),
			res: "*float32",
		},
		{
			typ: types.NewMap(types.Typ[types.String], types.NewSlice(types.Typ[types.Uint32])),
			res: "map[string][]uint32",
		},
		{
			typ: types.NewArray(types.Typ[types.Complex128], 3),
			res: "[3]complex128",
		},
	} {
		var (
			buf strings.Builder
			b   builder
		)

		b.init()
		format.Node(&buf, token.NewFileSet(), b.fieldToType(test.typ))

		if str := buf.String(); str != test.res {
			t.Errorf("test %d: expecting type %q, got %q", n+1, test.res, str)
		}
	}
}

func TestIsRecursive(t *testing.T) {
	for n, test := range [...]struct {
		input       string
		isRecursive bool
	}{
		{
			"package a\n\ntype a struct { B b }\n\ntype b struct {c *b}",
			false,
		},
		{
			"package a\n\ntype a struct {B *a}",
			true,
		},
		{
			"package a\n\ntype a struct {B map[string]int}",
			false,
		},
		{
			"package a\n\ntype a struct {B map[*a]int}",
			true,
		},
		{
			"package a\n\ntype a struct {B map[string]a}",
			true,
		},
		{
			"package a\n\ntype a struct {B []a}",
			true,
		},
		{
			"package a\n\ntype a struct {B [2]*a}",
			true,
		},
		{
			"package a\n\ntype a struct {b func() int}",
			false,
		},
		{
			"package a\n\ntype a struct {b func() a}",
			true,
		},
		{
			"package a\n\ntype a struct {b func(int) }",
			false,
		},
		{
			"package a\n\ntype a struct {b func(a) }",
			true,
		},
		{
			"package a\n\ntype a struct { a b }\ntype b interface {C() int}",
			false,
		},
		{
			"package a\n\ntype a struct { a b }\ntype b interface {C() b}",
			false,
		},
		{
			"package a\n\ntype a interface { A() a }",
			true,
		},
		{
			"package a\n\ntype a interface { A() b }\ntype b interface { B() a\n}",
			true,
		},
		{
			"package a\n\ntype a interface { A() b }\ntype b struct { B a\n}",
			true,
		},
	} {
		if self := parseType(t, test.input); isTypeRecursive(self, map[types.Type]bool{}) != test.isRecursive {
			t.Errorf("test %d: didn't get expected recursive value: %v", n+1, test.isRecursive)
		}
	}
}

func parseFile(t *testing.T, input string) *types.Package {
	t.Helper()

	fset := token.NewFileSet()

	f, err := parser.ParseFile(fset, "a.go", input, parser.AllErrors|parser.ParseComments)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	conf := types.Config{
		GoVersion: runtime.Version(),
		Importer:  importer.ForCompiler(fset, runtime.Compiler, nil),
	}

	pkg, err := conf.Check("a", fset, []*ast.File{f}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	return pkg
}

func parseType(t *testing.T, input string) types.Type {
	return parseFile(t, input).Scope().Lookup("a").Type()
}

func TestConStruct(t *testing.T) {
	for n, test := range [...]struct {
		input, output string
	}{
		{"package a\n\nimport \"strings\"\n\ntype a struct { r strings.Reader }", "type a struct {\n\tr strings.Reader\n}"},
		{"package a\n\ntype a struct { a *a }", "type a struct {\n\ta *a_a\n}"},
		{"package a\n\ntype a struct { a b }\ntype b struct { c int }", "type a struct {\n\ta struct {\n\t\tc int\n\t}\n}"},
		{"package a\n\ntype a struct { a func(b) c }\ntype b struct { c int }\ntype c int", "type a struct {\n\ta func(struct {\n\t\tc int\n\t}) int\n}"},
		{"package a\n\ntype a struct { a b }\ntype b interface {\n\tc\n\tA() int\n\tinterface {E()}\n}\ntype c interface {\n\tB(bool, float32) string\n}", "type a struct {\n\ta interface {\n\t\tinterface {\n\t\t\tB(bool, float32) string\n\t\t}\n\t\tinterface {\n\t\t\tE()\n\t\t}\n\t\tA() int\n\t}\n}"},
		{"package a\n\ntype a struct { a b }\ntype b interface {C() int}", "type a struct {\n\ta interface {\n\t\tC() int\n\t}\n}"},
		{"package a\n\ntype a struct { a b }\ntype b interface {C() b}", "type a struct {\n\ta a_b\n}"},
		{"package a\n\ntype a struct { b }\ntype b interface {C() b}", "type a struct {\n\tb a_b\n}"},
		{"package a\n\nimport \"sync\"\n\ntype a = sync.Mutex", "type a struct {\n\t_ struct {\n\t}\n\tmu struct {\n\t\tstate int32\n\t\tsema  uint32\n\t}\n}"},
		{"package a\n\ntype a struct { err error }", "type a struct {\n\terr error\n}"},
	} {
		var (
			buf strings.Builder
			b   builder
		)

		self := parseType(t, test.input)
		imps := gotypes.Imports(self.(interface{ Obj() *types.TypeName }).Obj().Pkg())

		b.init()
		b.mod = &gotypes.ModFile{Imports: map[string]module.Version{}}
		b.getStruct(imps, "a")

		str := b.conStruct("a", self.Underlying().(*types.Struct))

		b.genImports()
		format.Node(&buf, token.NewFileSet(), str)

		if str := buf.String(); str != test.output {
			t.Errorf("test %d: expecting output:\n%s\n\ngot:\n%s", n+1, test.output, str)
		}
	}
}

func buildPackage(t *testing.T, imp module.Version, typeName string) string {
	t.Helper()

	var gomod, file bytes.Buffer

	if imp.Version == "" {
		fmt.Fprintf(&gomod, "module a\n\ngo 1.25.5")
	} else {
		fmt.Fprintf(&gomod, "module a\n\ngo 1.25.5\n\nrequire %s %s", imp.Path, imp.Version)
	}

	fmt.Fprintf(&file, "package a\n\nimport b %q\ntype c = b.%s", imp.Path, typeName)

	tmp := t.TempDir()

	err := os.WriteFile(filepath.Join(tmp, "go.mod"), gomod.Bytes(), 0600)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	err = os.WriteFile(filepath.Join(tmp, "a.go"), file.Bytes(), 0600)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	return tmp
}

func parseTypeFromImport(t *testing.T, imp module.Version, typeName string) types.Type {
	t.Helper()

	tmp := buildPackage(t, imp, typeName)

	pkg, err := gotypes.ParsePackage(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	return pkg.Scope().Lookup("c").Type()
}

func TestConStructFromImport(t *testing.T) {
	for n, test := range [...]struct {
		imp              module.Version
		typename, output string
	}{
		{
			module.Version{Path: "vimagination.zapto.org/memfs", Version: "v1.1.1"},
			"FS",
			"type a struct {\n\tmu   sync.RWMutex\n\tfsRO struct {\n\t\tde vimagination_zapto_org_memfs_directoryEntry\n\t}\n}",
		},
	} {
		var (
			buf strings.Builder
			b   builder
		)

		self := parseTypeFromImport(t, test.imp, test.typename)
		imps := gotypes.Imports(self.(interface{ Obj() *types.TypeName }).Obj().Pkg())

		b.init()
		b.mod = &gotypes.ModFile{Imports: map[string]module.Version{}}
		b.getStruct(imps, "a")

		str := b.conStruct("a", self.Underlying().(*types.Struct))

		b.genImports()
		format.Node(&buf, token.NewFileSet(), str)

		if str := buf.String(); str != test.output {
			t.Errorf("test %d: expecting output:\n%s\n\ngot:\n%s", n+1, test.output, str)
		}
	}
}
