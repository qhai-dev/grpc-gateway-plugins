package ts

import "strings"

// Depth reports the current indent level.
func (f *File) Depth() int { return f.indent }

// IsIdentifier reports whether s can be used as a bare TypeScript property or
// variable name without quoting.
func IsIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_', r == '$':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}

// Property declares a property name, quoting it when it is not a valid
// identifier. Quoted names are always valid as object literal keys and as
// interface member names.
func Property(name string) string {
	if IsIdentifier(name) {
		return name
	}
	return Quote(name)
}

// Quote renders a double-quoted TypeScript string literal.
func Quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// Access renders the property accessor for name: `.name` when it is a valid
// identifier, `["name"]` otherwise. Callers prepend `?.` themselves.
func Access(name string) string {
	if IsIdentifier(name) {
		return "." + name
	}
	return "[" + Quote(name) + "]"
}

// JSDoc renders proto leading comments as a JSDoc block, one source line per
// output line. indent is prepended to every line. It returns "" when there is no
// comment text.
//
// protoc's leading comments carry the space after `//` already, so each line is
// used as-is apart from that; the historical protoc-gen-go convention of adding
// another would double the gap.
func JSDoc(raw, indent string) string {
	raw = strings.TrimRight(raw, "\n")
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString(indent)
	b.WriteString("/**\n")
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(line, " \t")
		// A literal */ inside a comment would terminate the block early.
		line = strings.ReplaceAll(line, "*/", "*\\/")
		if line == "" {
			b.WriteString(indent)
			b.WriteString(" *\n")
			continue
		}
		b.WriteString(indent)
		b.WriteString(" * ")
		b.WriteString(strings.TrimPrefix(line, " "))
		b.WriteString("\n")
	}
	b.WriteString(indent)
	b.WriteString(" */")
	return b.String()
}
