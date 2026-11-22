// Package ir builds the shared intermediate representation from a protogen
// request.
package ir

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/qhai-dev/protoc-gen-grpc-gateway-axios/internal/contract"
	"github.com/qhai-dev/protoc-gen-grpc-gateway-axios/internal/httprule"
)

// BuildOptions tunes IR construction.
type BuildOptions struct {
	// AdditionalBindings is "ignore" or "methods".
	AdditionalBindings string
	// StrictHTTP errors on methods without a google.api.http rule.
	StrictHTTP bool
	// Streaming is "ignore" or "error".
	Streaming string
	// Queries lists fully qualified methods to treat as queries regardless of
	// their HTTP method.
	Queries map[string]bool
}

// Builder resolves protogen descriptors into an IR.
type Builder struct {
	contract *contract.Options
	opts     BuildOptions
	ir       *IR
}

// NewBuilder starts a builder.
func NewBuilder(c *contract.Options, opts BuildOptions) *Builder {
	return &Builder{
		contract: c,
		opts:     opts,
		ir: &IR{
			messages: map[protoreflect.FullName]*Message{},
			enums:    map[protoreflect.FullName]*Enum{},
		},
	}
}

// Build indexes every file in the request and returns the IR.
//
// Every file is indexed, including imports that are never generated: a field may
// reference a type from an imported proto, and that type still has to be named.
// Only files that protogen marks for generation produce output.
func (b *Builder) Build(gen *protogen.Plugin) (*IR, error) {
	b.ir.Request = gen.Files

	// Pass 1: create a File node for every proto file, so that a type can always
	// name the file it belongs to regardless of request order.
	files := make(map[protoreflect.FullName]*File, len(gen.Files))
	for _, f := range gen.Files {
		file := &File{
			Proto:      f,
			ProtoPath:  f.Desc.Path(),
			Package:    string(f.Desc.Package()),
			OutputBase: contract.FileBase(f.Desc.Path()),
		}
		files[f.Desc.Package()] = file
		if f.Generate && !b.contract.SkipProto(f.Desc.Path()) {
			b.ir.Files = append(b.ir.Files, file)
		}
	}

	// Pass 2: index every message and enum before resolving any field, so a
	// forward reference inside the same file resolves.
	for _, f := range gen.Files {
		file := files[f.Desc.Package()]
		for _, e := range f.Enums {
			b.indexEnum(e, file, nil)
		}
		for _, m := range f.Messages {
			b.indexMessage(m, file, nil)
		}
	}

	// Pass 3: fill in fields, services and bindings for generated files only.
	for _, file := range b.ir.Files {
		b.resolveFile(file)
		if err := b.resolveServices(file); err != nil {
			return nil, err
		}
	}

	return b.ir, nil
}

// indexEnum registers one enum, nesting included in its TypeScript name.
func (b *Builder) indexEnum(pe *protogen.Enum, file *File, scope []string) {
	parts := append(append([]string{}, scope...), string(pe.Desc.Name()))
	e := &Enum{
		TSName:   b.contract.TypeName(parts),
		FullName: pe.Desc.FullName(),
		File:     file,
		Proto:    pe,
	}
	for _, v := range pe.Values {
		e.Values = append(e.Values, &EnumValue{
			Name:    string(v.Desc.Name()),
			Number:  int32(v.Desc.Number()),
			Comment: string(v.Comments.Leading),
		})
	}
	b.ir.putEnum(e)
}

// indexMessage registers one message and recurses into its children.
func (b *Builder) indexMessage(pm *protogen.Message, file *File, scope []string) {
	if pm.Desc.IsMapEntry() {
		return
	}
	parts := append(append([]string{}, scope...), string(pm.Desc.Name()))
	m := &Message{
		TSName:   b.contract.TypeName(parts),
		FullName: pm.Desc.FullName(),
		WKT:      ClassifyMessage(pm.Desc),
		File:     file,
		Proto:    pm,
	}
	b.ir.putMessage(m)

	for _, e := range pm.Enums {
		b.indexEnum(e, file, parts)
	}
	for _, nested := range pm.Messages {
		b.indexMessage(nested, file, parts)
	}
}

// resolveFile attaches fields, oneofs and nested declarations, using protogen
// nodes because they carry the source comments.
func (b *Builder) resolveFile(file *File) {
	for _, pe := range file.Proto.Enums {
		if e := b.ir.LookupEnum(pe.Desc.FullName()); e != nil {
			e.Comment = string(pe.Comments.Leading)
			file.Enums = append(file.Enums, e)
		}
	}
	for _, pm := range file.Proto.Messages {
		if pm.Desc.IsMapEntry() {
			continue
		}
		file.Messages = append(file.Messages, b.resolveMessage(pm))
	}
}

func (b *Builder) resolveMessage(pm *protogen.Message) *Message {
	m := b.ir.LookupMessage(pm.Desc.FullName())
	if m == nil {
		return nil
	}
	m.Comment = string(pm.Comments.Leading)

	for _, pe := range pm.Enums {
		if e := b.ir.LookupEnum(pe.Desc.FullName()); e != nil {
			e.Comment = string(pe.Comments.Leading)
			m.Enums = append(m.Enums, e)
		}
	}
	for _, nested := range pm.Messages {
		if nested.Desc.IsMapEntry() {
			continue
		}
		if nm := b.resolveMessage(nested); nm != nil {
			m.Messages = append(m.Messages, nm)
		}
	}

	// A synthetic oneof exists only to carry proto3 `optional`; it must never be
	// modelled as a business oneof, or the field would be forced into a union.
	oneofs := make(map[string]*Oneof, len(pm.Oneofs))
	for _, po := range pm.Oneofs {
		if po.Desc.IsSynthetic() {
			continue
		}
		o := &Oneof{Name: string(po.Desc.Name())}
		oneofs[o.Name] = o
		m.Oneofs = append(m.Oneofs, o)
	}

	for _, pf := range pm.Fields {
		f := &Field{
			Name:      b.contract.FieldName(pf.Desc.JSONName(), string(pf.Desc.Name())),
			ProtoName: string(pf.Desc.Name()),
			JSONName:  pf.Desc.JSONName(),
			Comment:   string(pf.Comments.Leading),
			Type:      b.resolveType(pf.Desc),
			Proto:     pf,
		}
		if pf.Desc.IsList() {
			f.Cardinality = Repeated
		}
		if od := pf.Desc.ContainingOneof(); od != nil && !od.IsSynthetic() {
			if o, ok := oneofs[string(od.Name())]; ok {
				o.Fields = append(o.Fields, f)
				f.Oneof = o
			}
		}
		m.Fields = append(m.Fields, f)
	}
	return m
}

// resolveType maps a field descriptor to a Type.
func (b *Builder) resolveType(fd protoreflect.FieldDescriptor) *Type {
	if fd.IsMap() {
		return &Type{
			Kind:     KindMap,
			MapKey:   b.resolveSingular(fd.MapKey()),
			MapValue: b.resolveSingular(fd.MapValue()),
		}
	}
	return b.resolveSingular(fd)
}

func (b *Builder) resolveSingular(fd protoreflect.FieldDescriptor) *Type {
	switch fd.Kind() {
	case protoreflect.EnumKind:
		if IsNullValue(fd.Enum().FullName()) {
			return &Type{Kind: KindScalar, Scalar: protoreflect.EnumKind, WKT: WKTValue}
		}
		return &Type{Kind: KindEnum, Enum: b.ir.LookupEnum(fd.Enum().FullName())}

	case protoreflect.MessageKind, protoreflect.GroupKind:
		wkt := ClassifyMessage(fd.Message())
		if wkt == WKTAny {
			return &Type{Kind: KindAny, WKT: WKTAny}
		}
		if wkt != WKTNone {
			return &Type{Kind: KindMessage, WKT: wkt}
		}
		return &Type{Kind: KindMessage, Message: b.ir.LookupMessage(fd.Message().FullName())}

	default:
		return &Type{Kind: KindScalar, Scalar: fd.Kind()}
	}
}

// resolveServices builds every service, method and binding of a generated file.
func (b *Builder) resolveServices(file *File) error {
	used := map[string]int{}

	for _, ps := range file.Proto.Services {
		svc := &Service{
			Name:       string(ps.Desc.Name()),
			Comment:    string(ps.Comments.Leading),
			ClientName: contract.ClientName(string(ps.Desc.Name())),
			ClientVar:  contract.ClientVarName(string(ps.Desc.Name())),
			KeysName:   contract.KeysName(string(ps.Desc.Name())),
			Proto:      ps,
		}
		for _, pm := range ps.Methods {
			m, err := b.buildMethod(pm, used)
			if err != nil {
				return fmt.Errorf("%s: %w", pm.Desc.FullName(), err)
			}
			svc.Methods = append(svc.Methods, m)
		}
		file.Services = append(file.Services, svc)
	}
	return nil
}

func (b *Builder) buildMethod(pm *protogen.Method, used map[string]int) (*Method, error) {
	m := &Method{
		Name:      string(pm.Desc.Name()),
		Comment:   string(pm.Comments.Leading),
		Input:     b.ir.LookupMessage(pm.Input.Desc.FullName()),
		Output:    b.ir.LookupMessage(pm.Output.Desc.FullName()),
		Streaming: pm.Desc.IsStreamingClient() || pm.Desc.IsStreamingServer(),
		Proto:     pm,
	}

	base := contract.LowerCamel(string(pm.Desc.Name()))

	if m.Streaming {
		if b.opts.Streaming == "error" {
			return nil, fmt.Errorf("streaming RPCs are not supported in v1 (streaming=error)")
		}
		m.SkipReason = "streaming RPC: not supported in v1"
		m.ClientName = uniqueName(base, used)
		m.HookName = contract.HookName(m.ClientName)
		return m, nil
	}

	rules, err := httprule.Rules(pm.Desc)
	if err != nil {
		return nil, err
	}
	if len(rules) == 0 {
		if b.opts.StrictHTTP {
			return nil, fmt.Errorf("missing google.api.http annotation (strict_http=true)")
		}
		m.SkipReason = "no google.api.http annotation"
		m.ClientName = uniqueName(base, used)
		m.HookName = contract.HookName(m.ClientName)
		return m, nil
	}

	limit := len(rules)
	if b.opts.AdditionalBindings != "methods" {
		limit = 1
	}

	for i := 0; i < limit; i++ {
		rule := rules[i]
		name := base
		if i > 0 {
			name = fmt.Sprintf("%sBinding%d", base, i+1)
		}
		name = uniqueName(name, used)

		binding := &Binding{
			HTTPMethod:   rule.Method,
			Template:     rule.Template,
			ResponseBody: rule.ResponseBody,
			IsPrimary:    i == 0,
			ClientName:   name,
			HookName:     contract.HookName(name),
			BodyAll:      rule.Body.IsRoot(),
		}
		if !binding.BodyAll {
			binding.Body = rule.Body
		}

		// Resolve every access chain up front so templates only render strings.
		for _, v := range rule.Template.Variables() {
			acc, err := b.ResolveAccess(pm.Desc.Input(), v.FieldPath)
			if err != nil {
				return nil, err
			}
			binding.PathVariables = append(binding.PathVariables, &PathVariable{
				FieldPath: v.FieldPath,
				Access:    acc,
				MultiSeg:  v.MultiSeg,
			})
		}
		if !binding.BodyAll && !binding.Body.Empty() {
			acc, err := b.ResolveAccess(pm.Desc.Input(), binding.Body)
			if err != nil {
				return nil, err
			}
			binding.BodyAccess = acc
		}
		if !binding.ResponseBody.Empty() {
			acc, err := b.ResolveAccess(pm.Desc.Output(), binding.ResponseBody)
			if err != nil {
				return nil, err
			}
			binding.ResponseBodyAccess = acc
		}

		leaves, err := httprule.QueryLeaves(
			pm.Desc.Input(), rule.Template.BoundPaths(), rule.Body, binding.BodyAll,
		)
		if err != nil {
			return nil, err
		}
		for _, leaf := range leaves {
			if leaf.SkipReason != "" {
				// The gateway cannot parse this leaf from a query parameter, so
				// emitting it would produce a runtime 400 on every call.
				continue
			}
			qf, err := b.buildQueryField(pm.Desc.Input(), leaf)
			if err != nil {
				return nil, err
			}
			binding.QueryFields = append(binding.QueryFields, qf)
		}

		m.Bindings = append(m.Bindings, binding)
	}

	m.ClientName = m.Bindings[0].ClientName
	m.HookName = m.Bindings[0].HookName
	return m, nil
}

func (b *Builder) buildQueryField(root protoreflect.MessageDescriptor, leaf httprule.Leaf) (*QueryField, error) {
	acc, err := b.ResolveAccess(root, leaf.Path)
	if err != nil {
		return nil, err
	}
	// The query key uses the emitted field names joined with dots, so it matches
	// the property names in the generated request type. The gateway resolves both
	// proto and JSON spellings, so either is safe; consistency with the types file
	// is what makes the generated code readable.
	key := strings.Join(acc.Hops, ".")
	return &QueryField{
		Path:       leaf.Path,
		Access:     acc,
		Key:        key,
		Leaf:       acc.Type,
		IsRepeated: leaf.Field.IsList(),
		IsMap:      leaf.Field.IsMap(),
	}, nil
}

// uniqueName keeps exported names unique inside one generated module, because
// every service of a file lands in the same file.
func uniqueName(name string, used map[string]int) string {
	used[name]++
	if used[name] == 1 {
		return name
	}
	return fmt.Sprintf("%s%d", name, used[name])
}
