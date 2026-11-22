package contract

import (
	"fmt"
	"path"
	"strings"
)

// FileBase returns the proto path with the .proto suffix removed. This is the
// single anchor for every output path and import path in the contract.
//
//	user/v1/user.proto -> user/v1/user
func FileBase(protoPath string) string {
	return strings.TrimSuffix(protoPath, ".proto")
}

// TypesFile returns the generated types file path for a proto path.
func (o *Options) TypesFile(protoPath string) string {
	return FileBase(protoPath) + o.TypesSuffix
}

// ApiFile returns the generated Axios file path for a proto path.
func (o *Options) ApiFile(protoPath string) string {
	return FileBase(protoPath) + o.ApiSuffix
}

// QueryFile returns the generated React Query file path for a proto path.
func (o *Options) QueryFile(protoPath string) string {
	return FileBase(protoPath) + o.QuerySuffix
}

// TypesImportPath returns the module specifier and namespace alias used to
// import the generated types file for targetProto from the file at sourceProto.
//
// It returns ok=false for proto files that must never be generated or imported:
// well-known types are inlined as TypeScript primitives, and the google.api
// descriptors exist only to carry annotations.
func (o *Options) TypesImportPath(sourceProto, targetProto string) (spec, alias string, ok bool) {
	if o.SkipProto(targetProto) {
		return "", "", false
	}
	alias = ImportAlias(targetProto, o.TypesSuffix)
	if module, mapped := o.ModuleMap[targetProto]; mapped {
		// Package specifiers never get the relative import extension.
		return module, alias, true
	}
	return o.relativeImport(sourceProto, o.TypesFile(targetProto)), alias, true
}

// ApiImportPath returns the module specifier and namespace alias used to import
// the generated Axios file for targetProto from the file at sourceProto.
func (o *Options) ApiImportPath(sourceProto, targetProto string) (spec, alias string, ok bool) {
	if o.SkipProto(targetProto) {
		return "", "", false
	}
	return o.relativeImport(sourceProto, o.ApiFile(targetProto)), ImportAlias(targetProto, o.ApiSuffix), true
}

// QueryImportPath returns the module specifier and namespace alias used to import
// the generated React Query file for targetProto.
func (o *Options) QueryImportPath(sourceProto, targetProto string) (spec, alias string, ok bool) {
	if o.SkipProto(targetProto) {
		return "", "", false
	}
	return o.relativeImport(sourceProto, o.QueryFile(targetProto)), ImportAlias(targetProto, o.QuerySuffix), true
}

// relativeImport builds a relative module specifier.
//
// import_extension exists for NodeNext, where the TypeScript source file
// `address_type.ts` is imported as `./address_type.js`: the emitted specifier must
// name the runtime file, not the source file. So only the `.ts` extension is
// replaced, never the whole configured suffix: `user.types.ts` with
// import_extension=.js becomes `user.types.js`, and a naive whole-suffix trim
// would have produced `user.js`.
//
// With an empty import_extension the specifier is extensionless, which is what
// bundler-style resolution (`moduleResolution: bundler`) expects.
func (o *Options) relativeImport(sourceProto, targetFile string) string {
	target := strings.TrimSuffix(targetFile, ".ts") + o.ImportExtension
	return relativePath(path.Dir(FileBase(sourceProto)+".ts"), target)
}

// relativePath builds a `./`-prefixed relative module specifier from one
// directory to another file.
func relativePath(fromFileDir, target string) string {
	from := splitDir(fromFileDir)
	to := splitDir(path.Dir(target))

	common := 0
	for common < len(from) && common < len(to) && from[common] == to[common] {
		common++
	}

	parts := make([]string, 0, len(from)-common+len(to)-common+1)
	for i := common; i < len(from); i++ {
		parts = append(parts, "..")
	}
	parts = append(parts, to[common:]...)
	parts = append(parts, path.Base(target))

	joined := path.Join(parts...)
	// path.Join keeps leading "..", so only prefix "./" for sibling or deeper
	// paths. A specifier like "./../x" is valid but noise.
	if strings.HasPrefix(joined, "..") {
		return joined
	}
	return "./" + joined
}

func splitDir(dir string) []string {
	if dir == "." || dir == "" {
		return nil
	}
	return strings.Split(dir, "/")
}

// ImportAlias derives the namespace import alias for a proto file. Aliases are
// deterministic and never collide between files that share a base name in
// different directories, because the whole path is folded into one identifier.
//
//	user/v1/address.proto (+ _type.ts) -> user_v1_address_type
func ImportAlias(protoPath, suffix string) string {
	name := FileBase(protoPath) + strings.TrimSuffix(suffix, path.Ext(suffix))

	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	alias := b.String()
	if alias == "" || (alias[0] >= '0' && alias[0] <= '9') {
		alias = "_" + alias
	}
	return alias
}

// SkipProto reports whether a proto file is deliberately outside the generated
// set. Buf with include_imports lists google/protobuf and google/api protos in
// file_to_generate; generating timestamp_type.ts there would be noise, and the
// types plugin inlines those types anyway.
func (o *Options) SkipProto(protoPath string) bool {
	switch {
	case strings.HasPrefix(protoPath, "google/protobuf/"),
		protoPath == "google/api/annotations.proto",
		protoPath == "google/api/http.proto",
		protoPath == "google/api/httpbody.proto",
		strings.HasPrefix(protoPath, "google/api/"):
		return true
	}
	return false
}

// Header renders the stable file banner. No timestamps, no absolute paths: two
// runs over the same input must produce byte-identical files.
func Header(pluginName, protoPath string) string {
	return fmt.Sprintf(
		"// Code generated by %s. DO NOT EDIT.\n// @grpc-gateway-typescript-contract: %s\n// source: %s\n",
		pluginName, ContractVersion, protoPath,
	)
}
