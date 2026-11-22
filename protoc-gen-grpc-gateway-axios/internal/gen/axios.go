// Package gen renders the Axios client file from the IR.
//
// Every method is generated explicitly: the URL is assembled from literals and
// interpolated path variables, query parameters are appended to a URLSearchParams,
// and the body is picked out of the request. Nothing is left to Axios's default
// parameter serialization, because that serialization is not what grpc-gateway
// parses (docs/v1.md §4.8).
package gen

import (
	"fmt"
	"strings"

	"github.com/qhai-dev/protoc-gen-grpc-gateway-axios/internal/contract"
	"github.com/qhai-dev/protoc-gen-grpc-gateway-axios/internal/ir"
	"github.com/qhai-dev/protoc-gen-grpc-gateway-axios/internal/ts"
	"github.com/qhai-dev/protoc-gen-grpc-gateway-axios/internal/typeref"
)

const pluginName = "protoc-gen-grpc-gateway-axios"

// Options are the plugin-specific parameters (docs/v1.md §9.3).
type Options struct {
	// ClientStyle is "class" or "functions".
	ClientStyle string
	// AdditionalBindings is "ignore" or "methods"; the IR builder decides how many
	// bindings exist from it.
	AdditionalBindings string
	// StrictHTTP errors on methods without a google.api.http rule.
	StrictHTTP bool
	// Streaming is "ignore" or "error".
	Streaming string
}

// DefaultOptions returns the documented defaults.
func DefaultOptions() *Options {
	return &Options{
		ClientStyle:        "class",
		AdditionalBindings: "ignore",
		Streaming:          "ignore",
	}
}

// Set applies one plugin parameter.
func (o *Options) Set(name, value string) (bool, error) {
	switch name {
	case "client_style":
		o.ClientStyle = value
	case "additional_bindings":
		o.AdditionalBindings = value
	case "strict_http":
		b, err := parseBool(value)
		if err != nil {
			return true, fmt.Errorf("strict_http: %w", err)
		}
		o.StrictHTTP = b
	case "streaming":
		o.Streaming = value
	default:
		return false, nil
	}
	return true, nil
}

// BuildOptions exposes the IR options implied by these parameters.
func (o *Options) BuildOptions() ir.BuildOptions {
	return ir.BuildOptions{
		AdditionalBindings: o.AdditionalBindings,
		StrictHTTP:         o.StrictHTTP,
		Streaming:          o.Streaming,
	}
}

// Validate rejects unsupported values.
func (o *Options) Validate() error {
	switch o.ClientStyle {
	case "class", "functions":
	default:
		return fmt.Errorf("client_style must be class or functions, got %q", o.ClientStyle)
	}
	switch o.AdditionalBindings {
	case "ignore", "methods":
	default:
		return fmt.Errorf("additional_bindings must be ignore or methods, got %q", o.AdditionalBindings)
	}
	switch o.Streaming {
	case "ignore", "error":
	default:
		return fmt.Errorf("streaming must be ignore or error, got %q", o.Streaming)
	}
	return nil
}

func parseBool(value string) (bool, error) {
	switch strings.ToLower(value) {
	case "true", "1", "yes":
		return true, nil
	case "false", "0", "no":
		return false, nil
	}
	return false, fmt.Errorf("expected true or false, got %q", value)
}

// Generator renders one Axios client file.
type Generator struct {
	contract *contract.Options
	opts     *Options

	file    *ts.File
	render  *typeref.Renderer
	current *ir.File

	// needsEncodePathSegments is set when a multi-segment path variable is emitted.
	needsEncodePathSegments bool
	// methodErr carries an error out of an indenting closure, which cannot return.
	methodErr error
	// statementErr mirrors methodErr for the method body.
	statementErr error
}

// New starts an Axios generator.
func New(c *contract.Options, opts *Options) *Generator {
	return &Generator{contract: c, opts: opts}
}

// Generate renders the client file for one proto file.
//
// Declarations are built into a scratch buffer first, so helper requirements are
// known before the prelude is written: the helper must precede its uses. The
// scratch buffer is rendered with its final indentation already applied, because a
// class body is indented as a whole and replaying it verbatim is simpler than
// tracking depth twice.
func (g *Generator) Generate(f *ir.File) (string, error) {
	g.current = f
	g.needsEncodePathSegments = false

	scratch := ts.NewFile("")
	g.file = scratch
	g.render = typeref.New(g.contract, g.file, f.ProtoPath, nil)

	if err := g.writeBody(f); err != nil {
		return "", err
	}

	// The scratch body was rendered at whatever depth each declaration ended at,
	// so it is replayed verbatim below rather than re-indented.
	final := ts.NewFile(contract.Header(pluginName, f.ProtoPath))
	for _, imp := range scratch.Imports() {
		final.AddNamespaceImport(imp.Module, imp.Alias)
	}
	// A proto file with no service still produces an output file, so build systems
	// can declare it up front. That file declares nothing, and must stay
	// import-free: an unused type import is noise at best and a lint error under
	// noUnusedLocals.
	if body := scratch.Body(); body != "" {
		// Axios supplies the instance and the per-call configuration the generated
		// code passes through for cancellation and headers.
		final.AddTypeImport("axios", "AxiosInstance", "AxiosRequestConfig")
		_ = body
	}

	if body := scratch.Body(); body != "" {
		// Axios supplies the instance and the per-call configuration the generated
		// code passes through for cancellation and headers.
		final.AddTypeImport("axios", "AxiosInstance", "AxiosRequestConfig")

		if g.needsEncodePathSegments {
			final.PVerbatim(encodePathSegmentsHelper)
			final.P("")
		}
		final.PVerbatim(body)
	}
	return final.WriteString(), nil
}

// writeBody renders every service of the file.
func (g *Generator) writeBody(f *ir.File) error {
	wrote := false
	for _, svc := range f.Services {
		if wrote {
			g.file.P("")
		}
		if err := g.writeService(svc); err != nil {
			return err
		}
		wrote = true
	}
	return nil
}

// encodePathSegmentsHelper encodes a multi-segment path variable segment by
// segment.
//
// A variable such as {name=shippers/*} carries slashes in its value, and the
// gateway matches those slashes as path separators. Encoding the whole value as
// one token turns `shippers/123` into `shippers%2F123`, which no longer matches
// the route (docs/v1.md §4.7). The helper is inlined per file rather than shared,
// so every generated file is self-contained and Bazel actions stay independent
// (docs/v1.md §4.15).
const encodePathSegmentsHelper = `const encodePathSegments = (value: unknown): string =>
  String(value)
    .split("/")
    .map(encodeURIComponent)
    .join("/");`

// writeService renders one service.
func (g *Generator) writeService(svc *ir.Service) error {
	if g.opts.ClientStyle == "functions" {
		return g.writeFunctionService(svc)
	}
	return g.writeClassService(svc)
}

// writeClassService renders `export class UserServiceClient`.
func (g *Generator) writeClassService(svc *ir.Service) error {
	if doc := ts.JSDoc(svc.Comment, ""); doc != "" {
		g.file.PVerbatim(doc)
	}
	g.file.P("export class %s {", svc.ClientName)
	g.file.Indent(func() {
		g.file.P("constructor(private readonly http: AxiosInstance) {}")
		for _, m := range svc.Methods {
			g.file.P("")
			if err := g.writeMethod(m, true); err != nil {
				g.methodErr = err
				return
			}
		}
	})
	g.file.P("}")
	if g.methodErr != nil {
		return g.methodErr
	}
	return nil
}

// writeFunctionService renders one exported function per method.
func (g *Generator) writeFunctionService(svc *ir.Service) error {
	for i, m := range svc.Methods {
		if i > 0 {
			g.file.P("")
		}
		if err := g.writeMethod(m, false); err != nil {
			return err
		}
	}
	return nil
}

// writeMethod renders every binding of one RPC.
func (g *Generator) writeMethod(m *ir.Method, inClass bool) error {
	if len(m.Bindings) == 0 {
		// A skipped method leaves a trace, so the missing API surface is explained
		// in the generated file rather than only in the generator's head.
		g.file.P("// %s not generated: %s", m.Name, m.SkipReason)
		return nil
	}
	for i, b := range m.Bindings {
		if i > 0 {
			g.file.P("")
		}
		if err := g.writeBinding(m, b, inClass); err != nil {
			return err
		}
	}
	return nil
}

// writeBinding renders one HTTP binding as a class method or a function.
func (g *Generator) writeBinding(m *ir.Method, b *ir.Binding, inClass bool) error {
	if m.Input == nil {
		return fmt.Errorf("%s: request type is not part of this request", m.Name)
	}

	// response_body makes the gateway marshal only that field, so the HTTP body is
	// the field value. Both the axios generic and the promise type are therefore the
	// field's type, never the wrapper message.
	responseType := g.render.Message(m.Output)
	if b.ResponseBodyAccess != nil {
		responseType = g.render.Type(b.ResponseBodyAccess.Type, b.ResponseBodyAccess.Repeated)
	}

	if doc := ts.JSDoc(m.Comment, strings.Repeat("  ", g.file.Depth())); doc != "" {
		g.file.PVerbatim(doc)
	}

	if inClass {
		g.file.P("async %s(", b.ClientName)
		g.file.Indent(func() {
			g.file.P("request: %s,", g.render.Message(m.Input))
			g.file.P("config?: AxiosRequestConfig,")
		})
	} else {
		// Function-style clients take the instance as the first parameter, since
		// there is no constructor to receive it.
		g.file.P("export async function %s(", b.ClientName)
		g.file.Indent(func() {
			g.file.P("http: AxiosInstance,")
			g.file.P("request: %s,", g.render.Message(m.Input))
			g.file.P("config?: AxiosRequestConfig,")
		})
	}
	g.file.P("): Promise<%s> {", responseType)

	g.file.Indent(func() {
		g.statementErr = g.writeStatements(b, responseType)
	})
	g.file.P("}")
	return g.statementErr
}

// writeStatements renders the body of one method.
func (g *Generator) writeStatements(b *ir.Binding, responseType string) error {
	// Path variables must be present: a missing one produces a URL such as
	// `/v1/users/undefined`, which the gateway may route to a handler with a
	// literal "undefined" id.
	for i, v := range b.PathVariables {
		if i > 0 {
			g.file.P("")
		}
		// Only `== null` is used. A truthiness test would reject 0, false and "",
		// all of which are legitimate values (docs/v1.md §4.7).
		g.file.P("if (%s == null) {", v.Access.String())
		g.file.P("  throw new Error(%s);", ir.Quote("missing required path parameter: "+v.FieldPath.String()))
		g.file.P("}")
	}

	if len(b.QueryFields) > 0 {
		if len(b.PathVariables) > 0 {
			g.file.P("")
		}
		g.writeQuery(b)
	}

	if len(b.PathVariables) > 0 || len(b.QueryFields) > 0 {
		g.file.P("")
	}
	g.writeRequest(b, responseType)
	return nil
}

// writeQuery collects query parameters into a URLSearchParams.
//
// URLSearchParams is used rather than axios `params` because it applies the
// encoding the gateway's url.ParseQuery expects, needs no global paramsSerializer,
// and keeps the generated code readable (docs/v1.md §4.8).
func (g *Generator) writeQuery(b *ir.Binding) {
	g.file.P("const query = new URLSearchParams();")
	for _, qf := range b.QueryFields {
		g.writeQueryField(qf)
	}
	g.file.P("const queryString = query.toString();")
}

// writeQueryField emits the append statements for one leaf field.
func (g *Generator) writeQueryField(qf *ir.QueryField) {
	key := qf.Key
	access := qf.Access.String()

	switch {
	case qf.IsMap:
		// The bracket form is mandatory: the gateway parses `name[key]=value` and
		// URL-decodes both halves. `name.key=value` would be read as a field path
		// and rejected (docs/v1.md §4.8). The key keeps its brackets literal and
		// the map entry's key is interpolated inside them.
		g.file.P("Object.entries(%s ?? {}).forEach(([key, value]) => {", access)
		g.file.P("  query.append(`%s[${key}]`, String(value));", key)
		g.file.P("});")

	case qf.IsRepeated:
		// Repeated values become same-name repeated parameters: ids=1&ids=2.
		g.file.P("(%s ?? []).forEach((value) => {", access)
		g.file.P("  query.append(%s, String(value));", ir.Quote(key))
		g.file.P("});")

	default:
		// Only null and undefined are omitted, so 0, false and "" still reach the
		// gateway (docs/v1.md §4.8).
		g.file.P("if (%s != null) {", access)
		g.file.P("  query.append(%s, String(%s));", ir.Quote(key), access)
		g.file.P("}")
	}
}

// writeRequest emits the axios call and the response unwrapping.
func (g *Generator) writeRequest(b *ir.Binding, responseType string) {
	// A class method reads the injected private field; a function reads its
	// parameter. Both are named `http`, so only the receiver differs.
	http := "http."
	if g.opts.ClientStyle != "functions" {
		http = "this.http."
	}

	// response_body is reflected in the response *shape*, not in a nested access:
	// grpc-gateway marshals the selected field's value as the whole HTTP body, so
	// there is nothing to unwrap. Only the generic type argument differs, which is
	g.file.P("const response = await %srequest<%s>({", http, responseType)
	g.file.Indent(func() {
		g.file.P("...config,")
		g.file.P("url: %s,", g.urlExpression(b))
		g.file.P("method: %s,", ir.Quote(b.HTTPMethod))
		switch {
		case b.BodyAll:
			g.file.P("data: request,")
		case b.BodyAccess != nil:
			g.file.P("data: %s,", b.BodyAccess.String())
		}
	})
	g.file.P("});")
	g.file.P("")
	if b.ResponseBodyAccess != nil {
		// response_body changes what the server sends, not just how it is nested.
		//
		// grpc-gateway generates a wrapper whose XXX_ResponseBody() returns the
		// selected field, and ForwardResponseMessage marshals that return value
		// directly. So the HTTP body *is* the field value: `response.data` is already
		// the User, not a GetUserResponse with a `user` property. docs/v1.md §4.10
		// describes an unwrap that does not exist on the wire.
		g.file.P("return response.data;")
		return
	}
	g.file.P("return response.data;")
}

// urlExpression renders the URL assembly, including query string appending.
func (g *Generator) urlExpression(b *ir.Binding) string {
	var literal strings.Builder
	literal.WriteString("`")
	for _, seg := range b.Template.Segments {
		literal.WriteString("/")
		if seg.Variable == nil {
			literal.WriteString(escapeTemplate(seg.Literal))
			continue
		}
		var access *ir.Access
		for _, v := range b.PathVariables {
			if v.FieldPath.String() == seg.Variable.FieldPath.String() {
				access = v.Access
				break
			}
		}
		if access == nil {
			// Unreachable: PathVariables is built from these same template
			// variables. Failing loudly beats emitting a silently wrong URL.
			panic("unresolved path variable: " + seg.Variable.FieldPath.String())
		}
		literal.WriteString("${")
		if seg.Variable.MultiSeg {
			// A multi-segment value keeps its slashes literal.
			g.needsEncodePathSegments = true
			literal.WriteString("encodePathSegments(")
			literal.WriteString(access.String())
			literal.WriteString(")")
		} else {
			literal.WriteString("encodeURIComponent(String(")
			literal.WriteString(access.String())
			literal.WriteString("))")
		}
		literal.WriteString("}")
	}
	if b.Template.Verb != "" {
		// The verb is part of the registered route, so it stays outside the
		// variable and outside the encoding.
		literal.WriteString(":")
		literal.WriteString(escapeTemplate(b.Template.Verb))
	}
	literal.WriteString("`")

	if len(b.QueryFields) == 0 {
		return literal.String()
	}
	// The separator is only added when there is actually a query string, so a call
	// with no parameters produces the bare registered path.
	return fmt.Sprintf("%s.concat(queryString === \"\" ? \"\" : `?${queryString}`)", literal.String())
}

// escapeTemplate escapes backticks and interpolation markers, so a literal path
// segment cannot break out of the template string.
func escapeTemplate(s string) string {
	s = strings.ReplaceAll(s, "`", "\\`")
	return strings.ReplaceAll(s, "${", "\\${")
}
