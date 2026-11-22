// Package request prepares a CodeGeneratorRequest for protogen.
//
// protogen was written for Go generators and insists on a Go import path for
// every proto file. A TypeScript generator has no Go packages, and requiring
// `option go_package` in every proto would make this toolchain unusable outside a
// Go codebase. This package synthesizes a stable mapping for files that lack one;
// mappings the user supplied always win.
package request

import (
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/pluginpb"
)

// Prepare mutates req so protogen can read it.
//
// Two normalizations happen here, both before protogen parses the parameter
// string:
//
//  1. A `M` mapping is synthesized for every proto file that has no go_package,
//     because protogen refuses to start otherwise.
//  2. List-valued parameters are rewritten as repeated `name=value` pairs. protoc
//     splits the plugin parameter string on commas, so
//     `queries=a.B.C,user.v1.UserService.Validate` would otherwise reach protogen
//     as `queries=a.B.C` plus a bare `user.v1.UserService.Validate`, which looks
//     like an unknown parameter.
//
// The synthesized imports never reach the generated TypeScript: the contract's own
// `M` mappings are what the contract layer consults, and protogen's Go paths are
// internal bookkeeping only.
func Prepare(req *pluginpb.CodeGeneratorRequest) {
	normalizeListParams(req)
	supplyGoImportPaths(req)
}

// listParams names the parameters whose values form a comma-separated list.
var listParams = map[string]bool{
	"queries": true,
}

// normalizeListParams rewrites `name=v1,v2` into `name=v1,name=v2`.
//
// It only rewrites a bare token when the token follows a list-valued parameter; any
// other bare token is left alone so protogen can report it as unknown.
func normalizeListParams(req *pluginpb.CodeGeneratorRequest) {
	parts := strings.Split(req.GetParameter(), ",")
	var out []string
	lastList := ""
	for _, part := range parts {
		if part == "" {
			continue
		}
		name, value, hasValue := strings.Cut(part, "=")
		if !hasValue {
			if lastList != "" {
				// A continuation of the preceding list.
				out = append(out, lastList+"="+part)
				continue
			}
			out = append(out, part)
			continue
		}
		out = append(out, part)
		if listParams[name] {
			lastList = name
		} else {
			lastList = ""
		}
		_ = value
	}
	req.Parameter = proto.String(strings.Join(out, ","))
}

// supplyGoImportPaths appends an `M<proto>=<module>` parameter for every proto file
// that has no go_package option, unless the user already mapped it.
func supplyGoImportPaths(req *pluginpb.CodeGeneratorRequest) {
	existing := map[string]bool{}
	for _, param := range strings.Split(req.GetParameter(), ",") {
		if len(param) > 1 && param[0] == 'M' {
			if eq := strings.Index(param, "="); eq > 1 {
				existing[param[1:eq]] = true
			}
		}
	}

	var extra []string
	for _, fd := range req.GetProtoFile() {
		path := fd.GetName()
		if existing[path] {
			continue
		}
		if fd.GetOptions().GetGoPackage() != "" {
			continue
		}
		extra = append(extra, "M"+path+"="+syntheticGoPackage(path))
	}
	if len(extra) == 0 {
		return
	}

	parameter := req.GetParameter()
	if parameter != "" {
		parameter += ","
	}
	parameter += strings.Join(extra, ",")
	req.Parameter = proto.String(parameter)
}

// syntheticGoPackage derives a stable, valid Go import path from a proto path.
func syntheticGoPackage(protoPath string) string {
	base := strings.TrimSuffix(protoPath, ".proto")
	base = strings.ReplaceAll(base, "/", "_")
	base = strings.ReplaceAll(base, "-", "_")
	base = strings.ReplaceAll(base, ".", "_")
	return "grpcgateway.typescript/" + base
}
