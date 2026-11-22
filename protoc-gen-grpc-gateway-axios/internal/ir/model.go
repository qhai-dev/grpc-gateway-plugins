// Package ir defines the intermediate representation the plugins consume.
//
// The three plugins share this IR deliberately (docs/v1.md §6.1): method names,
// client names, binding suffixes, response_body return types and query field
// lists must be decided once rather than three times with small differences.
package ir

import (
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/qhai-dev/protoc-gen-grpc-gateway-axios/internal/httprule"
)

// IR is the resolved view of one CodeGeneratorRequest.
type IR struct {
	// Files holds only the files that are actually generated, in request order.
	Files []*File
	// Request has every file, generated or not, for cross-file type lookups.
	Request []*protogen.File

	messages map[protoreflect.FullName]*Message
	enums    map[protoreflect.FullName]*Enum
}

// File is one proto file scheduled for generation.
type File struct {
	Proto      *protogen.File
	ProtoPath  string
	Package    string
	OutputBase string
	Comment    string

	Messages []*Message
	Enums    []*Enum
	Services []*Service

	ir *IR
}

// Message is a proto message with its TypeScript name resolved.
type Message struct {
	// TSName is the name used inside its own file: Outer_Inner for nested types.
	TSName string
	// FullName is the proto full name.
	FullName protoreflect.FullName
	Comment  string

	Fields   []*Field
	Oneofs   []*Oneof
	Messages []*Message
	Enums    []*Enum

	// WKT classifies well-known types, which are inlined as TS primitives and
	// never imported from a generated file.
	WKT WKT

	// File is nil for well-known types from google/protobuf, which are never
	// generated.
	File *File

	Proto *protogen.Message
}

// Enum is a proto enum with its TypeScript name resolved.
type Enum struct {
	TSName   string
	FullName protoreflect.FullName
	Comment  string
	Values   []*EnumValue
	File     *File

	Proto *protogen.Enum
}

// EnumValue is one member of an enum.
type EnumValue struct {
	Name    string
	Number  int32
	Comment string
}

// Field is one proto field with everything the generators need.
type Field struct {
	// Name is the property name emitted in TypeScript: json_name by default,
	// the proto name when use_proto_names=true.
	Name string
	// ProtoName is always the descriptor's name.
	ProtoName string
	// JSONName is the descriptor's canonical JSON name.
	JSONName string
	Comment  string

	Type        *Type
	Cardinality Cardinality
	// Oneof is the containing business oneof, or nil. Synthetic oneofs created
	// for proto3 optional fields are never reported here.
	Oneof *Oneof

	Proto *protogen.Field
}

// Oneof is a business oneof; synthetic ones are excluded.
type Oneof struct {
	Name   string
	Fields []*Field
}

// Service is one proto service.
type Service struct {
	Name    string
	Comment string
	Methods []*Method

	// ClientName is the generated class name, e.g. UserServiceClient.
	ClientName string
	// ClientVar is the TS variable name for a client instance, e.g. userService.
	ClientVar string
	// KeysName is the React Query key factory name, e.g. userServiceKeys.
	KeysName string

	Proto *protogen.Service
}

// Method is one RPC.
type Method struct {
	Name    string
	Comment string
	// ClientName is the generated TS method name, e.g. getUser or getUserBinding2.
	ClientName string
	// HookName is the React Query hook, e.g. useGetUser.
	HookName string

	Input  *Message
	Output *Message

	Streaming bool

	// Bindings holds every HTTP binding that will be generated. It is empty when
	// SkipReason is set.
	Bindings []*Binding

	// SkipReason is non-empty when the method is intentionally not generated.
	SkipReason string

	Proto *protogen.Method
}

// Binding is one HTTP binding of a method.
type Binding struct {
	HTTPMethod string
	Template   *httprule.Template
	// Body is the request field sent as the HTTP body. Empty means no body.
	Body httprule.FieldPath
	// BodyAll is true for body: "*".
	BodyAll bool
	// BodyAccess is the resolved access chain for a non-root body, so templates
	// emit `request.user?.profile` rather than `request["user.profile"]`.
	BodyAccess *Access
	// ResponseBody is the response field the gateway writes as the body.
	ResponseBody httprule.FieldPath
	// ResponseBodyAccess is its resolved access chain from the response message.
	ResponseBodyAccess *Access
	// PathVariables are the resolved template variables, in path order.
	PathVariables []*PathVariable
	QueryFields   []*QueryField
	IsPrimary     bool
	// ClientName is the generated method name for this binding.
	ClientName string
	// HookName is the React Query hook name for this binding.
	HookName string
}

// PathVariable is one path template variable with its access chain resolved.
type PathVariable struct {
	// FieldPath is the proto path the variable binds.
	FieldPath httprule.FieldPath
	// Access is the resolved chain from the request root.
	Access *Access
	// MultiSeg reports whether the value spans several URL segments, in which
	// case it must be encoded segment by segment.
	MultiSeg bool
}

// QueryField is one leaf field collected for query parameter encoding.
type QueryField struct {
	// Path is the proto field path from the request root, e.g. filter.status.
	Path httprule.FieldPath
	// Access is the resolved access chain from `request`.
	Access *Access
	// Key is the query parameter name, in proto names: the gateway resolves
	// either spelling, and proto names are what the field path is expressed in.
	Key string
	// Leaf is the resolved type of the leaf field.
	Leaf       *Type
	IsRepeated bool
	IsMap      bool
}

// Put registers a message in the lookup index.
func (i *IR) putMessage(m *Message) { i.messages[m.FullName] = m }

// putEnum registers an enum in the lookup index.
func (i *IR) putEnum(e *Enum) { i.enums[e.FullName] = e }

// LookupMessage resolves a proto full name.
func (i *IR) LookupMessage(name protoreflect.FullName) *Message { return i.messages[name] }

// LookupEnum resolves a proto full name.
func (i *IR) LookupEnum(name protoreflect.FullName) *Enum { return i.enums[name] }
