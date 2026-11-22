package httprule

import (
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// Leaf is one query-encodable leaf field reached from the request root.
type Leaf struct {
	// Path is the proto field path from the request root, in proto names.
	Path FieldPath
	// Field is the descriptor of the leaf field.
	Field protoreflect.FieldDescriptor
	// ParentPath is the chain of singular message fields leading to the leaf's
	// parent, so templates can emit request.a?.b.
	ParentPath FieldPath
	// SkipReason is non-empty when the field cannot be expressed as a query
	// parameter at all; the client must not emit it.
	SkipReason string
}

// QueryLeaves collects the leaf fields of msg that the gateway can populate from
// query parameters, excluding everything bound by the path template and covered
// by the body.
//
// The traversal mirrors runtime.PopulateQueryParameters:
//
//   - singular message fields are recursed into and become dotted names,
//   - repeated scalars and enums become same-name repeated parameters,
//   - maps become name[key]=value and are never expanded further,
//   - well-known types are single values parsed from one string, so they are
//     leaves even though they are messages,
//   - repeated non-WKT messages cannot be expressed (the gateway's parser
//     rejects them), so they are returned with SkipReason set.
//
// Two deliberate differences from the reference implementations reviewed in
// docs/v1-review.md: exclusions are prefix-based, so binding "user" also
// excludes "user.name"; and cycle protection is per recursion path rather than a
// global visited set, so a message type reached through two different fields
// expands in both places.
// QueryLeaves collects the leaf fields of msg that the gateway can populate from
// query parameters, excluding everything bound by boundPaths (the path
// template's variables) and covered by body.
func QueryLeaves(
	msg protoreflect.MessageDescriptor,
	boundPaths []FieldPath,
	body FieldPath,
	bodyAll bool,
) ([]Leaf, error) {
	if bodyAll {
		return nil, nil
	}

	skip := make([]FieldPath, 0, len(boundPaths)+1)
	skip = append(skip, boundPaths...)
	if !body.Empty() && !body.IsRoot() {
		skip = append(skip, body)
	}

	var leaves []Leaf
	visited := make([]protoreflect.FullName, 0, 8)
	if err := collect(msg, nil, skip, &visited, &leaves); err != nil {
		return nil, err
	}
	return leaves, nil
}

func collect(
	msg protoreflect.MessageDescriptor,
	fieldPrefix FieldPath,
	skip []FieldPath,
	visited *[]protoreflect.FullName,
	leaves *[]Leaf,
) error {
	for _, seen := range *visited {
		if seen == msg.FullName() {
			return nil
		}
	}
	*visited = append(*visited, msg.FullName())
	defer func() { *visited = (*visited)[:len(*visited)-1] }()

	fields := msg.Fields()
	for i := 0; i < fields.Len(); i++ {
		f := fields.Get(i)

		fp := make(FieldPath, len(fieldPrefix)+1)
		copy(fp, fieldPrefix)
		fp[len(fieldPrefix)] = string(f.Name())

		if excluded(fp, skip) {
			continue
		}

		switch {
		case f.IsMap():
			leaf := Leaf{Path: fp, Field: f, ParentPath: fieldPrefix}
			if f.MapValue().Kind() == protoreflect.MessageKind && !IsWKTQueryLeaf(f.MapValue().Message()) {
				leaf.SkipReason = fmt.Sprintf(
					"map value type %s cannot be parsed by grpc-gateway from a query parameter",
					f.MapValue().Message().FullName(),
				)
			}
			*leaves = append(*leaves, leaf)

		case f.IsList():
			leaf := Leaf{Path: fp, Field: f, ParentPath: fieldPrefix}
			if f.Message() != nil && !IsWKTQueryLeaf(f.Message()) {
				leaf.SkipReason = fmt.Sprintf(
					"repeated message type %s cannot be parsed by grpc-gateway from a query parameter",
					f.Message().FullName(),
				)
			}
			*leaves = append(*leaves, leaf)

		case f.Message() != nil && !IsWKTQueryLeaf(f.Message()):
			if err := collect(f.Message(), fp, skip, visited, leaves); err != nil {
				return err
			}

		default:
			*leaves = append(*leaves, Leaf{Path: fp, Field: f, ParentPath: fieldPrefix})
		}
	}
	return nil
}

func excluded(fp FieldPath, skip []FieldPath) bool {
	for _, s := range skip {
		if fp.HasPrefix(s) {
			return true
		}
	}
	return false
}

// wktQueryLeaves lists the well-known types the gateway parses from a single
// query string. Flattening them would emit parameters the gateway cannot read.
var wktQueryLeaves = map[string]bool{
	"google.protobuf.Timestamp":   true,
	"google.protobuf.Duration":    true,
	"google.protobuf.FieldMask":   true,
	"google.protobuf.Any":         true,
	"google.protobuf.Empty":       true,
	"google.protobuf.Struct":      true,
	"google.protobuf.Value":       true,
	"google.protobuf.ListValue":   true,
	"google.protobuf.DoubleValue": true,
	"google.protobuf.FloatValue":  true,
	"google.protobuf.Int64Value":  true,
	"google.protobuf.UInt64Value": true,
	"google.protobuf.Int32Value":  true,
	"google.protobuf.UInt32Value": true,
	"google.protobuf.BoolValue":   true,
	"google.protobuf.StringValue": true,
	"google.protobuf.BytesValue":  true,
}

// IsWKTQueryLeaf reports whether a well-known type is a single query value.
func IsWKTQueryLeaf(msg protoreflect.MessageDescriptor) bool {
	return msg != nil && wktQueryLeaves[string(msg.FullName())]
}

// EnsureLeavesAreConsistent is a self-check: every leaf path must resolve by
// walking proto field names from the request root.
func EnsureLeavesAreConsistent(msg protoreflect.MessageDescriptor, leaves []Leaf) error {
	for _, leaf := range leaves {
		current := msg
		for i, name := range leaf.Path {
			f := lookupField(current, name)
			if f == nil {
				return fmt.Errorf("leaf %q: field %q not found in %s", leaf.Path, name, current.FullName())
			}
			if i == len(leaf.Path)-1 {
				break
			}
			if f.Message() == nil {
				return fmt.Errorf("leaf %q: %q is not a message", leaf.Path, name)
			}
			current = f.Message()
		}
	}
	return nil
}
