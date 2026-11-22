// Package ts renders TypeScript source with stable formatting: deterministic
// import ordering, deterministic declaration order, and no environment-dependent
// output.
package ts

import (
	"fmt"
	"sort"
	"strings"
)

// ImportKind distinguishes a namespace import from a named import.
type ImportKind int

const (
	// Namespace is `import type * as alias from "module"`.
	Namespace ImportKind = iota
	// Named is `import { A, B } from "module"`.
	Named
)

// Import is one import statement.
type Import struct {
	Module string
	Alias  string
	Names  []string
	Kind   ImportKind
}

// File accumulates the body of one generated file and the imports it needs.
type File struct {
	header  string
	imports map[string]*Import
	// named holds type-only named imports keyed by module.
	named map[string]*Import
	// runtime holds value imports keyed by module.
	runtime map[string]*Import
	body    strings.Builder
	indent  int
}

// NewFile starts a generated file with the standard contract header.
func NewFile(header string) *File {
	return &File{
		header:  header,
		imports: map[string]*Import{},
		named:   map[string]*Import{},
		runtime: map[string]*Import{},
	}
}

// AddNamespaceImport records `import type * as alias from "module"`.
//
// Repeated calls for the same module collapse, which keeps output stable when
// many fields reference the same external proto file. The alias is a pure
// function of the proto path, so a repeated module always arrives with the same
// alias; when two protos are mapped to one module through M, the first binding
// wins deterministically because imports are written in sorted module order.
func (f *File) AddNamespaceImport(module, alias string) {
	if _, ok := f.imports[module]; ok {
		return
	}
	f.imports[module] = &Import{Module: module, Alias: alias, Kind: Namespace}
}

// AddTypeImportNamed records `import type { A, B } from "module"` for a specific
// module, adding to whatever that module already imports.
func (f *File) AddTypeImportNamed(module string, names ...string) {
	f.AddTypeImport(module, names...)
}

// AddTypeImport records `import type { A, B } from "module"`.
//
// Type-only is correct for every generated SDK file: nothing imported this way is
// used as a value, and `verbatimModuleSyntax` requires the distinction.
func (f *File) AddTypeImport(module string, names ...string) {
	if len(names) == 0 {
		return
	}
	if existing, ok := f.named[module]; ok {
		existing.Names = append(existing.Names, names...)
		return
	}
	f.named[module] = &Import{Module: module, Names: names, Kind: Named}
}

// AddValueImport records `import { A, B } from "module"` for values used at
// runtime, such as react-query's useQuery.
func (f *File) AddRuntimeImport(module string, names ...string) {
	if len(names) == 0 {
		return
	}
	if existing, ok := f.runtime[module]; ok {
		existing.Names = append(existing.Names, names...)
		return
	}
	f.runtime[module] = &Import{Module: module, Names: names, Kind: Named}
}

// Imports returns the namespace imports, in module order.
func (f *File) Imports() []*Import {
	modules := make([]string, 0, len(f.imports))
	for m := range f.imports {
		modules = append(modules, m)
	}
	sort.Strings(modules)
	out := make([]*Import, 0, len(modules))
	for _, m := range modules {
		out = append(out, f.imports[m])
	}
	return out
}

// TypeImports returns the type-only named imports, in module order.
func (f *File) TypeImports() []*Import {
	keys := sortedKeys(f.named)
	out := make([]*Import, 0, len(keys))
	for _, k := range keys {
		out = append(out, f.named[k])
	}
	return out
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

// PRaw writes a multi-line block, indenting every non-empty line.
func (f *File) PRaw(block string) {
	for _, line := range strings.Split(strings.TrimRight(block, "\n"), "\n") {
		f.writeIndented(line)
	}
}

// PVerbatim writes a multi-line block exactly as given: the caller has already
// applied indentation to every line. Used for JSDoc and for replaying a rendered
// body, whose continuation lines carry their own indent.
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

// Body returns the accumulated body without the trailing newline, for files that
// are rendered into one buffer and replayed into another.
func (f *File) Body() string {
	return strings.TrimRight(f.body.String(), "\n")
}

// Empty reports whether nothing but the header has been written.
func (f *File) Empty() bool {
	return strings.TrimSpace(f.body.String()) == ""
}

// WriteString renders the complete file: header, sorted imports, body.
//
// Three groups are emitted, each sorted by module and separated by a blank line:
// namespace imports of generated types files, type-only named imports, then
// runtime value imports. None of them can depend on the order fields happened to
// be visited.
func (f *File) WriteString() string {
	var b strings.Builder
	b.WriteString(f.header)

	type group struct {
		keys []string
		emit func(module string)
	}
	var groups []group

	if len(f.imports) > 0 {
		keys := sortedKeys(f.imports)
		groups = append(groups, group{keys: keys, emit: func(m string) {
			fmt.Fprintf(&b, "import type * as %s from %q;\n", f.imports[m].Alias, m)
		}})
	}
	if len(f.named) > 0 {
		keys := sortedKeys(f.named)
		groups = append(groups, group{keys: keys, emit: func(m string) {
			names := dedupeSorted(f.named[m].Names)
			fmt.Fprintf(&b, "import type { %s } from %q;\n", strings.Join(names, ", "), m)
		}})
	}
	if len(f.runtime) > 0 {
		keys := sortedKeys(f.runtime)
		groups = append(groups, group{keys: keys, emit: func(m string) {
			names := dedupeSorted(f.runtime[m].Names)
			fmt.Fprintf(&b, "import { %s } from %q;\n", strings.Join(names, ", "), m)
		}})
	}

	if len(groups) > 0 {
		b.WriteString("\n")
		for i, g := range groups {
			if i > 0 {
				b.WriteString("\n")
			}
			for _, m := range g.keys {
				g.emit(m)
			}
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

func sortedKeys(m map[string]*Import) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func dedupeSorted(names []string) []string {
	seen := make(map[string]bool, len(names))
	out := make([]string, 0, len(names))
	for _, n := range names {
		if seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
