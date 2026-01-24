package main

import (
	"go/format"
	"go/token"
	"strings"
	"testing"

	"vimagination.zapto.org/gotypes"
)

func TestBuildFunc(t *testing.T) {
	b, err := newBuilder(".")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	imps := gotypes.Imports(b.pkg)
	b.init()

	for n, test := range [...]struct {
		typ, res string
	}{
		{"strings.Reader", "func make_strings_Reader(x *strings.Reader) *strings_Reader {\n\treturn (*strings_Reader)(unsafe.Pointer(x))\n}"},
		{"vimagination.zapto.org/httpreaderat.block", "func make_vimagination_zapto_org_httpreaderat_block(x *httpreaderat.block) *vimagination_zapto_org_httpreaderat_block {\n\treturn (*vimagination_zapto_org_httpreaderat_block)(unsafe.Pointer(x))\n}"},
	} {
		var buf strings.Builder

		str, err := b.getStruct(imps, test.typ)
		if err != nil {
			t.Errorf("test %d: unexpected error: %s", n+1, err)
		} else {
			b.genImports()

			format.Node(&buf, token.NewFileSet(), b.buildFunc(str))

			if str := buf.String(); str != test.res {
				t.Errorf("test %d: expecting type %q, got %q", n+1, test.res, str)
			}
		}
	}
}
