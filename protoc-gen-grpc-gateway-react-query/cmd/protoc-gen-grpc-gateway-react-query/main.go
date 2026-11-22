// Command protoc-gen-grpc-gateway-react-query generates TanStack Query v5 hooks
// from proto services.
//
// It emits <base>_react_query.ts next to the source proto path. The hooks take a
// generated Axios client as a parameter, so the module never reaches for a global
// axios instance and can be pointed at any host or configuration
// (docs/v1.md §5.8).
package main

import (
	"fmt"

	"google.golang.org/protobuf/compiler/protogen"

	"github.com/qhai-dev/protoc-gen-grpc-gateway-react-query/internal/contract"
	"github.com/qhai-dev/protoc-gen-grpc-gateway-react-query/internal/gen"
	"github.com/qhai-dev/protoc-gen-grpc-gateway-react-query/internal/ir"
	"github.com/qhai-dev/protoc-gen-grpc-gateway-react-query/internal/plugin"
)

func main() {
	contractOpts := contract.DefaultOptions()
	pluginOpts := gen.DefaultOptions()

	plugin.RunWithLists(contractOpts, pluginOpts, []string{"queries"}, func(p *protogen.Plugin) error {
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
			out := p.NewGeneratedFile(contractOpts.QueryFile(f.ProtoPath), goImportPath())
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
	return protogen.GoImportPath("grpc.gateway.typescript/reactquery")
}
