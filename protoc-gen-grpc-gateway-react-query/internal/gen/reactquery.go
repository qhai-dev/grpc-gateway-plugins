// Package gen renders the React Query file from the IR.
//
// The generated module is a set of hooks plus the query-key factory they share.
// Hooks take the client as a parameter rather than reading a global, so the same
// generated SDK works against several hosts, several axios configurations and in
// tests (docs/v1.md §5.8).
package gen

import (
	"fmt"
	"strings"

	"github.com/qhai-dev/protoc-gen-grpc-gateway-react-query/internal/contract"
	"github.com/qhai-dev/protoc-gen-grpc-gateway-react-query/internal/ir"
	"github.com/qhai-dev/protoc-gen-grpc-gateway-react-query/internal/ts"
	"github.com/qhai-dev/protoc-gen-grpc-gateway-react-query/internal/typeref"
)

const pluginName = "protoc-gen-grpc-gateway-react-query"

// Options are the plugin-specific parameters (docs/v1.md §9.4).
type Options struct {
	// HookPrefix is "none" or "service". "service" forces every hook to carry its
	// service name, which avoids collisions when several services of a file declare
	// the same method name.
	HookPrefix string
	// RequestOptional allows `request` to be undefined, defaulting `enabled` to
	// whether a request was supplied.
	RequestOptional bool
	// Queries lists fully qualified methods to treat as queries even though their
	// HTTP method is not GET, such as a POST-based search.
	Queries map[string]bool
	// AdditionalBindings is "ignore" or "methods"; it must match the Axios plugin
	// or the two generated APIs will not line up.
	AdditionalBindings string
	// StrictHTTP and Streaming mirror the Axios plugin so both plugins agree on
	// which methods exist.
	StrictHTTP bool
	Streaming  string
}

// DefaultOptions returns the documented defaults.
func DefaultOptions() *Options {
	return &Options{
		HookPrefix:         "none",
		RequestOptional:    true,
		Queries:            map[string]bool{},
		AdditionalBindings: "ignore",
		Streaming:          "ignore",
	}
}

// Set applies one plugin parameter.
func (o *Options) Set(name, value string) (bool, error) {
	switch name {
	case "hook_prefix":
		o.HookPrefix = value
	case "request_optional":
		b, err := parseBool(value)
		if err != nil {
			return true, fmt.Errorf("request_optional: %w", err)
		}
		o.RequestOptional = b
	case "queries":
		// protoc splits the plugin parameter string on commas, so a list arrives as one
		// `queries=` pair per entry after request.Prepare normalizes it.
		for _, q := range strings.Split(value, ",") {
			if q = strings.TrimSpace(q); q != "" {
				o.Queries[q] = true
			}
		}
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
		Queries:            o.Queries,
	}
}

// Validate rejects unsupported values.
func (o *Options) Validate() error {
	switch o.HookPrefix {
	case "none", "service":
	default:
		return fmt.Errorf("hook_prefix must be none or service, got %q", o.HookPrefix)
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

// Generator renders one React Query file.
type Generator struct {
	contract *contract.Options
	opts     *Options

	file    *ts.File
	render  *typeref.Renderer
	current *ir.File
	service *ir.Service

	// needsQuery and needsMutation decide which hooks are imported, so a file with
	// only queries does not import useMutation and vice versa.
	needsQuery    bool
	needsMutation bool
	// hookErr carries an error out of an indenting closure, which cannot return.
	hookErr error
}

// New starts a React Query generator.
func New(c *contract.Options, opts *Options) *Generator {
	return &Generator{contract: c, opts: opts}
}

// Generate renders the hooks file for one proto file.
func (g *Generator) Generate(f *ir.File) (string, error) {
	g.current = f
	g.needsQuery = false
	g.needsMutation = false

	scratch := ts.NewFile("")
	g.file = scratch
	g.render = typeref.New(g.contract, g.file, f.ProtoPath, nil)

	if err := g.writeBody(f); err != nil {
		return "", err
	}

	final := ts.NewFile(contract.Header(pluginName, f.ProtoPath))
	// The scratch file accumulated every import the body needs, including the
	// client class import added while rendering a hook signature.
	for _, imp := range scratch.Imports() {
		final.AddNamespaceImport(imp.Module, imp.Alias)
	}
	for _, imp := range scratch.TypeImports() {
		final.AddTypeImport(imp.Module, imp.Names...)
	}

	if g.needsQuery || g.needsMutation {
		// useQuery and useMutation are called at runtime, so they cannot be
		// type-only imports. verbatimModuleSyntax enforces that distinction.
		var runtime []string
		var optionTypes []string
		if g.needsQuery {
			runtime = append(runtime, "useQuery")
			optionTypes = append(optionTypes, "UseQueryOptions")
		}
		if g.needsMutation {
			runtime = append(runtime, "useMutation")
			optionTypes = append(optionTypes, "UseMutationOptions")
		}
		final.AddRuntimeImport("@tanstack/react-query", runtime...)
		final.AddTypeImport("@tanstack/react-query", optionTypes...)
	}

	final.PVerbatim(scratch.Body())
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

// writeService renders the key factory and the hooks of one service.
func (g *Generator) writeService(svc *ir.Service) error {
	g.service = svc

	if err := g.writeKeys(svc); err != nil {
		return err
	}
	for _, m := range svc.Methods {
		for _, b := range m.Bindings {
			g.file.P("")
			// Errors cannot propagate out of a rendering loop that mixes statements;
			// they are recorded and checked once.
			if err := g.writeHook(m, b); err != nil {
				return err
			}
			if g.hookErr != nil {
				return g.hookErr
			}
		}
	}
	return nil
}

// writeKeys renders the query-key factory.
//
// Every key contains the whole request, not just an id: a list query keyed on an
// id would serve stale data for a different filter (docs/v1.md §5.5). The factory
// is exported so applications can invalidate by prefix.
func (g *Generator) writeKeys(svc *ir.Service) error {
	if doc := ts.JSDoc(svc.Comment, ""); doc != "" {
		g.file.PVerbatim(doc)
	}
	g.file.P("export const %s = {", svc.KeysName)
	g.file.Indent(func() {
		// The `all` key scopes every other key of this service, so
		// invalidateQueries({ queryKey: keys.all }) clears the whole service.
		g.file.P("all: [%s] as const,", ir.Quote(svc.Name))
		for _, m := range svc.Methods {
			for _, b := range m.Bindings {
				g.file.P("")
				g.writeKeyEntry(m, b)
			}
		}
	})
	g.file.P("} as const;")
	return nil
}

// writeKeyEntry renders one key function.
func (g *Generator) writeKeyEntry(m *ir.Method, b *ir.Binding) {
	requestType := g.requestType(m)
	key := g.keyName(m, b)

	if g.isQuery(m, b) && g.opts.RequestOptional {
		// Queries may be called before the parameter is known, so the key accepts
		// undefined; mutations are always driven by a concrete request.
		g.file.P("%s: (request: %s | undefined) =>", key, requestType)
	} else {
		g.file.P("%s: (request: %s) =>", key, requestType)
	}
	g.file.P("  [...%s.all, %s, request] as const,", g.service.KeysName, ir.Quote(key))
}

// writeHook renders one hook.
func (g *Generator) writeHook(m *ir.Method, b *ir.Binding) error {
	if g.isQuery(m, b) {
		g.writeQueryHook(m, b)
		return nil
	}
	g.writeMutationHook(m, b)
	return nil
}

// clientType resolves the generated client class name and records the import of
// the file that declares it.
//
// The class lives in <base>_api.ts, which the contract derives from the proto path
// without reading that file, just as the Axios plugin derives this file's path
// without reading it (docs/v1.md §2.2). A named type import is used rather than a
// namespace one: there is exactly one class per service and the emitted hook
// signature reads better without a prefix.
func (g *Generator) clientType(svc *ir.Service) (string, error) {
	spec, _, ok := g.contract.ApiImportPath(g.current.ProtoPath, g.current.ProtoPath)
	if !ok {
		return "", fmt.Errorf("cannot resolve the client import for %s", g.current.ProtoPath)
	}
	g.file.AddTypeImport(spec, svc.ClientName)
	return svc.ClientName, nil
}

// writeQueryHook renders a useQuery hook.
func (g *Generator) writeQueryHook(m *ir.Method, b *ir.Binding) {
	g.needsQuery = true

	clientType, err := g.clientType(g.service)
	if err != nil {
		g.hookErr = err
		return
	}

	resultType := g.resultType(m, b)
	hookName := g.hookName(m, b)

	if doc := ts.JSDoc(m.Comment, ""); doc != "" {
		g.file.PVerbatim(doc)
	}
	g.file.P("export function %s(", hookName)
	g.file.Indent(func() {
		g.file.P("client: %s,", clientType)
		if g.opts.RequestOptional {
			g.file.P("request: %s | undefined,", g.requestType(m))
		} else {
			g.file.P("request: %s,", g.requestType(m))
		}
		g.file.P("options?: Omit<")
		g.file.Indent(func() {
			// The four type arguments are TQueryFnData, TError, TData and
			// TQueryKey. Giving UseQueryOptions its defaults (TData =
			// TQueryFnData) means a caller may pass `select` only when it preserves
			// the shape, which is the honest type for a generated hook: the hook
			// cannot know what shape a caller wants to select.
			g.file.P("UseQueryOptions<%s, Error, %s>,", resultType, resultType)
			// queryKey and queryFn belong to the generated code: a caller-supplied
			// one would fetch a different resource under this hook's key. `enabled`
			// is intentionally NOT omitted, because the caller may still force a
			// query on or off; it is read and defaulted below.
			g.file.P(`"queryKey" | "queryFn"`)
		})
		g.file.P(">,")
	})
	g.file.P(") {")
	g.file.Indent(func() {
		// Rest destructuring keeps every other option (select, staleTime, ...)
		// working without listing them.
		g.file.P("const { enabled, ...rest } = options ?? {};")
		g.file.P("")
		g.file.P("return useQuery({")
		g.file.Indent(func() {
			g.file.P("...rest,")
			g.file.P("queryKey: %s.%s(request),", g.service.KeysName, g.keyName(m, b))
			g.file.P("queryFn: ({ signal }) => {")
			g.file.Indent(func() {
				if g.opts.RequestOptional {
					g.file.P("if (request === undefined) {")
					g.file.P(`  return Promise.reject(new Error("request is required"));`)
					g.file.P("}")
				}
				// Passing the signal through is what makes React Query's
				// cancellation reach axios; without it an in-flight request keeps
				// running after the component unmounts (docs/v1.md §5.6).
				g.file.P("return client.%s(request, { signal });", b.ClientName)
			})
			g.file.P("},")
			if g.opts.RequestOptional {
				// Defaulting enabled to "a request was supplied" is what makes
				// `useGetUser(client, id ? { id } : undefined)` useful: it does not
				// fire until the id exists.
				g.file.P("enabled: enabled ?? request !== undefined,")
			} else {
				// The request is always present, so the query is enabled unless the
				// caller turns it off.
				g.file.P("enabled: enabled ?? true,")
			}
		})
		g.file.P("});")
	})
	g.file.P("}")
}

// writeMutationHook renders a useMutation hook.
func (g *Generator) writeMutationHook(m *ir.Method, b *ir.Binding) {
	g.needsMutation = true

	clientType, err := g.clientType(g.service)
	if err != nil {
		g.hookErr = err
		return
	}

	resultType := g.resultType(m, b)
	hookName := g.hookName(m, b)
	requestType := g.requestType(m)

	if doc := ts.JSDoc(m.Comment, ""); doc != "" {
		g.file.PVerbatim(doc)
	}
	g.file.P("export function %s(", hookName)
	g.file.Indent(func() {
		g.file.P("client: %s,", clientType)
		g.file.P("options?: UseMutationOptions<%s, Error, %s>,", resultType, requestType)
	})
	g.file.P(") {")
	g.file.Indent(func() {
		// ...options comes first so the generated mutationFn cannot be replaced: a
		// caller-supplied one would bypass the generated client entirely.
		g.file.P("return useMutation({")
		g.file.Indent(func() {
			g.file.P("...options,")
			g.file.P("mutationFn: (request: %s) => client.%s(request),", requestType, b.ClientName)
		})
		g.file.P("});")
	})
	g.file.P("}")
}

// hookName resolves the hook name, honouring hook_prefix=service.
func (g *Generator) hookName(m *ir.Method, b *ir.Binding) string {
	if g.opts.HookPrefix == "service" {
		// b.HookName is useGetUser; the service-scoped form is
		// useUserServiceGetUser.
		return "use" + g.service.Name + strings.TrimPrefix(b.HookName, "use")
	}
	return b.HookName
}

// keyName resolves the key factory entry name, matching the hook name so the two
// cannot drift apart.
func (g *Generator) keyName(m *ir.Method, b *ir.Binding) string {
	return lowerFirst(strings.TrimPrefix(g.hookName(m, b), "use"))
}

// isQuery decides whether a method is generated as a query or a mutation.
//
// The default is by HTTP method, with an explicit override for endpoints that use
// POST for a read (docs/v1.md §5.9).
func (g *Generator) isQuery(m *ir.Method, b *ir.Binding) bool {
	if m.Proto != nil && g.opts.Queries[string(m.Proto.Desc.FullName())] {
		return true
	}
	return b.HTTPMethod == "GET"
}

// requestType resolves the request message type name.
func (g *Generator) requestType(m *ir.Method) string {
	if m.Input == nil {
		// An unresolvable request type is a generator bug caught by the IR builder;
		// rendering `unknown` keeps the generated file valid nonetheless.
		return "unknown"
	}
	return g.render.Message(m.Input)
}

// resultType resolves what the hook yields: the response message, or the
// response_body field when the gateway unwraps it. It must match the Axios
// plugin's return type or the two generated APIs will not line up.
func (g *Generator) resultType(m *ir.Method, b *ir.Binding) string {
	if b.ResponseBodyAccess != nil {
		return g.render.Type(b.ResponseBodyAccess.Type, b.ResponseBodyAccess.Repeated)
	}
	return g.render.Message(m.Output)
}

// lowerFirst lowercases the first byte.
func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}
