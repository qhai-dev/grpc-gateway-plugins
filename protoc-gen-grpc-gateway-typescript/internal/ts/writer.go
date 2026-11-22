// Package ts renders TypeScript source with stable formatting: deterministic
// import ordering, deterministic declaration order, and no environment-dependent
// output.
package ts

import (
	"fmt"
	"sort"
	"strings"
)

// Import is one namespace import statement.
type Import struct {
	Module string
	Alias  string
}

// File accumulates the body of one generated file and the imports it needs.
type File struct {
	header  string
	imports map[string]*Import
	body    strings.Builder
	indent  int
}

// NewFile starts a generated file with the standard contract header.
func NewFile(header string) *File {
	return &File{header: header, imports: map[string]*Import{}}
}

// AddNamespaceImport records `import type * as alias from "module"`.
//
// Repeated calls for the same module are collapsed, which keeps output stable
// when many fields reference the same external proto file. The alias is a pure
// function of the proto path, so a repeated module always arrives with the same
// alias; when two protos are mapped to one module via M, the first binding wins
// deterministically because imports are written in sorted module order.
func (f *File) AddNamespaceImport(module, alias string) {
	if _, ok := f.imports[module]; ok {
		return
	}
	f.imports[module] = &Import{Module: module, Alias: alias}
}

// P writes one line at the current indent level. With no args the format string
// is used verbatim; an empty line becomes a newline.
func (f *File) P(format string, args ...any) {
	line := format
	if len(args) > 0 {
		line = fmt.Sprintf(format, args...)
	}
	f.writeIndented(line)
}

// PRaw writes a multi-line block, indenting every non-empty line. Used for
// content the caller has already formatted but not indented.
func (f *File) PRaw(block string) {
	for _, line := range strings.Split(strings.TrimRight(block, "\n"), "\n") {
		f.writeIndented(line)
	}
}

// PVerbatim writes a multi-line block exactly as given: the caller has already
// applied indentation to every line. Used for JSDoc, whose continuation lines
// carry their own indent.
func (f *File) PVerbatim(block string) {
	f.body.WriteString(strings.TrimRight(block, "\n"))
	f.body.WriteString("\n")
}

func (f *File) writeIndented(line string) {
	if line == "" {
		f.body.WriteString("\n")
		return
	}
	f.body.WriteString(strings.Repeat("  ", f.indent))
	f.body.WriteString(line)
	f.body.WriteString("\n")
}

// Indent runs fn one level deeper.
func (f *File) Indent(fn func()) {
	f.indent++
	defer func() { f.indent-- }()
	fn()
}

// Empty reports whether nothing but the header has been written.
func (f *File) Empty() bool {
	return strings.TrimSpace(f.body.String()) == ""
}

// WriteString renders the complete file: header, sorted imports, body.
func (f *File) WriteString() string {
	var b strings.Builder
	b.WriteString(f.header)

	if len(f.imports) > 0 {
		modules := make([]string, 0, len(f.imports))
		for m := range f.imports {
			modules = append(modules, m)
		}
		sort.Strings(modules)
		b.WriteString("\n")
		for _, m := range modules {
			fmt.Fprintf(&b, "import type * as %s from %q;\n", f.imports[m].Alias, m)
		}
		b.WriteString("\n")
	}

	body := strings.TrimRight(f.body.String(), "\n")
	if body != "" {
		b.WriteString(body)
		b.WriteString("\n")
	}
	return b.String()
}
