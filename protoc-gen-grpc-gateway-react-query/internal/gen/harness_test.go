package gen_test

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"

	"github.com/qhai-dev/protoc-gen-grpc-gateway-react-query/internal/contract"
	"github.com/qhai-dev/protoc-gen-grpc-gateway-react-query/internal/gen"
	"github.com/qhai-dev/protoc-gen-grpc-gateway-react-query/internal/ir"
	"github.com/qhai-dev/protoc-gen-grpc-gateway-react-query/internal/params"
	"github.com/qhai-dev/protoc-gen-grpc-gateway-react-query/internal/request"
)

// update controls `go test -update`, which rewrites the golden files.
var update = flag.Bool("update", false, "rewrite golden files")

// generate runs the real pipeline and returns the generated files by output path.
func generate(t *testing.T, parameter string) map[string]string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixture.pb"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	set := &descriptorpb.FileDescriptorSet{}
	if err := proto.Unmarshal(raw, set); err != nil {
		t.Fatalf("decoding fixture: %v", err)
	}

	req := &pluginpb.CodeGeneratorRequest{
		FileToGenerate: []string{
			"user/v1/user.proto",
			"common/v1/address.proto",
			"shipping/v1/shipping.proto",
		},
		ProtoFile: set.File,
		Parameter: proto.String(parameter),
	}
	request.Prepare(req)

	c := contract.DefaultOptions()
	opts := gen.DefaultOptions()

	p, err := protogen.Options{ParamFunc: params.New(c, opts, "queries").Set}.New(req)
	if err != nil {
		t.Fatalf("protogen.New: %v", err)
	}
	if err := opts.Validate(); err != nil {
		t.Fatalf("validating options: %v", err)
	}

	model, err := ir.NewBuilder(c, opts.BuildOptions()).Build(p)
	if err != nil {
		t.Fatalf("building IR: %v", err)
	}

	generator := gen.New(c, opts)
	out := map[string]string{}
	for _, f := range model.Files {
		content, err := generator.Generate(f)
		if err != nil {
			t.Fatalf("generating %s: %v", f.ProtoPath, err)
		}
		out[c.QueryFile(f.ProtoPath)] = content
	}
	return out
}

// checkGolden compares one output file with its golden copy, or rewrites it.
func checkGolden(t *testing.T, name, got string) {
	t.Helper()

	goldenPath := filepath.Join("..", "..", "testdata", "golden", name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("creating golden dir: %v", err)
		}
		if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
			t.Fatalf("writing golden: %v", err)
		}
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("reading golden %s (run go test ./internal/gen -update to create it): %v", name, err)
	}
	if got != string(want) {
		t.Errorf("%s does not match its golden file.\nRun `go test ./internal/gen -update` to accept the new output.\n\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}
