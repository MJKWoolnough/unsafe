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
		var (
			buf strings.Builder
			b   builder
		)

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

func TestBuildFunc(t *testing.T) {
	b, err := newBuilder(".")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	imps := gotypes.Imports(b.pkg)

	for n, test := range [...]struct {
		typ, res string
	}{
		{"strings.Reader", "func makestrings_Reader(x *strings.Reader) *strings_Reader {\n\treturn (*strings_Reader)(unsafe.Pointer(x))\n}"},
	} {
		var buf strings.Builder

		str, err := b.getStruct(imps, test.typ)
		if err != nil {
			t.Errorf("test %d: unexpected error: %s", n+1, err)
		} else {
			format.Node(&buf, token.NewFileSet(), buildFunc(str))

			if str := buf.String(); str != test.res {
				t.Errorf("test %d: expecting type %q, got %q", n+1, test.res, str)
			}
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
		{"package a\n\ntype a struct { a b }\ntype b interface {C() int}", "type a struct {\n\ta interface {\n\t\tC() int\n\t}\n}"},
		{"package a\n\ntype a struct { a b }\ntype b interface {C() b}", "type a struct {\n\ta a_b\n}"},
		{"package a\n\nimport \"sync\"\n\ntype a = sync.Mutex", "type a struct {\n\t_ struct {\n\t}\n\tmu struct {\n\t\tstate int32\n\t\tsema  uint32\n\t}\n}"},
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
	for n, test := range [...]struct {
		typeName []string
		output   string
	}{
		{
			[]string{"strings.Reader"},
			`package e

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
`,
		},
		{
			[]string{"go/types.Package", "go/token.FileSet"},
			`package e

import (
	"go/token"
	"go/types"
	"unsafe"
)

type go_token_FileSet struct {
	mutex sync.RWMutex
	base  int
	tree  struct {
		root *go_token_node
	}
	last atomic.Pointer
}
type go_token_node struct {
	parent *go_token_node
	left   *go_token_node
	right  *go_token_node
	file   *token.File
	key    struct {
		start int
		end   int
	}
	balance int32
	height  int32
}
type go_types_Package struct {
	path      string
	name      string
	scope     *types.Scope
	imports   []*types.Package
	complete  bool
	fake      bool
	cgo       bool
	goVersion string
}

func makego_types_Package(x *types.Package) *go_types_Package {
	return (*go_types_Package)(unsafe.Pointer(x))
}
func makego_token_FileSet(x *token.FileSet) *go_token_FileSet {
	return (*go_token_FileSet)(unsafe.Pointer(x))
}
`,
		},
	} {
		b, err := newBuilder(".")
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		var buf strings.Builder

		if err := b.WriteType(&buf, "e", test.typeName...); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		if str := buf.String(); str != test.output {
			t.Errorf("test %d: expecting output:\n%s\n\ngot:\n%s", n+1, test.output, str)
		}
	}
}
