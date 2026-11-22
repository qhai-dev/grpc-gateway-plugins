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
// The synthesized imports never reach the generated TypeScript: the contract's
// own `M` mappings are what the contract layer consults, and protogen's Go paths
// are internal bookkeeping only.
func Prepare(req *pluginpb.CodeGeneratorRequest) {
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
