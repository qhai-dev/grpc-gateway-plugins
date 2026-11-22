// Package contract holds the generation contract shared by the three plugins:
// file naming, import paths, field naming and the parameters that must be
// configured identically on all three binaries.
package contract

import (
	"errors"
	"fmt"
	"strings"
)

// ContractVersion is stamped into every generated file header.
const ContractVersion = "v1"

// ErrUnknownParam reports a parameter the contract layer does not own, so the
// calling plugin can try its own parameters before failing.
var ErrUnknownParam = errors.New("unknown parameter")

// Options are the contract parameters. The design document (docs/v1.md §9.1)
// requires every plugin to accept the full set, even when a plugin only uses a
// subset: a suffix changed on one plugin alone would break the imports the other
// two generate.
type Options struct {
	TypesSuffix         string
	ApiSuffix           string
	QuerySuffix         string
	ImportExtension     string
	UseProtoNames       bool
	EmptyFiles          string
	LongType            string
	NestedNameSeparator string
	ModuleMap           map[string]string
}

// DefaultOptions returns the documented defaults.
func DefaultOptions() *Options {
	return &Options{
		TypesSuffix:         "_type.ts",
		ApiSuffix:           "_api.ts",
		QuerySuffix:         "_react_query.ts",
		EmptyFiles:          "emit",
		LongType:            "string",
		NestedNameSeparator: "_",
		ModuleMap:           map[string]string{},
	}
}

// Set applies one protoc plugin parameter. The bool result reports whether the
// parameter belongs to the contract layer.
func (o *Options) Set(name, value string) (bool, error) {
	switch name {
	case "types_suffix":
		o.TypesSuffix = value
	case "api_suffix":
		o.ApiSuffix = value
	case "query_suffix":
		o.QuerySuffix = value
	case "import_extension":
		o.ImportExtension = value
	case "use_proto_names":
		b, err := parseBool(name, value)
		if err != nil {
			return true, err
		}
		o.UseProtoNames = b
	case "empty_files":
		o.EmptyFiles = value
	case "long_type":
		o.LongType = value
	case "nested_name_separator":
		o.NestedNameSeparator = value
	default:
		// M<proto>=<module> is a repeatable mapping, same shape as protoc-gen-go.
		if len(name) > 1 && name[0] == 'M' {
			protoPath := name[1:]
			if value == "" {
				return true, fmt.Errorf("M%s: module path must not be empty", protoPath)
			}
			o.ModuleMap[protoPath] = value
			return true, nil
		}
		return false, ErrUnknownParam
	}
	return true, nil
}

// Validate rejects parameter combinations that would silently break imports.
func (o *Options) Validate() error {
	for _, s := range []struct{ name, value string }{
		{"types_suffix", o.TypesSuffix},
		{"api_suffix", o.ApiSuffix},
		{"query_suffix", o.QuerySuffix},
	} {
		if s.value == "" {
			return fmt.Errorf("%s must not be empty", s.name)
		}
		if strings.ContainsAny(s.value, `/\`) {
			return fmt.Errorf("%s %q must not contain a path separator", s.name, s.value)
		}
	}
	if o.ImportExtension != "" && !strings.HasPrefix(o.ImportExtension, ".") {
		return fmt.Errorf("import_extension %q must start with %q", o.ImportExtension, ".")
	}
	switch o.EmptyFiles {
	case "emit", "skip":
	default:
		return fmt.Errorf("empty_files must be emit or skip, got %q", o.EmptyFiles)
	}
	switch o.LongType {
	case "string", "number":
	default:
		return fmt.Errorf("long_type must be string or number, got %q", o.LongType)
	}
	if o.NestedNameSeparator == "" {
		return errors.New("nested_name_separator must not be empty")
	}
	return nil
}

func parseBool(name, value string) (bool, error) {
	switch strings.ToLower(value) {
	case "true", "1", "yes":
		return true, nil
	case "false", "0", "no":
		return false, nil
	}
	return false, fmt.Errorf("%s: expected true or false, got %q", name, value)
}
