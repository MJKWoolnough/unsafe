package main

import (
	"errors"
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

func TestGetAllStructs(t *testing.T) {
	pkg, err := gotypes.ParsePackage(".")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	imps := gotypes.Imports(pkg)

	structs := make(map[string]types.Object)

	if err := getAllStructs(imps, structs, "Request"); !errors.Is(err, ErrNoModuleType) {
		t.Errorf("expected error %v, got %v", ErrNoModuleType, err)
	}

	if err := getAllStructs(imps, structs, "unknown.Request"); !errors.Is(err, ErrNoModule) {
		t.Errorf("expected error %v, got %v", ErrNoModule, err)
	}

	if err := getAllStructs(imps, structs, "vimagination.zapto.org/httpreaderat.Requested"); !errors.Is(err, ErrNoType) {
		t.Errorf("expected error %v, got %v", ErrNoType, err)
	}

	if err := getAllStructs(imps, structs, "internal/sync.Mutex"); err != nil {
		t.Errorf("unexpected error %v", err)
	} else if len(structs) > 0 {
		t.Errorf("expected no types, got %v", structs)
	}

	if err := getAllStructs(imps, structs, "vimagination.zapto.org/httpreaderat.Request"); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	for typeName, structStr := range map[string]string{
		"vimagination.zapto.org/httpreaderat.Request": "struct{url string; length int64; blockSize int64; cache *vimagination.zapto.org/cache.LRU[int64, string]}",
		"vimagination.zapto.org/cache.LRU":            "struct{vimagination.zapto.org/cache.cache[T, U, vimagination.zapto.org/cache.lru[T, U]]}",
	} {
		if s := structs[typeName].Type().Underlying().String(); s != structStr {
			t.Errorf("expecting struct %q to have type %q, got %q", typeName, structStr, s)
		}
	}
}

func TestDetermineImports(t *testing.T) {
	pkg, err := gotypes.ParsePackage(".")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	imps := gotypes.Imports(pkg)
	structs := make(map[string]types.Object)

	if err := getAllStructs(imps, structs, "strings.Reader"); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	expected := &ast.GenDecl{
		Tok: token.IMPORT,
		Specs: []ast.Spec{
			&ast.ImportSpec{
				Path: &ast.BasicLit{
					Value: `"strings"`,
				},
			},
			&ast.ImportSpec{
				Path: &ast.BasicLit{
					Value: `"unsafe"`,
				},
			},
		},
	}

	if imp := determineImports(structs); !reflect.DeepEqual(imp, expected) {
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

func TestIsStructRecursive(t *testing.T) {
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
	} {
		if self := parseType(t, test.input); isStructRecursive(self, map[*types.Struct]bool{self: true}) != test.isRecursive {
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

func parseType(t *testing.T, input string) *types.Struct {
	return parseFile(t, input).Scope().Lookup("a").Type().Underlying().(*types.Struct)
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
	} {
		var buf strings.Builder

		self := parseType(t, test.input)

		format.Node(&buf, token.NewFileSet(), conStruct("a", self))

		if str := buf.String(); str != test.output {
			t.Errorf("test %d: expecting output:\n%s\n\ngot:\n%s", n+1, test.output, str)
		}
	}
}

func TestWriteType(t *testing.T) {
	var buf strings.Builder

	pkg, err := gotypes.ParsePackage(".")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if err := WriteType(&buf, pkg, "e", "strings.Reader"); err != nil {
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
		t.Errorf("expecting output:\n%s\n\ngot:\n %s", expectation, str)
	}
}
