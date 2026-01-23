package main

import (
	"strings"
	"testing"
)

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
	"sync"
	"sync/atomic"
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
		{
			[]string{"vimagination.zapto.org/httpreaderat.block"},
			`package e

import (
	"unsafe"

	"vimagination.zapto.org/httpreaderat"
)

type vimagination_zapto_org_httpreaderat_block struct {
	data string
	prev *vimagination_zapto_org_httpreaderat_block
	next *vimagination_zapto_org_httpreaderat_block
}

func makevimagination_zapto_org_httpreaderat_block(x *httpreaderat.block) *vimagination_zapto_org_httpreaderat_block {
	return (*vimagination_zapto_org_httpreaderat_block)(unsafe.Pointer(x))
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
