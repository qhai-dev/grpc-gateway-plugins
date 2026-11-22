package httprule

import (
	"fmt"
	"strings"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Rule is a parsed google.api.http rule for one method.
type Rule struct {
	Method       string
	Template     *Template
	Body         FieldPath
	ResponseBody FieldPath
}

// Rules returns every google.api.http rule attached to a method: the primary
// rule first, then additional_bindings in declaration order.
//
// Nested additional_bindings are ignored, matching the gateway: the field is not
// recursive in the proto definition and the gateway only reads one level.
func Rules(method protoreflect.MethodDescriptor) ([]*Rule, error) {
	opts := method.Options()
	if opts == nil || !proto.HasExtension(opts, annotations.E_Http) {
		return nil, nil
	}
	rule, ok := proto.GetExtension(opts, annotations.E_Http).(*annotations.HttpRule)
	if !ok || rule == nil {
		return nil, nil
	}

	primary, err := parseRule(method, rule, 0)
	if err != nil {
		return nil, err
	}
	rules := []*Rule{primary}
	for i, extra := range rule.GetAdditionalBindings() {
		if len(extra.GetAdditionalBindings()) > 0 {
			return nil, fmt.Errorf(
				"%s: additional_bindings cannot be nested (binding %d)", method.FullName(), i+2,
			)
		}
		r, err := parseRule(method, extra, i+1)
		if err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, nil
}

func parseRule(method protoreflect.MethodDescriptor, rule *annotations.HttpRule, index int) (*Rule, error) {
	pattern, ok := rule.GetPattern().(*annotations.HttpRule_Custom)
	if ok {
		// custom { kind: "HEAD" path: "/v1/..." } maps to an arbitrary verb.
		kind := strings.ToUpper(strings.TrimSpace(pattern.Custom.GetKind()))
		if kind == "" {
			return nil, fmt.Errorf("%s: custom http rule %d has no kind", method.FullName(), index+1)
		}
		return buildRule(method, kind, pattern.Custom.GetPath(), rule, index)
	}

	method2, path := patternOf(rule)
	if method2 == "" || path == "" {
		return nil, fmt.Errorf("%s: http rule %d has no method/path pattern", method.FullName(), index+1)
	}
	return buildRule(method, method2, path, rule, index)
}

func patternOf(rule *annotations.HttpRule) (string, string) {
	switch p := rule.GetPattern().(type) {
	case *annotations.HttpRule_Get:
		return "GET", p.Get
	case *annotations.HttpRule_Put:
		return "PUT", p.Put
	case *annotations.HttpRule_Post:
		return "POST", p.Post
	case *annotations.HttpRule_Delete:
		return "DELETE", p.Delete
	case *annotations.HttpRule_Patch:
		return "PATCH", p.Patch
	}
	return "", ""
}

func buildRule(method protoreflect.MethodDescriptor, httpMethod, path string, rule *annotations.HttpRule, index int) (*Rule, error) {
	tmpl, err := ParseTemplate(path)
	if err != nil {
		return nil, fmt.Errorf("%s: http rule %d: %w", method.FullName(), index+1, err)
	}

	body, err := ParseFieldPath(rule.GetBody())
	if err != nil {
		return nil, fmt.Errorf("%s: http rule %d: body: %w", method.FullName(), index+1, err)
	}
	responseBody, err := ParseFieldPath(rule.GetResponseBody())
	if err != nil {
		return nil, fmt.Errorf("%s: http rule %d: response_body: %w", method.FullName(), index+1, err)
	}

	// Validate that every bound path names a real field: a typo in the proto
	// annotation should fail generation, not produce a broken URL at runtime.
	for _, v := range tmpl.Variables() {
		if err := validatePath(method.Input(), v.FieldPath); err != nil {
			return nil, fmt.Errorf("%s: http rule %d: path %q: %w", method.FullName(), index+1, path, err)
		}
	}
	if !body.Empty() && !body.IsRoot() {
		if err := validatePath(method.Input(), body); err != nil {
			return nil, fmt.Errorf("%s: http rule %d: body: %w", method.FullName(), index+1, err)
		}
	}
	if !responseBody.Empty() {
		if err := validatePath(method.Output(), responseBody); err != nil {
			return nil, fmt.Errorf("%s: http rule %d: response_body: %w", method.FullName(), index+1, err)
		}
	}

	return &Rule{
		Method:       httpMethod,
		Template:     tmpl,
		Body:         body,
		ResponseBody: responseBody,
	}, nil
}

// validatePath walks a field path through a message descriptor, requiring every
// hop except the last to be a singular message field.
func validatePath(msg protoreflect.MessageDescriptor, path FieldPath) error {
	if msg == nil {
		return fmt.Errorf("no message to resolve %q against", path)
	}
	current := msg
	for i, name := range path {
		if name == "*" {
			return nil
		}
		field := lookupField(current, name)
		if field == nil {
			return fmt.Errorf("field %q not found in %s", name, current.FullName())
		}
		if i == len(path)-1 {
			return nil
		}
		if field.Message() == nil || field.IsList() || field.IsMap() {
			return fmt.Errorf("field %q in %s is not a singular message", name, current.FullName())
		}
		current = field.Message()
	}
	return nil
}

func lookupField(msg protoreflect.MessageDescriptor, name string) protoreflect.FieldDescriptor {
	fields := msg.Fields()
	if f := fields.ByName(protoreflect.Name(name)); f != nil {
		return f
	}
	return fields.ByJSONName(name)
}
