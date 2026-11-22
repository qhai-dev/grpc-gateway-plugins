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

	"github.com/qhai-dev/protoc-gen-grpc-gateway-typescript/internal/contract"
	"github.com/qhai-dev/protoc-gen-grpc-gateway-typescript/internal/gen"
	"github.com/qhai-dev/protoc-gen-grpc-gateway-typescript/internal/ir"
	"github.com/qhai-dev/protoc-gen-grpc-gateway-typescript/internal/params"
	"github.com/qhai-dev/protoc-gen-grpc-gateway-typescript/internal/request"
)

// update controls `go test -update`, which rewrites the golden files instead of
// failing on a diff.
var update = flag.Bool("update", false, "rewrite golden files")

// loadRequest reads the checked-in descriptor set. Tests never shell out to
// protoc: the fixture is exactly what protoc hands the plugin, and pinning it
// keeps golden files reproducible without a protobuf toolchain installed.
func loadRequest(t *testing.T, parameter string) *pluginpb.CodeGeneratorRequest {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixture.pb"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	set := &descriptorpb.FileDescriptorSet{}
	if err := proto.Unmarshal(raw, set); err != nil {
		t.Fatalf("decoding fixture: %v", err)
	}

	return &pluginpb.CodeGeneratorRequest{
		FileToGenerate: []string{"user/v1/user.proto", "common/v1/address.proto"},
		ProtoFile:      set.File,
		Parameter:      proto.String(parameter),
	}
}

// generate runs the real plugin pipeline: parse parameters, build the IR, render
// every generated file. The returned map is keyed by output path.
func generate(t *testing.T, parameter string, mutate func(*gen.Options)) map[string]string {
	t.Helper()

	req := loadRequest(t, parameter)
	request.Prepare(req)

	c := contract.DefaultOptions()
	opts := gen.DefaultOptions()
	if mutate != nil {
		mutate(opts)
	}

	p, err := protogen.Options{ParamFunc: params.New(c, opts).Set}.New(req)
	if err != nil {
		t.Fatalf("protogen.New: %v", err)
	}

	model, err := ir.NewBuilder(c, ir.BuildOptions{}).Build(p)
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
		out[c.TypesFile(f.ProtoPath)] = content
	}
	return out
}

// checkGolden compares one output file against its golden copy, or rewrites it
// under -update.
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
		t.Fatalf("reading golden %s (run go test -update to create it): %v", name, err)
	}
	if got != string(want) {
		t.Errorf("%s does not match its golden file.\nRun `go test ./... -update` to accept the new output.\n\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}
