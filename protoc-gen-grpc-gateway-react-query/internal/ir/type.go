package ir

import "google.golang.org/protobuf/reflect/protoreflect"

// Kind is the shape of a Type.
type Kind int

const (
	// KindScalar is a proto scalar that maps to a TypeScript primitive.
	KindScalar Kind = iota
	// KindEnum maps to a string literal union by default.
	KindEnum
	// KindMessage is a named message type, or an inlined well-known type.
	KindMessage
	// KindMap is a proto map field. JSON object keys are always strings.
	KindMap
	// KindAny is google.protobuf.Any, an open object with a @type discriminator.
	KindAny
)

// Cardinality is the proto cardinality, with maps separated out.
type Cardinality int

const (
	// Singular is a plain field.
	Singular Cardinality = iota
	// Repeated is a list field. Maps are KindMap with Singular cardinality.
	Repeated
)

// Type is a resolved proto type.
type Type struct {
	Kind Kind
	// Scalar is set when Kind is KindScalar.
	Scalar protoreflect.Kind
	// Enum is set when Kind is KindEnum.
	Enum *Enum
	// Message is set when Kind is KindMessage. It is nil for well-known types
	// that are never generated, in which case WKT identifies the type.
	Message *Message
	// MapKey and MapValue are set when Kind is KindMap.
	MapKey   *Type
	MapValue *Type
	// WKT classifies a well-known type, which is inlined as a TS primitive.
	WKT WKT
}

// IsMap reports whether the type is a map.
func (t *Type) IsMap() bool { return t != nil && t.Kind == KindMap }
