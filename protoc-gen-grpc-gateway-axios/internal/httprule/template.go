// Package httprule parses google.api.http templates and decides which request
// fields go into the URL path, which into the query string and which into the
// body. The semantics follow grpc-gateway's runtime, most importantly
// runtime.PopulateQueryParameters.
package httprule

import (
	"fmt"
	"strings"
)

// Template is a parsed HTTP path template such as /v1/users/{id} or
// /v1/{name=shippers/*}:activate.
type Template struct {
	// Segments are the path segments in order.
	Segments []Segment
	// Verb is the trailing ":verb" suffix, without the colon.
	Verb string
	// Raw is the original template text.
	Raw string
}

// Segment is either a literal path segment or a variable.
type Segment struct {
	Literal  string
	Variable *Variable
}

// Variable is a field binding inside a path template.
type Variable struct {
	FieldPath FieldPath
	// SubSegments is the sub-template after "=", e.g. ["shippers", "*"].
	SubSegments []string
	// MultiSeg reports whether the variable spans more than one URL segment, so
	// its value must be encoded segment by segment rather than as one token.
	MultiSeg bool
}

// FieldPath is a dotted proto field path, using proto field names.
type FieldPath []string

// String renders the path in dotted form.
func (p FieldPath) String() string { return strings.Join(p, ".") }

// Empty reports whether the path has no segments.
func (p FieldPath) Empty() bool { return len(p) == 0 }

// IsRoot reports whether the path is the whole request, i.e. body: "*".
func (p FieldPath) IsRoot() bool { return len(p) == 1 && p[0] == "*" }

// HasPrefix reports whether p starts with prefix, i.e. p is prefix or a
// descendant of it.
func (p FieldPath) HasPrefix(prefix FieldPath) bool {
	if len(prefix) > len(p) {
		return false
	}
	for i, s := range prefix {
		if p[i] != s {
			return false
		}
	}
	return true
}

// ParseFieldPath parses "user.profile" into its segments.
func ParseFieldPath(s string) (FieldPath, error) {
	if s == "" {
		return nil, nil
	}
	if s == "*" {
		return FieldPath{"*"}, nil
	}
	parts := strings.Split(s, ".")
	for _, p := range parts {
		if p == "" {
			return nil, fmt.Errorf("invalid field path %q: empty segment", s)
		}
	}
	return FieldPath(parts), nil
}

// ParseTemplate parses an HTTP path template.
//
// Grammar (google/api/http.proto): "/" Segments [ Verb ], where each segment is
// a literal, a variable or "*", and the verb follows the last segment.
func ParseTemplate(tmpl string) (*Template, error) {
	if tmpl == "" {
		return nil, fmt.Errorf("empty path template")
	}
	if !strings.HasPrefix(tmpl, "/") {
		return nil, fmt.Errorf("path template %q must start with %q", tmpl, "/")
	}
	t := &Template{Raw: tmpl}

	rawSegments, err := splitSegments(tmpl[1:])
	if err != nil {
		return nil, err
	}
	// "/" names the root: one empty segment, which renders as "/" again.
	if len(rawSegments) == 1 && rawSegments[0] == "" {
		return &Template{Raw: tmpl}, nil
	}

	// The verb lives at the very end, outside any braces.
	last := len(rawSegments) - 1
	seg, verb, err := splitVerb(rawSegments[last])
	if err != nil {
		return nil, err
	}
	t.Verb = verb
	rawSegments[last] = seg

	for _, raw := range rawSegments {
		if strings.HasPrefix(raw, "{") {
			v, err := parseVariable(raw)
			if err != nil {
				return nil, err
			}
			t.Segments = append(t.Segments, Segment{Variable: v})
			continue
		}
		t.Segments = append(t.Segments, Segment{Literal: raw})
	}

	if err := t.validate(); err != nil {
		return nil, err
	}
	return t, nil
}

// splitSegments splits on "/" while respecting braces, so a sub-template such as
// {name=shippers/*} stays one segment.
func splitSegments(s string) ([]string, error) {
	var (
		segments []string
		current  strings.Builder
		depth    int
	)
	for _, r := range s {
		switch r {
		case '{':
			depth++
			current.WriteRune(r)
		case '}':
			depth--
			if depth < 0 {
				return nil, fmt.Errorf("unbalanced %q in template %q", "}", s)
			}
			current.WriteRune(r)
		case '/':
			if depth > 0 {
				current.WriteRune(r)
				continue
			}
			segments = append(segments, current.String())
			current.Reset()
		default:
			current.WriteRune(r)
		}
	}
	if depth != 0 {
		return nil, fmt.Errorf("unbalanced %q in template %q", "{", s)
	}
	segments = append(segments, current.String())
	return segments, nil
}

// splitVerb separates a trailing ":verb" from the final segment.
func splitVerb(segment string) (string, string, error) {
	depth := 0
	for i, r := range segment {
		switch r {
		case '{':
			depth++
		case '}':
			depth--
		case ':':
			if depth == 0 {
				verb := segment[i+1:]
				if verb == "" {
					return "", "", fmt.Errorf("empty verb in segment %q", segment)
				}
				return segment[:i], verb, nil
			}
		}
	}
	return segment, "", nil
}

func parseVariable(raw string) (*Variable, error) {
	if !strings.HasSuffix(raw, "}") {
		return nil, fmt.Errorf("malformed variable %q: missing %q", raw, "}")
	}
	inner := raw[1 : len(raw)-1]

	fieldPart := inner
	var sub string
	if idx := strings.Index(inner, "="); idx >= 0 {
		fieldPart = inner[:idx]
		sub = inner[idx+1:]
	}

	path, err := ParseFieldPath(fieldPart)
	if err != nil {
		return nil, err
	}
	if path.Empty() {
		return nil, fmt.Errorf("malformed variable %q: empty field path", raw)
	}

	v := &Variable{FieldPath: path}
	if sub != "" {
		v.SubSegments = strings.Split(sub, "/")
		for _, s := range v.SubSegments {
			if s == "" {
				return nil, fmt.Errorf("malformed variable %q: empty sub-segment", raw)
			}
		}
		if len(v.SubSegments) > 1 {
			v.MultiSeg = true
		}
		for _, s := range v.SubSegments {
			if s == "**" {
				v.MultiSeg = true
			}
		}
	}
	return v, nil
}

// validate rejects templates the gateway itself would reject.
func (t *Template) validate() error {
	vars := t.Variables()
	for i, a := range vars {
		for j, b := range vars {
			if i == j {
				continue
			}
			if a.FieldPath.HasPrefix(b.FieldPath) || b.FieldPath.HasPrefix(a.FieldPath) {
				return fmt.Errorf(
					"path template %q binds %q and %q: neither may be a prefix of the other",
					t.Raw, a.FieldPath, b.FieldPath,
				)
			}
		}
	}
	return nil
}

// Variables returns every variable in the template, in path order.
func (t *Template) Variables() []*Variable {
	var out []*Variable
	for _, s := range t.Segments {
		if s.Variable != nil {
			out = append(out, s.Variable)
		}
	}
	return out
}

// HasMultiSegVariable reports whether any variable spans multiple segments,
// which decides whether the generated file needs the encodePathSegments helper.
func (t *Template) HasMultiSegVariable() bool {
	for _, v := range t.Variables() {
		if v.MultiSeg {
			return true
		}
	}
	return false
}
