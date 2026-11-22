// Package params parses protoc plugin parameters for all three plugins.
//
// The package lives in its own directory so the plugin binaries and their tests
// share exactly one parsing path: a parameter that is accepted in a test but
// rejected (or silently ignored) in production would otherwise be invisible.
package params

import (
	"errors"
	"fmt"
	"strings"

	"github.com/qhai-dev/protoc-gen-grpc-gateway-axios/internal/contract"
)

// Setter is implemented by each plugin's own options struct.
type Setter interface {
	// Set applies one parameter, reporting whether it belongs to the plugin.
	Set(name, value string) (bool, error)
}

// Params is a protogen ParamFunc that routes each parameter to the contract
// layer or to the plugin's own options.
type Params struct {
	contract *contract.Options
	plugin   Setter
	seen     map[string]bool
}

// New binds contract and plugin parameters together.
func New(c *contract.Options, plugin Setter) *Params {
	return &Params{contract: c, plugin: plugin, seen: map[string]bool{}}
}

// Set is passed to protogen.Options as ParamFunc.
func (p *Params) Set(name, value string) error {
	// M mappings are repeatable: the same name may appear many times with
	// different values, so they never trip duplicate detection.
	if len(name) > 1 && name[0] == 'M' {
		_, err := p.contract.Set(name, value)
		return err
	}

	if p.seen[name] {
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

// Apply parses a comma-separated parameter string, as protoc passes it.
func (p *Params) Apply(parameter string) error {
	for _, item := range strings.Split(parameter, ",") {
		if item == "" {
			continue
		}
		name, value, _ := strings.Cut(item, "=")
		if err := p.Set(name, value); err != nil {
			return err
		}
	}
	return nil
}
