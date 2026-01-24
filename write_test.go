package main

import (
	"strings"
	"testing"

	"golang.org/x/mod/module"
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

func make_strings_Reader(x *strings.Reader) *strings_Reader {
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

func make_go_types_Package(x *types.Package) *go_types_Package {
	return (*go_types_Package)(unsafe.Pointer(x))
}

func make_go_token_FileSet(x *token.FileSet) *go_token_FileSet {
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

func make_vimagination_zapto_org_httpreaderat_block(x *httpreaderat.block) *vimagination_zapto_org_httpreaderat_block {
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

func TestWriteTypeFromImport(t *testing.T) {
	for n, test := range [...]struct {
		imp      module.Version
		typeName string
		output   string
	}{
		{
			module.Version{Path: "vimagination.zapto.org/memfs", Version: "v1.1.1"},
			"vimagination.zapto.org/memfs.FS",
			`package e

import (
	"io/fs"
	"sync"
	"time"
	"unsafe"

	"vimagination.zapto.org/memfs"
)

type vimagination_zapto_org_memfs_FS struct {
	mu   sync.RWMutex
	fsRO struct {
		de vimagination_zapto_org_memfs_directoryEntry
	}
}

type vimagination_zapto_org_memfs_dirEnt struct {
	directoryEntry vimagination_zapto_org_memfs_directoryEntry
	name           string
}

type vimagination_zapto_org_memfs_directoryEntry interface {
	IsDir() bool
	ModTime() time.Time
	Mode() fs.FileMode
	Size() int64
	Type() fs.FileMode
	bytes() ([]byte, interface {
		Error() string
	})
	getEntry(string) (*vimagination_zapto_org_memfs_dirEnt, interface {
		Error() string
	})
	open(name string, mode uint8) (fs.File, interface {
		Error() string
	})
	seal() vimagination_zapto_org_memfs_directoryEntry
	setMode(fs.FileMode)
	setTimes(time.Time, time.Time)
	string() (string, interface {
		Error() string
	})
}

func make_vimagination_zapto_org_memfs_FS(x *memfs.FS) *vimagination_zapto_org_memfs_FS {
	return (*vimagination_zapto_org_memfs_FS)(unsafe.Pointer(x))
}
`,
		},
	} {
		last := strings.LastIndexByte(test.typeName, '.')

		b, err := newBuilder(buildPackage(t, test.imp, test.typeName[last+1:]))
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		var buf strings.Builder

		if err := b.WriteType(&buf, "e", test.typeName); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		if str := buf.String(); str != test.output {
			t.Errorf("test %d: expecting output:\n%s\n\ngot:\n%s", n+1, test.output, str)
		}
	}
}
