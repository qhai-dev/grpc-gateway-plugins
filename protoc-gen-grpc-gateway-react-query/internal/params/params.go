// Package params parses protoc plugin parameters for this plugin.
//
// The package lives in its own directory so the plugin binary and its tests share
// exactly one parsing path: a parameter that is accepted in a test but rejected (or
// silently ignored) in production would otherwise be invisible.
package params

import (
	"errors"
	"fmt"
	"strings"

	"github.com/qhai-dev/protoc-gen-grpc-gateway-react-query/internal/contract"
)

// Setter is implemented by each plugin's own options struct.
type Setter interface {
	// Set applies one parameter, reporting whether it belongs to the plugin.
	Set(name, value string) (bool, error)
}

// Params is a protogen ParamFunc that routes each parameter to the contract layer
// or to the plugin's own options.
type Params struct {
	contract *contract.Options
	plugin   Setter
	seen     map[string]bool
	// repeatable are parameter names that may legally appear more than once,
	// because their values form a list.
	repeatable map[string]bool
}

// New binds contract and plugin parameters together.
//
// repeatable lists the plugin's list-valued parameters. protoc splits the plugin
// parameter string on commas, so a list such as
//
//	queries=user.v1.UserService.Search,user.v1.UserService.Validate
//
// arrives as `queries=...Search` followed by the bare token
// `user.v1.UserService.Validate`. A bare token cannot be matched to a name, so it is
// appended to the most recent list-valued parameter.
func New(c *contract.Options, plugin Setter, repeatable ...string) *Params {
	p := &Params{
		contract:   c,
		plugin:     plugin,
		seen:       map[string]bool{},
		repeatable: map[string]bool{},
	}
	for _, r := range repeatable {
		p.repeatable[r] = true
	}
	return p
}

// Set is passed to protogen.Options as ParamFunc.
func (p *Params) Set(name, value string) error {
	// M mappings are repeatable: the same name may appear many times with different
	// values, so they never trip duplicate detection.
	if len(name) > 1 && name[0] == 'M' {
		_, err := p.contract.Set(name, value)
		return err
	}

	if p.seen[name] && !p.repeatable[name] {
		// protogen concatenates flags from buf.gen.yaml, so a repeated
		// non-repeatable parameter is a configuration mistake worth reporting.
		return fmt.Errorf("parameter %q given more than once", name)
	}

	handled, err := p.contract.Set(name, value)
	if err != nil && !errors.Is(err, contract.ErrUnknownParam) {
		return err
	}
	if !handled {
		handled, err = p.plugin.Set(name, value)
		if err != nil {
			return err
		}
	}
	if !handled {
		return fmt.Errorf("unknown parameter %q", name)
	}
	p.seen[name] = true
	return nil
}

// Apply parses a parameter string the way protogen does: comma-separated
// `name=value` pairs, where a token without `=` continues a list.
func (p *Params) Apply(parameter string) error {
	var lastList string
	for _, item := range strings.Split(parameter, ",") {
		if item == "" {
			continue
		}
		name, value, hasValue := strings.Cut(item, "=")
		if !hasValue && lastList != "" {
			// A continuation of the previous list-valued parameter.
			if err := p.Set(lastList, name); err != nil {
				return err
			}
			continue
		}
		if err := p.Set(name, value); err != nil {
			return err
		}
		if p.repeatable[name] {
			lastList = name
		}
	}
	return nil
}
