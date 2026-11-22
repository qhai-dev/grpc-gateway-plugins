// Package plugin runs a protoc plugin: it parses the parameter string, builds
// the IR and emits the response. Each plugin binary is a thin main over this.
package plugin

import (
	"fmt"
	"io"
	"os"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/pluginpb"

	"github.com/qhai-dev/protoc-gen-grpc-gateway-typescript/internal/contract"
	"github.com/qhai-dev/protoc-gen-grpc-gateway-typescript/internal/params"
	"github.com/qhai-dev/protoc-gen-grpc-gateway-typescript/internal/request"
)

// OptionSetter is implemented by each plugin's own options struct.
type OptionSetter = params.Setter

// Run executes a plugin end to end.
//
// The response advertises FEATURE_PROTO3_OPTIONAL; without it protoc and Buf
// refuse any request that contains an `optional` keyword field
// (docs/v1.md §6.6).
func Run(c *contract.Options, pluginOpts OptionSetter, generate func(*protogen.Plugin) error) {
	req, err := readRequest(os.Stdin)
	if err != nil {
		fail(err)
	}
	request.Prepare(req)

	gen, err := protogen.Options{ParamFunc: params.New(c, pluginOpts).Set}.New(req)
	if err != nil {
		fail(err)
	}
	gen.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL)

	if err := c.Validate(); err != nil {
		gen.Error(err)
	} else if err := generate(gen); err != nil {
		gen.Error(err)
	}

	resp := gen.Response()
	if resp.GetError() != "" {
		// Failing loudly is required by the protocol: a response that carries
		// generated files plus an error would leave partial output on disk.
		fail(fmt.Errorf("%s", resp.GetError()))
	}
	if err := writeResponse(os.Stdout, resp); err != nil {
		fail(err)
	}
}

// readRequest reads a CodeGeneratorRequest from the plugin's stdin.
func readRequest(r io.Reader) (*pluginpb.CodeGeneratorRequest, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading CodeGeneratorRequest: %w", err)
	}
	req := &pluginpb.CodeGeneratorRequest{}
	if err := proto.Unmarshal(raw, req); err != nil {
		return nil, fmt.Errorf("decoding CodeGeneratorRequest: %w", err)
	}
	return req, nil
}

// writeResponse writes a CodeGeneratorResponse to the plugin's stdout.
func writeResponse(w io.Writer, resp *pluginpb.CodeGeneratorResponse) error {
	raw, err := proto.Marshal(resp)
	if err != nil {
		return fmt.Errorf("encoding CodeGeneratorResponse: %w", err)
	}
	if _, err := w.Write(raw); err != nil {
		return fmt.Errorf("writing CodeGeneratorResponse: %w", err)
	}
	return nil
}

// fail reports a fatal plugin error on stderr and exits non-zero, as the protocol
// requires. Partial output is never written.
func fail(err error) {
	fmt.Fprintf(os.Stderr, "%v\n", err)
	os.Exit(1)
}
