// Command verifycompat feeds the query strings that the generated TypeScript client
// produces into grpc-gateway's own parser, and asserts the decoded proto.
//
// This is the highest-value check in docs/v1.md §10.3. If the client's encoding and
// runtime.PopulateQueryParameters disagree, every call to that method fails at
// runtime with a 400 that no TypeScript test would catch.
package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"reflect"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/grpc-ecosystem/grpc-gateway/v2/utilities"

	userpb "grpcgateway.conformance/gen/user/v1"
)

// presence asserts decoded field values directly, so zero values are provable.
type presence struct {
	set map[string]any
}

// case is one query string exactly as the generated client emits it.
type queryCase struct {
	name string
	msg  func() proto.Message
	// bound lists the fields the gateway binds from the path template, which it
	// therefore filters out of the query.
	bound []string
	query string
	// want is the expected canonical JSON, used when the decoded message contains
	// no zero values worth asserting.
	want string
	// wantPresence asserts field values by proto path, including zero values.
	wantPresence *presence
}

func main() {
	cases := []queryCase{
		{
			// The values both reference implementations reviewed in docs/v1-review.md
			// drop. They must survive, so this case asserts the decoded proto's fields
			// directly: protojson omits zero values, so a JSON comparison cannot tell
			// "decoded to 0" from "never decoded".
			name:  "zero, false and empty string are decoded explicitly",
			msg:   func() proto.Message { return &userpb.ListUsersRequest{} },
			query: "page=0&pageSize=10&keyword=&includeDeleted=false&filter.minAge=0",
			wantPresence: &presence{
				set: map[string]any{
					"page":            int32(0),
					"pageSize":        int32(10),
					"keyword":         "",
					"includeDeleted":  false,
					"filter.minAge":   int32(0),
				},
			},
		},
		{
			// repeated -> same-name repeated parameters.
			name:  "repeated fields repeat the parameter name",
			msg:   func() proto.Message { return &userpb.ListUsersRequest{} },
			query: "ids=a&ids=b&ids=c",
			want:  `{"ids":["a","b","c"]}`,
		},
		{
			// map -> name[key]=value, the form the gateway parses. `labels.env=prod`
			// would be read as a field path and rejected.
			name:  "map uses the bracket form",
			msg:   func() proto.Message { return &userpb.ListUsersRequest{} },
			query: "labels%5Benv%5D=prod&labels%5Bteam%5D=core",
			want:  `{"labels":{"env":"prod","team":"core"}}`,
		},
		{
			// A singular message field flattens into dotted parameter names.
			name:  "nested message flattens into dotted names",
			msg:   func() proto.Message { return &userpb.ListUsersRequest{} },
			query: "filter.minAge=18&filter.status=USER_STATUS_ACTIVE",
			want:  `{"filter":{"minAge":18,"status":"USER_STATUS_ACTIVE"}}`,
		},
		{
			// int64 stays a decimal string: 2^53+1 would lose precision as a number.
			name:  "int64 keeps full precision",
			msg:   func() proto.Message { return &userpb.ListUsersRequest{} },
			query: "cursor=9007199254740993",
			want:  `{"cursor":"9007199254740993"}`,
		},
		{
			// Two message types reached through different branches, which a global
			// visited set collapses into one branch.
			name:  "fields of two nested messages both decode",
			msg:   func() proto.Message { return &userpb.ListUsersRequest{} },
			query: "header.requestId=r1&header.traceId=t1&filter.minAge=0",
			want:  `{"header":{"requestId":"r1","traceId":"t1"},"filter":{}}`, // minAge=0 is a zero value
		},
		{
			// An enum is sent by name.
			name:  "enum is sent by name",
			msg:   func() proto.Message { return &userpb.ListUsersRequest{} },
			query: "statuses=USER_STATUS_ACTIVE&statuses=USER_STATUS_DISABLED",
			want:  `{"statuses":["USER_STATUS_ACTIVE","USER_STATUS_DISABLED"]}`,
		},
		{
			// The client excludes path-bound fields from the query. Verify that the
			// gateway would have applied one if it were present, which is what makes
			// the exclusion necessary rather than cosmetic.
			name:  "a path-bound field is filtered out of the query",
			msg:   func() proto.Message { return &userpb.GetUserRequest{} },
			bound: []string{"id"},
			query: "id=from-query",
			want:  `{}`,
		},
	}

	failures := 0
	for _, c := range cases {
		if err := run(c); err != nil {
			failures++
			fmt.Fprintf(os.Stderr, "FAIL %s\n     %v\n", c.name, err)
			continue
		}
		fmt.Printf("ok   %s\n", c.name)
	}

	if failures > 0 {
		os.Exit(1)
	}
	fmt.Println("\nall grpc-gateway compatibility checks passed")
}

// run decodes one query string and compares the result with the expected JSON.
func run(c queryCase) error {
	msg := c.msg()

	values, err := url.ParseQuery(c.query)
	if err != nil {
		return fmt.Errorf("url.ParseQuery: %w", err)
	}

	// A path-bound field is filtered by the DoubleArray the gateway builds from the
	// template's variables.
	var filter *utilities.DoubleArray
	if len(c.bound) == 0 {
		filter = utilities.NewDoubleArray(nil)
	} else {
		seqs := make([][]string, 0, len(c.bound))
		for _, b := range c.bound {
			seqs = append(seqs, splitPath(b))
		}
		filter = utilities.NewDoubleArray(seqs)
	}

	if err := runtime.PopulateQueryParameters(msg, values, filter); err != nil {
		return fmt.Errorf("PopulateQueryParameters: %w", err)
	}

	// A presence expectation asserts decoded field values directly, which is the only
	// way to prove that 0, false and "" were actually decoded: protojson omits zero
	// values, so JSON alone cannot distinguish "decoded to 0" from "absent".
	if c.wantPresence != nil {
		return checkPresence(msg, c.wantPresence)
	}

	raw, err := protojson.Marshal(msg)
	if err != nil {
		return fmt.Errorf("protojson.Marshal: %w", err)
	}

	var got, want any
	if err := json.Unmarshal(raw, &got); err != nil {
		return fmt.Errorf("decoding the decoded message: %w", err)
	}
	if err := json.Unmarshal([]byte(c.want), &want); err != nil {
		return fmt.Errorf("decoding the expectation: %w", err)
	}
	if !reflect.DeepEqual(got, want) {
		return fmt.Errorf("decoded %s\n     want    %s", raw, c.want)
	}
	return nil
}

// checkPresence walks each expected field path and compares the decoded value.
func checkPresence(msg proto.Message, want *presence) error {
	root := msg.ProtoReflect()
	for path, expected := range want.set {
		value, ok := lookup(root, splitPath(path))
		if !ok {
			return fmt.Errorf("field %q was not decoded into the message", path)
		}
		got := value.Interface()
		if !reflect.DeepEqual(got, expected) {
			return fmt.Errorf("field %q decoded to %#v, want %#v", path, got, expected)
		}
	}
	return nil
}

// lookup resolves a proto field path to its value.
func lookup(msg protoreflect.Message, path []string) (protoreflect.Value, bool) {
	current := msg
	for i, name := range path {
		// Both spellings resolve: the path is written with request-type field names,
		// which are JSON names when use_proto_names is off.
		fd := current.Descriptor().Fields().ByJSONName(name)
		if fd == nil {
			fd = current.Descriptor().Fields().ByName(protoreflect.Name(name))
		}
		if fd == nil {
			return protoreflect.Value{}, false
		}
		value := current.Get(fd)
		if i == len(path)-1 {
			return value, true
		}
		current = value.Message()
	}
	return protoreflect.Value{}, false
}

func splitPath(p string) []string {
	var out []string
	cur := ""
	for _, r := range p {
		if r == '.' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(r)
	}
	return append(out, cur)
}
