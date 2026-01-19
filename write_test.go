package main

import (
	"go/ast"
	"go/format"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"vimagination.zapto.org/gotypes"
)

func TestDetermineImports(t *testing.T) {
	b, err := newBuilder(".")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	imps := gotypes.Imports(b.pkg)

	b.imports["strings"] = imps["strings"]

	expected := &ast.GenDecl{
		Tok: token.IMPORT,
		Specs: []ast.Spec{
			&ast.ImportSpec{
				Path: &ast.BasicLit{
					Kind:  token.STRING,
					Value: `"strings"`,
				},
			},
			&ast.ImportSpec{
				Path: &ast.BasicLit{
					Kind:  token.STRING,
					Value: `"unsafe"`,
				},
			},
		},
	}

	if imp := b.genImports(); !reflect.DeepEqual(imp, expected) {
		t.Errorf("expecting imports %v, got %v", expected, imp)
	}
}

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
		var buf strings.Builder

		format.Node(&buf, token.NewFileSet(), fieldToType(test.typ))

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

func TestBuildFunc(t *testing.T) {
	for n, test := range [...]struct {
		typ, res string
	}{
		{"strings.Reader", "func makestrings_Reader(x *strings.Reader) *strings_Reader {\n\treturn (*strings_Reader)(unsafe.Pointer(x))\n}"},
	} {
		var buf strings.Builder

		format.Node(&buf, token.NewFileSet(), buildFunc(test.typ))

		if str := buf.String(); str != test.res {
			t.Errorf("test %d: expecting type %q, got %q", n+1, test.res, str)
		}
	}
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
	} {
		var buf strings.Builder

		self := parseType(t, test.input)

		var b builder

		format.Node(&buf, token.NewFileSet(), b.conStruct("a", self.Underlying().(*types.Struct)))

		if str := buf.String(); str != test.output {
			t.Errorf("test %d: expecting output:\n%s\n\ngot:\n%s", n+1, test.output, str)
		}
	}
}

func TestWriteType(t *testing.T) {
	var buf strings.Builder

	b, err := newBuilder(".")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if err := b.WriteType(&buf, "e", "strings.Reader"); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	const expectation = `package e

import (
	"strings"
	"unsafe"
)

type strings_Reader struct {
	s        string
	i        int64
	prevRune int
}

func makestrings_Reader(x *strings.Reader) *strings_Reader {
	return (*strings_Reader)(unsafe.Pointer(x))
}
`

	if str := buf.String(); str != expectation {
		t.Errorf("expecting output:\n%s\n\ngot:\n%s", expectation, str)
	}
}
