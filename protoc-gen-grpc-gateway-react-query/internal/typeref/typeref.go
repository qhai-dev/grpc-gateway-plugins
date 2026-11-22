// Package typeref renders proto types as TypeScript type expressions and records
// the namespace imports those expressions need.
//
// All three plugins share this renderer through the contract: the Axios and
// React Query files must name types exactly as the types file declares them, or
// the generated SDK will not compile.
package typeref

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/qhai-dev/protoc-gen-grpc-gateway-react-query/internal/contract"
	"github.com/qhai-dev/protoc-gen-grpc-gateway-react-query/internal/ir"
	"github.com/qhai-dev/protoc-gen-grpc-gateway-react-query/internal/ts"
)

// Renderer renders type expressions for one output file.
type Renderer struct {
	contract *contract.Options
	file     *ts.File
	// currentProto is the proto file being generated; types declared in the file
	// it corresponds to are rendered through the local-name table.
	currentProto string
	// local maps a proto type name to the TypeScript name declared in the types
	// file of the same proto. In a types file that is every declaration; in an
	// Axios or React Query file it is empty, because those files declare no types.
	local map[protoreflect.FullName]string
	// NeedJSON records that a JsonValue alias must be emitted in the file.
	NeedJSON bool
}

// New starts a renderer for one output file.
func New(c *contract.Options, file *ts.File, currentProto string, local map[protoreflect.FullName]string) *Renderer {
	return &Renderer{contract: c, file: file, currentProto: currentProto, local: local}
}

// Type renders a field's type, adding [] for repeated fields.
//
// Parentheses around the element are only emitted when the element contains a
// union or an object literal, where `[]` would otherwise bind wrongly.
func (r *Renderer) Type(t *ir.Type, repeated bool) string {
	base := r.Base(t)
	if !repeated {
		return base
	}
	if strings.ContainsAny(base, "|&") || strings.HasPrefix(base, "{") {
		return "(" + base + ")[]"
	}
	return base + "[]"
}

// Base renders the TypeScript type for a proto type.
func (r *Renderer) Base(t *ir.Type) string {
	if t == nil {
		return "unknown"
	}
	// A well-known type is inlined before its Kind is considered: Timestamp is a
	// message on the wire but a string in the JSON form this SDK speaks, and
	// Struct/Value are messages that map to JsonValue.
	if t.WKT != ir.WKTNone {
		return r.wkt(t.WKT)
	}
	switch t.Kind {
	case ir.KindMap:
		// JSON object keys are always strings, whatever the proto key type is.
		return fmt.Sprintf("Record<string, %s>", r.Base(t.MapValue))

	case ir.KindAny:
		return `{ "@type": string; [key: string]: unknown }`

	case ir.KindEnum:
		return r.enum(t.Enum)

	case ir.KindMessage:
		return r.message(t.Message)

	default:
		return r.scalar(t)
	}
}

// WKT renders a well-known type as the TypeScript type protojson produces.
func (r *Renderer) WKT(w ir.WKT) string { return r.wkt(w) }

func (r *Renderer) wkt(w ir.WKT) string {
	switch w {
	case ir.WKTTimestamp:
		return "string" // RFC3339
	case ir.WKTDuration:
		return "string" // e.g. "3.5s"
	case ir.WKTFieldMask:
		return "string" // comma-separated paths
	case ir.WKTEmpty:
		return "Record<string, never>"
	case ir.WKTStruct:
		r.NeedJSON = true
		return "Record<string, JsonValue>"
	case ir.WKTValue:
		r.NeedJSON = true
		return "JsonValue"
	case ir.WKTListValue:
		r.NeedJSON = true
		return "JsonValue[]"
	case ir.WKTAny:
		return `{ "@type": string; [key: string]: unknown }`
	case ir.WKTDoubleValue, ir.WKTFloatValue, ir.WKTInt32Value, ir.WKTUInt32Value:
		return "number | null"
	case ir.WKTInt64Value, ir.WKTUInt64Value:
		if r.contract.LongType == "number" {
			return "number | null"
		}
		return "string | null"
	case ir.WKTBoolValue:
		return "boolean | null"
	case ir.WKTStringValue, ir.WKTBytesValue:
		return "string | null"
	}
	return "unknown"
}

func (r *Renderer) scalar(t *ir.Type) string {
	switch t.Scalar {
	case protoreflect.DoubleKind, protoreflect.FloatKind,
		protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind,
		protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return "number"

	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind,
		protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		// Matches protojson, which writes 64-bit integers as strings to avoid
		// precision loss past Number.MAX_SAFE_INTEGER.
		if r.contract.LongType == "number" {
			return "number"
		}
		return "string"

	case protoreflect.BoolKind:
		return "boolean"

	case protoreflect.StringKind, protoreflect.BytesKind:
		// bytes is base64 in protojson.
		return "string"

	case protoreflect.EnumKind:
		// google.protobuf.NullValue is the only enum rendered without a name.
		return "null"

	default:
		return "unknown"
	}
}

// message renders a message reference, importing it from another generated file
// when needed.
func (r *Renderer) message(m *ir.Message) string {
	if m == nil {
		return "unknown"
	}
	if name, ok := r.local[m.FullName]; ok {
		return name
	}
	if m.File == nil {
		// A type from a proto the contract never generates, and not a
		// well-known type either: nothing sane to name it with.
		return "unknown"
	}
	spec, alias, ok := r.contract.TypesImportPath(r.currentProto, m.File.ProtoPath)
	if !ok {
		return "unknown"
	}
	r.file.AddNamespaceImport(spec, alias)
	return alias + "." + m.TSName
}

// Message renders a message reference by proto name, for callers that only have
// the IR node.
func (r *Renderer) Message(m *ir.Message) string { return r.message(m) }

// enum renders an enum reference.
func (r *Renderer) enum(e *ir.Enum) string {
	if e == nil || ir.IsNullValue(e.FullName) {
		return "null"
	}
	if name, ok := r.local[e.FullName]; ok {
		return name
	}
	if e.File == nil {
		return "unknown"
	}
	spec, alias, ok := r.contract.TypesImportPath(r.currentProto, e.File.ProtoPath)
	if !ok {
		return "unknown"
	}
	r.file.AddNamespaceImport(spec, alias)
	return alias + "." + e.TSName
}

// Enum renders an enum reference by IR node.
func (r *Renderer) Enum(e *ir.Enum) string { return r.enum(e) }

// WritesJSONAlias reports whether Struct/Value/ListValue were rendered, meaning
// the file needs the JsonValue helper.
func (r *Renderer) WritesJSONAlias() bool { return r.NeedJSON }

// WriteJSONAlias emits the recursive JsonValue helper at the current indent.
func (r *Renderer) WriteJSONAlias() {
	r.file.P("type JsonValue =")
	r.file.Indent(func() {
		r.file.P("| null")
		r.file.P("| boolean")
		r.file.P("| number")
		r.file.P("| string")
		r.file.P("| JsonValue[]")
		r.file.P("| { [key: string]: JsonValue };")
	})
}
