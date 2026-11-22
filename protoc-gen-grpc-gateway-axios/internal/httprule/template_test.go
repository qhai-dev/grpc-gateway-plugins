package httprule

import "testing"

func TestParseTemplateSimple(t *testing.T) {
	tmpl, err := ParseTemplate("/v1/users/{id}")
	if err != nil {
		t.Fatalf("ParseTemplate: %v", err)
	}
	if len(tmpl.Segments) != 3 {
		t.Fatalf("segments = %d, want 3", len(tmpl.Segments))
	}
	if tmpl.Segments[0].Literal != "v1" || tmpl.Segments[1].Literal != "users" {
		t.Errorf("literals = %q, %q", tmpl.Segments[0].Literal, tmpl.Segments[1].Literal)
	}
	v := tmpl.Segments[2].Variable
	if v == nil {
		t.Fatal("expected a variable segment")
	}
	if got := v.FieldPath.String(); got != "id" {
		t.Errorf("field path = %q", got)
	}
	if v.MultiSeg {
		t.Error("a bare {id} must be single-segment")
	}
}

func TestParseTemplateMultiSegment(t *testing.T) {
	// The value of {name=shippers/*} contains a slash, so it must be encoded
	// segment by segment rather than as one token.
	tmpl, err := ParseTemplate("/v1/{name=shippers/*}")
	if err != nil {
		t.Fatalf("ParseTemplate: %v", err)
	}
	vars := tmpl.Variables()
	if len(vars) != 1 {
		t.Fatalf("variables = %d, want 1", len(vars))
	}
	if !vars[0].MultiSeg {
		t.Error("expected MultiSeg for a sub-template containing a slash")
	}
	if got := vars[0].FieldPath.String(); got != "name" {
		t.Errorf("field path = %q", got)
	}
}

func TestParseTemplateDoubleStar(t *testing.T) {
	tmpl, err := ParseTemplate("/v1/{path=**}")
	if err != nil {
		t.Fatalf("ParseTemplate: %v", err)
	}
	if !tmpl.HasMultiSegVariable() {
		t.Error("** must be multi-segment")
	}
}

func TestParseTemplateVerb(t *testing.T) {
	// The verb sits outside the braces and must survive parsing.
	tmpl, err := ParseTemplate("/v1/users/{id}:activate")
	if err != nil {
		t.Fatalf("ParseTemplate: %v", err)
	}
	if tmpl.Verb != "activate" {
		t.Errorf("verb = %q, want activate", tmpl.Verb)
	}
	if len(tmpl.Segments) != 3 {
		t.Fatalf("segments = %d", len(tmpl.Segments))
	}
	if v := tmpl.Segments[2].Variable; v == nil || v.FieldPath.String() != "id" {
		t.Errorf("last segment should be the {id} variable, got %+v", tmpl.Segments[2])
	}
}

func TestParseTemplateVerbWithSubTemplate(t *testing.T) {
	tmpl, err := ParseTemplate("/v1/{name=shippers/*}:activate")
	if err != nil {
		t.Fatalf("ParseTemplate: %v", err)
	}
	if tmpl.Verb != "activate" {
		t.Errorf("verb = %q", tmpl.Verb)
	}
	vars := tmpl.Variables()
	if len(vars) != 1 || vars[0].FieldPath.String() != "name" || !vars[0].MultiSeg {
		t.Errorf("variables = %+v", vars)
	}
}

func TestParseTemplateCustomVerb(t *testing.T) {
	// A custom verb may itself contain a colon-free word with dots and dashes.
	tmpl, err := ParseTemplate("/v1/users/{id}:batch-get")
	if err != nil {
		t.Fatalf("ParseTemplate: %v", err)
	}
	if tmpl.Verb != "batch-get" {
		t.Errorf("verb = %q", tmpl.Verb)
	}
}

func TestParseTemplateNestedField(t *testing.T) {
	tmpl, err := ParseTemplate("/v1/users/{user.id}")
	if err != nil {
		t.Fatalf("ParseTemplate: %v", err)
	}
	vars := tmpl.Variables()
	if len(vars) != 1 || vars[0].FieldPath.String() != "user.id" {
		t.Errorf("variables = %+v", vars)
	}
}

func TestParseTemplateRoot(t *testing.T) {
	tmpl, err := ParseTemplate("/")
	if err != nil {
		t.Fatalf("ParseTemplate(/): %v", err)
	}
	if len(tmpl.Segments) != 0 {
		t.Errorf("segments = %d, want 0", len(tmpl.Segments))
	}
}

func TestParseTemplateErrors(t *testing.T) {
	for _, in := range []string{
		"",          // empty
		"v1/users",  // no leading slash
		"/v1/{id",   // unbalanced brace
		"/v1/id}",   // unbalanced brace
		"/v1/{}",    // empty field path
		"/v1/{id}:", // empty verb
		"/v1/{a=}x", // empty sub-segment
	} {
		if _, err := ParseTemplate(in); err == nil {
			t.Errorf("ParseTemplate(%q) succeeded, want an error", in)
		}
	}
}

func TestParseTemplateRejectsPrefixVariables(t *testing.T) {
	// grpc-gateway rejects templates where one bound field is a prefix of
	// another: /v1/{a}/{a.b} would be ambiguous.
	if _, err := ParseTemplate("/v1/{user}/{user.id}"); err == nil {
		t.Error("expected an error for overlapping path variables")
	}
}

func TestParseFieldPath(t *testing.T) {
	p, err := ParseFieldPath("user.profile.name")
	if err != nil {
		t.Fatalf("ParseFieldPath: %v", err)
	}
	if p.String() != "user.profile.name" {
		t.Errorf("path = %q", p)
	}

	for _, in := range []string{"user..name", ".name", "name."} {
		if _, err := ParseFieldPath(in); err == nil {
			t.Errorf("ParseFieldPath(%q) succeeded, want an error", in)
		}
	}

	root, err := ParseFieldPath("*")
	if err != nil {
		t.Fatalf("ParseFieldPath(*): %v", err)
	}
	if !root.IsRoot() {
		t.Error("* must be the root path")
	}
}

func TestFieldPathHasPrefix(t *testing.T) {
	cases := []struct {
		path, prefix string
		want         bool
	}{
		{"user", "user", true},
		{"user.name", "user", true},
		{"user.profile.name", "user", true},
		{"user", "user.name", false},
		{"username", "user", false},
		{"a.b", "a.c", false},
	}
	for _, c := range cases {
		p, _ := ParseFieldPath(c.path)
		prefix, _ := ParseFieldPath(c.prefix)
		if got := p.HasPrefix(prefix); got != c.want {
			t.Errorf("(%q).HasPrefix(%q) = %v, want %v", c.path, c.prefix, got, c.want)
		}
	}
}
