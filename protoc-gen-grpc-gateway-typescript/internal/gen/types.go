// Package gen renders the TypeScript types file from the IR.
package gen

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/qhai-dev/protoc-gen-grpc-gateway-typescript/internal/contract"
	"github.com/qhai-dev/protoc-gen-grpc-gateway-typescript/internal/ir"
	"github.com/qhai-dev/protoc-gen-grpc-gateway-typescript/internal/ts"
	"github.com/qhai-dev/protoc-gen-grpc-gateway-typescript/internal/typeref"
)

const pluginName = "protoc-gen-grpc-gateway-typescript"

// Options are the plugin-specific parameters (docs/v1.md §9.2).
type Options struct {
	EnumStyle   string // union | const | enum
	OneofStyle  string // union | flat
	EmitComment bool
}

// DefaultOptions returns the documented defaults.
func DefaultOptions() *Options {
	return &Options{EnumStyle: "union", OneofStyle: "union", EmitComment: true}
}

// Set applies one plugin parameter, reporting whether it belongs to this plugin.
func (o *Options) Set(name, value string) (bool, error) {
	switch name {
	case "enum_style":
		o.EnumStyle = value
	case "oneof_style":
		o.OneofStyle = value
	case "emit_comments":
		b, err := parseBool(value)
		if err != nil {
			return true, fmt.Errorf("emit_comments: %w", err)
		}
		o.EmitComment = b
	default:
		return false, nil
	}
	return true, nil
}

// Validate rejects unsupported values.
func (o *Options) Validate() error {
	switch o.EnumStyle {
	case "union", "const", "enum":
	default:
		return fmt.Errorf("enum_style must be union, const or enum, got %q", o.EnumStyle)
	}
	switch o.OneofStyle {
	case "union", "flat":
	default:
		return fmt.Errorf("oneof_style must be union or flat, got %q", o.OneofStyle)
	}
	return nil
}

func parseBool(value string) (bool, error) {
	switch strings.ToLower(value) {
	case "true", "1", "yes":
		return true, nil
	case "false", "0", "no":
		return false, nil
	}
	return false, fmt.Errorf("expected true or false, got %q", value)
}

// Generator renders one types file.
type Generator struct {
	contract *contract.Options
	opts     *Options

	file    *ts.File
	render  *typeref.Renderer
	current *ir.File
	// local maps every type declared in this file to its TypeScript name, so a
	// field can reference a type declared later without an import.
	local map[protoreflect.FullName]string
}

// New starts a types generator.
func New(c *contract.Options, opts *Options) *Generator {
	return &Generator{contract: c, opts: opts}
}

// Generate renders the types file for one proto file.
func (g *Generator) Generate(f *ir.File) (string, error) {
	g.current = f
	g.file = ts.NewFile(contract.Header(pluginName, f.ProtoPath))
	g.local = map[protoreflect.FullName]string{}

	for _, m := range allMessages(f.Messages) {
		g.local[m.FullName] = m.TSName
	}
	for _, e := range allEnums(f) {
		g.local[e.FullName] = e.TSName
	}
	g.render = typeref.New(g.contract, g.file, f.ProtoPath, g.local)

	wrote := false
	for _, e := range f.Enums {
		if wrote {
			g.file.P("")
		}
		g.writeEnum(e)
		wrote = true
	}
	for _, m := range f.Messages {
		if wrote {
			g.file.P("")
		}
		g.writeMessage(m)
		wrote = true
	}

	if g.render.WritesJSONAlias() {
		g.file.P("")
		g.render.WriteJSONAlias()
	}

	return g.file.WriteString(), nil
}

// writeEnum emits one enum declaration.
func (g *Generator) writeEnum(e *ir.Enum) {
	g.comment(e.Comment, g.file.Depth())

	switch g.opts.EnumStyle {
	case "const":
		g.file.P("export const %s = {", e.TSName)
		g.file.Indent(func() {
			for _, v := range e.Values {
				g.comment(v.Comment, g.file.Depth())
				g.file.P("%s: %q,", v.Name, v.Name)
			}
		})
		g.file.P("} as const;")
		g.file.P("")
		g.file.P("export type %s = (typeof %s)[keyof typeof %s];", e.TSName, e.TSName, e.TSName)

	case "enum":
		g.file.P("export enum %s {", e.TSName)
		g.file.Indent(func() {
			for _, v := range e.Values {
				g.comment(v.Comment, g.file.Depth())
				g.file.P("%s = %q,", v.Name, v.Name)
			}
		})
		g.file.P("}")

	default: // union
		g.file.P("export type %s =", e.TSName)
		g.file.Indent(func() {
			for i, v := range e.Values {
				if i == len(e.Values)-1 {
					g.file.P("| %q;", v.Name)
					continue
				}
				g.file.P("| %q", v.Name)
			}
		})
	}
}

// writeMessage emits one message declaration, then its nested enums and
// messages in declaration order.
func (g *Generator) writeMessage(m *ir.Message) {
	g.comment(m.Comment, g.file.Depth())

	if g.opts.OneofStyle == "union" && len(m.Oneofs) > 0 {
		g.writeOneofUnion(m)
	} else {
		g.writeFlatObject(m)
	}

	for _, e := range m.Enums {
		g.file.P("")
		g.writeEnum(e)
	}
	for _, nested := range m.Messages {
		g.file.P("")
		g.writeMessage(nested)
	}
}

// writeFlatObject emits the plain object form: every field optional.
func (g *Generator) writeFlatObject(m *ir.Message) {
	if len(m.Fields) == 0 {
		// An empty message is `{}` on the wire, which leaves no members to name.
		g.file.P("export type %s = Record<string, never>;", m.TSName)
		return
	}
	g.file.P("export type %s = {", m.TSName)
	g.file.Indent(func() {
		for _, f := range m.Fields {
			g.comment(f.Comment, g.file.Depth())
			g.file.P("%s?: %s;", ts.Property(f.Name), g.fieldType(f))
		}
	})
	g.file.P("};")
}

// writeOneofUnion emits the mutually exclusive union form: each member of a oneof
// becomes an alternative that sets it and forbids its siblings with `?: never`.
//
// proto3 has an important case the union must represent: a oneof may be *unset*, and
// the wire form is then simply the message with none of those fields present. So the
// union includes a "no member chosen" arm. Without it, a response that leaves the
// oneof unset would not type-check against its own generated type, which is the
// situation a server returning an unstamped Book would create.
func (g *Generator) writeOneofUnion(m *ir.Message) {
	alternatives := oneofAlternatives(m)
	if alternatives == nil {
		// The cartesian product of several oneofs is too large to stay readable.
		// Flat optional members still describe the wire shape and still compile;
		// only compile-time exclusivity is lost, which the comment records.
		g.file.P("// %s declares %d oneofs; the union form needs too many arms, so members", m.TSName, len(m.Oneofs))
		g.file.P("// are emitted as optional fields (oneof_style=flat behaviour).")
		g.writeFlatObject(m)
		return
	}
	if len(alternatives) == 1 && len(m.Oneofs) == 1 && len(m.Oneofs[0].Fields) == 1 {
		// A single oneof with a single member is just an optional field: the flat form
		// already expresses both "set" and "unset".
		g.writeFlatObject(m)
		return
	}

	plain := make([]*ir.Field, 0, len(m.Fields))
	for _, f := range m.Fields {
		if f.Oneof == nil {
			plain = append(plain, f)
		}
	}
	members := make([]*ir.Field, 0, len(m.Fields))
	for _, o := range m.Oneofs {
		members = append(members, o.Fields...)
	}

	g.file.P("export type %s =", m.TSName)
	g.file.Indent(func() {
		// One arm per combination of set members, plus the all-unset arm.
		total := len(alternatives) + 1
		for i, arm := range alternatives {
			terminator := ""
			if i == total-1 {
				terminator = ";"
			}
			g.writeArm(plain, members, arm, terminator)
		}
		// The all-unset arm: every oneof member is absent. This is the shape of a
		// message whose oneof was never set, which proto3 permits.
		g.writeArm(plain, members, nil, ";")
	})
}

// writeArm emits one alternative of a oneof union. Passing an empty set produces the
// arm in which no oneof member is set, which proto3 permits.
func (g *Generator) writeArm(plain, members, set []*ir.Field, terminator string) {
	chosen := make(map[string]bool, len(set))
	for _, f := range set {
		chosen[f.Name] = true
	}

	g.file.P("| {")
	g.file.Indent(func() {
		for _, f := range plain {
			g.file.P("%s?: %s;", ts.Property(f.Name), g.fieldType(f))
		}
		for _, f := range set {
			g.file.P("%s: %s;", ts.Property(f.Name), g.fieldType(f))
		}
		for _, f := range members {
			if chosen[f.Name] {
				continue
			}
			g.file.P("%s?: never;", ts.Property(f.Name))
		}
	})
	if terminator == "" {
		g.file.P("}")
		return
	}
	g.file.P("}%s", terminator)
}

// oneofAlternatives enumerates every combination of oneof members that the union
// must cover. It returns nil when the product would be unreasonably large.
func oneofAlternatives(m *ir.Message) [][]*ir.Field {
	const maxAlternatives = 64

	alternatives := [][]*ir.Field{{}}
	for _, o := range m.Oneofs {
		if len(o.Fields) == 0 {
			continue
		}
		next := make([][]*ir.Field, 0, len(alternatives)*len(o.Fields))
		for _, prefix := range alternatives {
			for _, f := range o.Fields {
				arm := make([]*ir.Field, len(prefix), len(prefix)+1)
				copy(arm, prefix)
				next = append(next, append(arm, f))
			}
		}
		if len(next) > maxAlternatives {
			return nil
		}
		alternatives = next
	}
	return alternatives
}

// fieldType renders a field's TypeScript type.
func (g *Generator) fieldType(f *ir.Field) string {
	return g.render.Type(f.Type, f.Cardinality == ir.Repeated)
}

// comment writes a JSDoc block at the current indent level.
func (g *Generator) comment(text string, depth int) {
	if !g.opts.EmitComment {
		return
	}
	doc := ts.JSDoc(text, strings.Repeat("  ", depth))
	if doc == "" {
		return
	}
	// JSDoc is rendered with its own indent, so it must not be re-indented.
	g.file.PVerbatim(doc)
}

// allMessages lists every message of a file, nested ones included, in
// declaration order.
func allMessages(msgs []*ir.Message) []*ir.Message {
	var out []*ir.Message
	var walk func([]*ir.Message)
	walk = func(list []*ir.Message) {
		for _, m := range list {
			out = append(out, m)
			walk(m.Messages)
		}
	}
	walk(msgs)
	return out
}

// allEnums lists every enum of a file, nested ones included.
func allEnums(f *ir.File) []*ir.Enum {
	out := append([]*ir.Enum{}, f.Enums...)
	var walk func([]*ir.Message)
	walk = func(msgs []*ir.Message) {
		for _, m := range msgs {
			out = append(out, m.Enums...)
			walk(m.Messages)
		}
	}
	walk(f.Messages)
	return out
}
