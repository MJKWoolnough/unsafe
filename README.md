# unsafe

[![CI](https://github.com/MJKWoolnough/unsafe/actions/workflows/go-checks.yml/badge.svg)](https://github.com/MJKWoolnough/unsafe/actions)
[![Go Report Card](https://goreportcard.com/badge/vimagination.zapto.org/unsafe)](https://goreportcard.com/report/vimagination.zapto.org/unsafe)

--

Package unsafe is a program that localises a type from another package allowing the access to unexported fields.

## Highlights

 - Generates local copy of third-party type and a function to convert to it.
 - Optionally adds `go:generate` comment to allow easy regeneration.

## Usage


```bash
go run vimagination.zapto.org/unsafe@latest -o OUTPUT.go [-p PACKAGE_NAME] [-x] package.type [packge.type...]
```

At a minimum, the executable needs to be provided with an output file, via the `-o` flag, and one or more types to localise.

In addition, you can supply the `-x` flag to exclude the `go:generate` header comment, and can provide the `-p` flag to override the package name.

The following is an example command:

```bash
go run vimagination.zapto.org/unsafe@latest -o test/a_test.go -p a_test vimagination.zapto.org/cache.LRU
```

After the first time, assuming that the `-x` flag wasn't provided, the `go generate` command can be used to regenerate and update the output file.
