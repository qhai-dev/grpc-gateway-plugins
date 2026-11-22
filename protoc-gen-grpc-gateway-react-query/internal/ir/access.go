package ir

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// Access is a chain of property names leading from the request root to a field.
type Access struct {
	// Hops are the TypeScript property names, in order.
	Hops []string
	// Type is the resolved type of the final hop.
	Type *Type
	// Repeated and Map mirror the final field's cardinality.
	Repeated bool
	Map      bool
}

// ResolveAccess walks a proto field path from msg, using the contract's field
// naming for every hop, and returns the resolved chain.
func (b *Builder) ResolveAccess(msg protoreflect.MessageDescriptor, path []string) (*Access, error) {
	acc := &Access{}
	current := msg
	for idx, name := range path {
		if current == nil {
			return nil, fmt.Errorf("cannot resolve %q beyond a non-message field", strings.Join(path, "."))
		}
		fd := current.Fields().ByName(protoreflect.Name(name))
		if fd == nil {
			fd = current.Fields().ByJSONName(name)
		}
		if fd == nil {
			return nil, fmt.Errorf("field %q not found in %s", name, current.FullName())
		}
		acc.Hops = append(acc.Hops, b.contract.FieldName(fd.JSONName(), string(fd.Name())))

		if idx == len(path)-1 {
			acc.Type = b.resolveType(fd)
			acc.Repeated = fd.IsList()
			acc.Map = fd.IsMap()
			break
		}
		if fd.Message() == nil || fd.IsList() || fd.IsMap() {
			return nil, fmt.Errorf("field %q in %s is not a singular message", name, current.FullName())
		}
		current = fd.Message()
	}
	return acc, nil
}

// String renders the access chain from `request`.
//
// Every hop after the first uses optional chaining, which is required rather than
// defensive: all singular fields in the generated types are optional, so an
// intermediate message may be absent.
func (a *Access) String() string {
	var sb strings.Builder
	sb.WriteString("request")
	for i, hop := range a.Hops {
		if i == 0 {
			sb.WriteString(PropertyAccess(hop))
			continue
		}
		if IsTSIdentifier(hop) {
			sb.WriteString("?.")
			sb.WriteString(hop)
			continue
		}
		sb.WriteString("?.")
		sb.WriteString(propertyOrBracket(hop))
	}
	return sb.String()
}

// From renders the access chain rooted at an arbitrary expression, e.g. `data`
// for response unwrapping.
func (a *Access) From(root string) string {
	full := a.String()
	return root + strings.TrimPrefix(full, "request")
}

// PropertyAccess renders `.name` or `["name"]`.
func PropertyAccess(name string) string {
	return propertyOrBracket(name)
}

func propertyOrBracket(name string) string {
	if IsTSIdentifier(name) {
		return "." + name
	}
	return "[" + Quote(name) + "]"
}

// IsTSIdentifier reports whether s can appear as a bare property name.
func IsTSIdentifier(s string) bool {
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

// Quote renders a double-quoted string literal.
func Quote(s string) string {
	var sb strings.Builder
	sb.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			sb.WriteString(`\"`)
		case '\\':
			sb.WriteString(`\\`)
		case '\n':
			sb.WriteString(`\n`)
		case '\r':
			sb.WriteString(`\r`)
		case '\t':
			sb.WriteString(`\t`)
		default:
			sb.WriteRune(r)
		}
	}
	sb.WriteByte('"')
	return sb.String()
}
