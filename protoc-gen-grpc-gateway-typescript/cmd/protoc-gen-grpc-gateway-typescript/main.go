// Command protoc-gen-grpc-gateway-typescript generates TypeScript types from
// proto messages and enums.
//
// It emits <base>_type.ts next to the source proto path, where <base> is the proto
// path with the .proto suffix removed. Well-known types are inlined as TypeScript
// primitives and google/protobuf/*.proto is never generated, even when Buf is
// configured with include_imports.
package main

import (
	"fmt"

	"google.golang.org/protobuf/compiler/protogen"

	"github.com/qhai-dev/protoc-gen-grpc-gateway-typescript/internal/contract"
	"github.com/qhai-dev/protoc-gen-grpc-gateway-typescript/internal/gen"
	"github.com/qhai-dev/protoc-gen-grpc-gateway-typescript/internal/ir"
	"github.com/qhai-dev/protoc-gen-grpc-gateway-typescript/internal/plugin"
)

func main() {
	contractOpts := contract.DefaultOptions()
	pluginOpts := gen.DefaultOptions()

	plugin.Run(contractOpts, pluginOpts, func(p *protogen.Plugin) error {
		if err := pluginOpts.Validate(); err != nil {
			return err
		}

		model, err := ir.NewBuilder(contractOpts, ir.BuildOptions{}).Build(p)
		if err != nil {
			return err
		}

		generator := gen.New(contractOpts, pluginOpts)
		for _, f := range model.Files {
			content, err := generator.Generate(f)
			if err != nil {
				return fmt.Errorf("%s: %w", f.ProtoPath, err)
			}
			out := p.NewGeneratedFile(contractOpts.TypesFile(f.ProtoPath), goImportPath())
			if _, err := out.Write([]byte(content)); err != nil {
				return err
			}
		}
		return nil
	})
}

// goImportPath satisfies protogen's import-path bookkeeping. TypeScript output
// has no Go package, and the path is only used to detect two generators writing
// the same Go file, which cannot happen across the three plugins because each
// writes its own suffix.
func goImportPath() protogen.GoImportPath {
	return protogen.GoImportPath("grpc.gateway.typescript/types")
}
