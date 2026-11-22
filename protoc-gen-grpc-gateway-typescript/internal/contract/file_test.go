package contract

import "testing"

func TestFileBase(t *testing.T) {
	cases := []struct{ in, want string }{
		{"user/v1/user.proto", "user/v1/user"},
		{"address.proto", "address"},
		{"a/b.proto", "a/b"},
	}
	for _, c := range cases {
		if got := FileBase(c.in); got != c.want {
			t.Errorf("FileBase(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestOutputFiles(t *testing.T) {
	o := DefaultOptions()
	if got := o.TypesFile("user/v1/user.proto"); got != "user/v1/user_pb.ts" {
		t.Errorf("TypesFile = %q", got)
	}
	if got := o.ApiFile("user/v1/user.proto"); got != "user/v1/user_api.ts" {
		t.Errorf("ApiFile = %q", got)
	}
	if got := o.QueryFile("user/v1/user.proto"); got != "user/v1/user_react_query.ts" {
		t.Errorf("QueryFile = %q", got)
	}
}

func TestCustomSuffixes(t *testing.T) {
	o := DefaultOptions()
	o.TypesSuffix = ".types.ts"
	o.ApiSuffix = ".client.ts"
	o.QuerySuffix = ".queries.ts"

	if got := o.TypesFile("user/v1/user.proto"); got != "user/v1/user.types.ts" {
		t.Errorf("TypesFile = %q", got)
	}
	spec, alias, ok := o.TypesImportPath("user/v1/order.proto", "user/v1/user.proto")
	if !ok || spec != "./user.types" {
		t.Errorf("TypesImportPath = %q, ok=%v", spec, ok)
	}
	if alias != "user_v1_user_types" {
		t.Errorf("alias = %q", alias)
	}

	// With a runtime extension the custom suffix is preserved and only .ts is
	// replaced, so `user.types.ts` is imported as `user.types.js`.
	o.ImportExtension = ".js"
	spec, _, _ = o.TypesImportPath("user/v1/order.proto", "user/v1/user.proto")
	if spec != "./user.types.js" {
		t.Errorf("TypesImportPath with import_extension = %q", spec)
	}
}

func TestImportExtensionReplacesSourceExtension(t *testing.T) {
	// NodeNext resolves the emitted specifier against the runtime file, so
	// `_pb.ts` must become `_pb.js`, not `_pb.ts.js`.
	o := DefaultOptions()
	o.ImportExtension = ".js"

	spec, _, ok := o.TypesImportPath("user/v1/user.proto", "common/v1/address.proto")
	if !ok {
		t.Fatal("expected an import path")
	}
	if spec != "../../common/v1/address_pb.js" {
		t.Errorf("spec = %q", spec)
	}
}

func TestImportWithoutExtension(t *testing.T) {
	o := DefaultOptions()
	spec, _, _ := o.TypesImportPath("user/v1/user.proto", "common/v1/address.proto")
	if spec != "../../common/v1/address_pb" {
		t.Errorf("spec = %q", spec)
	}
}

func TestImportSameDirectory(t *testing.T) {
	o := DefaultOptions()
	o.ImportExtension = ".js"
	spec, _, _ := o.TypesImportPath("user/v1/user.proto", "user/v1/order.proto")
	if spec != "./order_pb.js" {
		t.Errorf("spec = %q", spec)
	}
}

func TestModuleMapBeatsRelativePath(t *testing.T) {
	o := DefaultOptions()
	o.ImportExtension = ".js"
	o.ModuleMap["google/type/money.proto"] = "@acme/grpc-types/google/type/money"

	spec, _, _ := o.TypesImportPath("user/v1/user.proto", "google/type/money.proto")
	// A package specifier never gets the relative import extension.
	if spec != "@acme/grpc-types/google/type/money" {
		t.Errorf("spec = %q", spec)
	}
}

func TestSkipProto(t *testing.T) {
	o := DefaultOptions()
	for _, p := range []string{
		"google/protobuf/timestamp.proto",
		"google/protobuf/wrappers.proto",
		"google/api/annotations.proto",
		"google/api/http.proto",
	} {
		if !o.SkipProto(p) {
			t.Errorf("SkipProto(%q) = false, want true", p)
		}
	}
	if o.SkipProto("user/v1/user.proto") {
		t.Errorf("SkipProto(user proto) = true, want false")
	}
}

func TestImportAliasIsStableAndUnique(t *testing.T) {
	// Two files with the same base name in different directories must not
	// collide: namespace imports would otherwise be ambiguous.
	a := ImportAlias("user/v1/user.proto", "_pb.ts")
	b := ImportAlias("admin/v1/user.proto", "_pb.ts")
	if a == b {
		t.Errorf("aliases collide: %q", a)
	}
	if a != "user_v1_user_pb" {
		t.Errorf("alias = %q", a)
	}
	// Deterministic across calls.
	if ImportAlias("user/v1/user.proto", "_pb.ts") != a {
		t.Error("alias is not deterministic")
	}
}

func TestOptionsRejectBadSuffix(t *testing.T) {
	o := DefaultOptions()
	o.TypesSuffix = "bad/path.ts"
	if err := o.Validate(); err == nil {
		t.Error("expected an error for a suffix containing a path separator")
	}
}

func TestOptionsRejectBigint(t *testing.T) {
	// bigint breaks React Query query keys, so it is deliberately not accepted.
	o := DefaultOptions()
	o.LongType = "bigint"
	if err := o.Validate(); err == nil {
		t.Error("expected an error for long_type=bigint")
	}
}

func TestLowerCamel(t *testing.T) {
	cases := []struct{ in, want string }{
		{"GetUser", "getUser"},
		{"ListUsers", "listUsers"},
		{"UserService", "userService"},
		{"ID", "id"},
		{"URLs", "urls"},
		{"ListURLs", "listURLs"},
	}
	for _, c := range cases {
		if got := LowerCamel(c.in); got != c.want {
			t.Errorf("LowerCamel(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestHookAndClientNames(t *testing.T) {
	if got := ClientName("UserService"); got != "UserServiceClient" {
		t.Errorf("ClientName = %q", got)
	}
	if got := HookName("getUser"); got != "useGetUser" {
		t.Errorf("HookName = %q", got)
	}
	if got := HookName("getUserBinding2"); got != "useGetUserBinding2" {
		t.Errorf("HookName = %q", got)
	}
	if got := KeysName("UserService"); got != "userServiceKeys" {
		t.Errorf("KeysName = %q", got)
	}
}

func TestNestedNameSeparator(t *testing.T) {
	o := DefaultOptions()
	if got := o.TypeName([]string{"Outer", "Inner"}); got != "Outer_Inner" {
		t.Errorf("TypeName = %q", got)
	}
	o.NestedNameSeparator = "$"
	if got := o.TypeName([]string{"Outer", "Inner"}); got != "Outer$Inner" {
		t.Errorf("TypeName with custom separator = %q", got)
	}
}
