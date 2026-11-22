// Command protoc-gen-grpc-gateway-axios generates Axios clients from proto
// services and their google.api.http annotations.
//
// It emits <base>_api.ts next to the source proto path. The generated code never
// creates an axios instance: the caller injects one, which keeps base URL,
// interceptors, auth and retries under the application's control
// (docs/v1.md §4.14).
package main

import (
	"fmt"

	"google.golang.org/protobuf/compiler/protogen"

	"github.com/qhai-dev/protoc-gen-grpc-gateway-axios/internal/contract"
	"github.com/qhai-dev/protoc-gen-grpc-gateway-axios/internal/gen"
	"github.com/qhai-dev/protoc-gen-grpc-gateway-axios/internal/ir"
	"github.com/qhai-dev/protoc-gen-grpc-gateway-axios/internal/plugin"
)

func main() {
	contractOpts := contract.DefaultOptions()
	pluginOpts := gen.DefaultOptions()

	plugin.Run(contractOpts, pluginOpts, func(p *protogen.Plugin) error {
		if err := pluginOpts.Validate(); err != nil {
			return err
		}

		model, err := ir.NewBuilder(contractOpts, pluginOpts.BuildOptions()).Build(p)
		if err != nil {
			return err
		}

		generator := gen.New(contractOpts, pluginOpts)
		for _, f := range model.Files {
			content, err := generator.Generate(f)
			if err != nil {
				return fmt.Errorf("%s: %w", f.ProtoPath, err)
			}
			out := p.NewGeneratedFile(contractOpts.ApiFile(f.ProtoPath), goImportPath())
			if _, err := out.Write([]byte(content)); err != nil {
				return err
			}
		}
		return nil
	})
}

// goImportPath satisfies protogen's import-path bookkeeping. TypeScript output has
// no Go package, and the path is only used to detect two generators writing the
// same Go file, which cannot happen across the three plugins because each writes
// its own suffix.
func goImportPath() protogen.GoImportPath {
	return protogen.GoImportPath("grpc.gateway.typescript/api")
}
