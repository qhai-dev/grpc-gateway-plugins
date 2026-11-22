package httprule

// BoundPaths returns the field paths bound by the template's variables, in path
// order. These fields are excluded from query parameters, together with all
// their descendants.
func (t *Template) BoundPaths() []FieldPath {
	vars := t.Variables()
	out := make([]FieldPath, 0, len(vars))
	for _, v := range vars {
		out = append(out, v.FieldPath)
	}
	return out
}
