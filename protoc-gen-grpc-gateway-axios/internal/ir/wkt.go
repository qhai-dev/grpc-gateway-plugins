package ir

import "google.golang.org/protobuf/reflect/protoreflect"

// WKT classifies a well-known type. Well-known types are inlined as TypeScript
// primitives: no file is generated for google/protobuf/*.proto and no import is
// emitted for them (docs/v1.md §3.5).
type WKT int

const (
	// WKTNone is an ordinary message.
	WKTNone WKT = iota
	WKTTimestamp
	WKTDuration
	WKTFieldMask
	WKTStruct
	WKTValue
	WKTListValue
	WKTEmpty
	WKTAny
	WKTDoubleValue
	WKTFloatValue
	WKTInt64Value
	WKTUInt64Value
	WKTInt32Value
	WKTUInt32Value
	WKTBoolValue
	WKTStringValue
	WKTBytesValue
)

// ClassifyMessage reports the well-known-type flavour of a message.
func ClassifyMessage(msg protoreflect.MessageDescriptor) WKT {
	if msg == nil {
		return WKTNone
	}
	return messageWKT[string(msg.FullName())]
}

var messageWKT = map[string]WKT{
	"google.protobuf.Timestamp":   WKTTimestamp,
	"google.protobuf.Duration":    WKTDuration,
	"google.protobuf.FieldMask":   WKTFieldMask,
	"google.protobuf.Struct":      WKTStruct,
	"google.protobuf.Value":       WKTValue,
	"google.protobuf.ListValue":   WKTListValue,
	"google.protobuf.Empty":       WKTEmpty,
	"google.protobuf.Any":         WKTAny,
	"google.protobuf.DoubleValue": WKTDoubleValue,
	"google.protobuf.FloatValue":  WKTFloatValue,
	"google.protobuf.Int64Value":  WKTInt64Value,
	"google.protobuf.UInt64Value": WKTUInt64Value,
	"google.protobuf.Int32Value":  WKTInt32Value,
	"google.protobuf.UInt32Value": WKTUInt32Value,
	"google.protobuf.BoolValue":   WKTBoolValue,
	"google.protobuf.StringValue": WKTStringValue,
	"google.protobuf.BytesValue":  WKTBytesValue,
}

// IsWrapper reports whether the WKT is one of the google.protobuf.*Value
// wrappers, which map to `T | null` rather than to a named type.
func (w WKT) IsWrapper() bool {
	switch w {
	case WKTDoubleValue, WKTFloatValue, WKTInt64Value, WKTUInt64Value,
		WKTInt32Value, WKTUInt32Value, WKTBoolValue, WKTStringValue, WKTBytesValue:
		return true
	}
	return false
}

// NeedsJsonValue reports whether rendering this type requires the JsonValue
// helper alias.
func (w WKT) NeedsJsonValue() bool {
	switch w {
	case WKTStruct, WKTValue, WKTListValue:
		return true
	}
	return false
}

// IsNullValue reports whether an enum is google.protobuf.NullValue, which maps to
// the TypeScript null type.
func IsNullValue(name protoreflect.FullName) bool {
	return name == "google.protobuf.NullValue"
}
