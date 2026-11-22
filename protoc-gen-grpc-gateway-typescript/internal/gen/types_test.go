package gen_test

import (
	"strings"
	"testing"
)

// TestGoldenTypes pins the full generated output. Any change to naming, imports
// or type mapping shows up here as a readable diff.
func TestGoldenTypes(t *testing.T) {
	out := generate(t, "import_extension=.js", nil)

	checkGolden(t, "user_v1_user_pb.ts", out["user/v1/user_pb.ts"])
	checkGolden(t, "common_v1_address_pb.ts", out["common/v1/address_pb.ts"])
}

// TestTypeMappingMatrix asserts every row of the mapping table in docs/v1.md
// §3.5 individually, so a regression names the row instead of showing up as one
// large golden diff.
func TestTypeMappingMatrix(t *testing.T) {
	user := generate(t, "", nil)["user/v1/user_pb.ts"]

	cases := []struct{ field, want string }{
		// Scalars.
		{"id", "string"},
		{"name", "string"},
		{"age", "number"},            // int32
		{"createdAtEpoch", "string"}, // int64 defaults to string
		{"avatar", "string"},         // bytes is base64
		{"active", "boolean"},
		{"status", "UserStatus"}, // enum becomes a string union

		// Well-known types, inlined as primitives.
		{"createdAt", "string"},                   // Timestamp
		{"maskPaths", "string"},                   // FieldMask
		{"metadata", "Record<string, JsonValue>"}, // Struct
		{"empty", "Record<string, never>"},
		{"extension", `{ "@type": string; [key: string]: unknown }`},

		// Wrappers become nullable primitives.
		{"tagline", "string | null"}, // StringValue
		{"balance", "string | null"}, // Int64Value with long_type=string

		// Nested naming.
		{"nested", "User_Nested"},
		{"nestedStatuses", "User_Nested_NestedStatus[]"},
	}

	for _, c := range cases {
		want := c.field + "?: " + c.want + ";"
		if !strings.Contains(user, want) {
			t.Errorf("missing %q in the generated User type", want)
		}
	}
}

// TestCollectionsMapping covers repeated and map fields, including the rule that
// JSON map keys are always strings.
func TestCollectionsMapping(t *testing.T) {
	user := generate(t, "", nil)["user/v1/user_pb.ts"]

	if !strings.Contains(user, "ids?: string[];") {
		t.Error("repeated string must map to string[]")
	}
	if !strings.Contains(user, "labels?: Record<string, string>;") {
		t.Error("map<string,string> must map to Record<string, string>")
	}
	if !strings.Contains(user, "contactsByKind?: Record<string, Contact>;") {
		t.Error("map<string,Contact> must map to Record<string, Contact>")
	}
}

// TestInt64AsNumber covers long_type=number, the documented opt-in with a
// precision caveat.
func TestInt64AsNumber(t *testing.T) {
	user := generate(t, "long_type=number", nil)["user/v1/user_pb.ts"]

	if !strings.Contains(user, "createdAtEpoch?: number;") {
		t.Error("long_type=number must map int64 to number")
	}
	if !strings.Contains(user, "balance?: number | null;") {
		t.Error("long_type=number must map Int64Value to number | null")
	}
}

// TestEnumStyleUnionIsDefault checks the union form and that every value name is
// preserved.
func TestEnumStyleUnionIsDefault(t *testing.T) {
	user := generate(t, "", nil)["user/v1/user_pb.ts"]

	want := `export type UserStatus =
  | "USER_STATUS_UNSPECIFIED"
  | "USER_STATUS_ACTIVE"
  | "USER_STATUS_DISABLED";`
	if !strings.Contains(user, want) {
		t.Errorf("expected the default union form, got:\n%s", section(user, "UserStatus"))
	}
}

// TestEnumStyleConst checks the const-object variant.
func TestEnumStyleConst(t *testing.T) {
	user := generate(t, "enum_style=const", nil)["user/v1/user_pb.ts"]

	for _, want := range []string{
		"export const UserStatus = {",
		`USER_STATUS_UNSPECIFIED: "USER_STATUS_UNSPECIFIED",`,
		"} as const;",
		"export type UserStatus = (typeof UserStatus)[keyof typeof UserStatus];",
	} {
		if !strings.Contains(user, want) {
			t.Errorf("enum_style=const output is missing %q", want)
		}
	}
}

// TestEnumStyleEnum checks the TypeScript enum variant.
func TestEnumStyleEnum(t *testing.T) {
	user := generate(t, "enum_style=enum", nil)["user/v1/user_pb.ts"]

	if !strings.Contains(user, "export enum UserStatus {") {
		t.Error("enum_style=enum must emit a TypeScript enum")
	}
	if !strings.Contains(user, `USER_STATUS_ACTIVE = "USER_STATUS_ACTIVE",`) {
		t.Error("enum members must keep their wire names")
	}
}

// TestOneofUnionIsMutuallyExclusive checks the default oneof form: exactly one
// member may be set, the others are forbidden with `?: never`, and the no-member-set
// arm is present because proto3 permits an unset oneof.
func TestOneofUnionIsMutuallyExclusive(t *testing.T) {
	user := generate(t, "", nil)["user/v1/user_pb.ts"]

	want := `export type Payment =
  | {
    card: Card;
    bank?: never;
    cash?: never;
  }
  | {
    bank: Bank;
    card?: never;
    cash?: never;
  }
  | {
    cash: Cash;
    card?: never;
    bank?: never;
  }
  | {
    card?: never;
    bank?: never;
    cash?: never;
  };`
	if !strings.Contains(user, want) {
		t.Errorf("unexpected Payment union:\n%s", section(user, "type Payment"))
	}
}

// TestOneofAllowsUnset is the regression guard for a defect found while building the
// example project: a oneof may be unset in proto3, and the union must include an arm
// for that, or a server response with no member chosen cannot be typed as its own
// generated type.
func TestOneofAllowsUnset(t *testing.T) {
	user := generate(t, "", nil)["user/v1/user_pb.ts"]

	// The final arm sets no member at all.
	paymentSection := section(user, "type Payment")
	if !strings.Contains(paymentSection, "card?: never;\n    bank?: never;\n    cash?: never;\n  };") {
		t.Errorf("the union must end with an arm that sets no member:\n%s", paymentSection)
	}
	// And the singular-member case stays flat, where "unset" is already expressible.
	if !strings.Contains(user, "export type Contact =") {
		t.Error("Contact must still be generated")
	}
}

// TestOneofFlatFallback checks oneof_style=flat, which emits plain optional
// members.
func TestOneofFlatFallback(t *testing.T) {
	user := generate(t, "oneof_style=flat", nil)["user/v1/user_pb.ts"]

	want := `export type Payment = {
  card?: Card;
  bank?: Bank;
  cash?: Cash;
};`
	if !strings.Contains(user, want) {
		t.Errorf("unexpected flat Payment:\n%s", section(user, "type Payment"))
	}
}

// TestSyntheticOneofIsNotAUnion is the proto3 optional regression guard: an
// `optional` field creates a synthetic oneof, and if it were treated as a
// business oneof the field would be forced into an exclusive union.
func TestSyntheticOneofIsNotAUnion(t *testing.T) {
	user := generate(t, "", nil)["user/v1/user_pb.ts"]

	if !strings.Contains(user, "nickname?: string;") {
		t.Error("a proto3 optional field must stay a plain optional property")
	}
	if strings.Contains(user, "never") && strings.Count(user, "export type User =") == 0 {
		t.Error("the User type was lost")
	}

	// User must be a plain object, not a union of synthetic-oneof arms.
	if !strings.Contains(user, "export type User = {") {
		t.Errorf("User must be an object type, got:\n%s", section(user, "type User ="))
	}
}

// TestNestedTypeNaming checks Outer_Inner and the configurable separator.
func TestNestedTypeNaming(t *testing.T) {
	if user := generate(t, "", nil)["user/v1/user_pb.ts"]; !strings.Contains(user, "export type User_Nested = {") {
		t.Error("nested messages must be named Outer_Inner")
	}
	if user := generate(t, "nested_name_separator=__", nil)["user/v1/user_pb.ts"]; !strings.Contains(user, "export type User__Nested = {") {
		t.Error("nested_name_separator must be honoured")
	}
}

// TestUseProtoNames checks that field names switch to proto names, which must
// match the gateway's marshaler configuration.
func TestUseProtoNames(t *testing.T) {
	user := generate(t, "use_proto_names=true", nil)["user/v1/user_pb.ts"]

	if !strings.Contains(user, "page_size?: number;") {
		t.Error("use_proto_names must emit proto field names")
	}
	if strings.Contains(user, "pageSize?") {
		t.Error("use_proto_names must not emit JSON names")
	}
}

// TestCommentsEmitted and its off-switch.
func TestCommentsEmitted(t *testing.T) {
	user := generate(t, "", nil)["user/v1/user_pb.ts"]
	if !strings.Contains(user, "/**") {
		t.Error("proto leading comments must become JSDoc")
	}
	if !strings.Contains(user, "GetUserRequest identifies a user.") {
		t.Error("expected the message comment text")
	}
	if !strings.Contains(user, "id is the user id.") {
		t.Error("expected the field comment text")
	}
}

func TestCommentsDisabled(t *testing.T) {
	user := generate(t, "emit_comments=false", nil)["user/v1/user_pb.ts"]
	if strings.Contains(user, "/**") {
		t.Error("emit_comments=false must drop JSDoc")
	}
}

// TestFileHeader checks the stable banner the contract requires.
func TestFileHeader(t *testing.T) {
	user := generate(t, "", nil)["user/v1/user_pb.ts"]

	for _, want := range []string{
		"// Code generated by protoc-gen-grpc-gateway-typescript. DO NOT EDIT.",
		"// @grpc-gateway-typescript-contract: v1",
		"// source: user/v1/user.proto",
	} {
		if !strings.Contains(user, want) {
			t.Errorf("header is missing %q", want)
		}
	}
}

// TestCrossFileNamespaceImport checks that a type from another proto is imported
// under a namespace alias instead of being copied into the file.
func TestCrossFileNamespaceImport(t *testing.T) {
	user := generate(t, "import_extension=.js", nil)["user/v1/user_pb.ts"]

	if !strings.Contains(user, `import type * as common_v1_address_pb from "../../common/v1/address_pb.js";`) {
		t.Errorf("expected a namespace import, got:\n%s", user[:min(len(user), 400)])
	}
	if !strings.Contains(user, "address?: common_v1_address_pb.Address;") {
		t.Error("cross-file types must be referenced through the namespace")
	}
}

// TestNoWellKnownTypeImports checks that google/protobuf is never imported.
func TestNoWellKnownTypeImports(t *testing.T) {
	user := generate(t, "", nil)["user/v1/user_pb.ts"]

	for _, forbidden := range []string{
		"google/protobuf",
		"timestamp_pb",
		"wrappers_pb",
		"struct_pb",
		"any_pb",
	} {
		if strings.Contains(user, forbidden) {
			t.Errorf("well-known types must be inlined, found %q", forbidden)
		}
	}
}

// TestDeterministicOutput runs generation twice and requires byte equality: no
// timestamps, no map iteration order.
func TestDeterministicOutput(t *testing.T) {
	first := generate(t, "import_extension=.js", nil)
	second := generate(t, "import_extension=.js", nil)

	for name, a := range first {
		if b := second[name]; a != b {
			t.Errorf("%s differs between runs", name)
		}
	}
}

// TestImportOrderIsSorted checks that imports do not depend on field order.
func TestImportOrderIsSorted(t *testing.T) {
	user := generate(t, "", nil)["user/v1/user_pb.ts"]

	var imports []string
	for _, line := range strings.Split(user, "\n") {
		if strings.HasPrefix(line, "import ") {
			imports = append(imports, line)
		}
	}
	for i := 1; i < len(imports); i++ {
		if imports[i-1] > imports[i] {
			t.Errorf("imports are not sorted:\n%s\n%s", imports[i-1], imports[i])
		}
	}
}

// section returns the lines around a declaration, for readable failure output.
func section(src, marker string) string {
	idx := strings.Index(src, marker)
	if idx < 0 {
		return "(not found)"
	}
	end := idx + 400
	if end > len(src) {
		end = len(src)
	}
	return src[idx:end]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
