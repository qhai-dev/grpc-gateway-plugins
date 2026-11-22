package contract

import (
	"strings"
	"unicode"
)

// FieldName returns the property name emitted for a proto field.
//
// protojson accepts both names on input, but the response side depends on the
// gateway marshaler, so this option is shared by all three plugins (§2.3).
func (o *Options) FieldName(jsonName, protoName string) string {
	if o.UseProtoNames {
		return protoName
	}
	return jsonName
}

// TypeName builds the TypeScript name for a message or enum, folding nesting
// into the name protoc-gen-go style: Outer_Inner.
func (o *Options) TypeName(parts []string) string {
	return strings.Join(parts, o.NestedNameSeparator)
}

// LowerCamel converts a proto identifier to lowerCamelCase for a TS member:
// GetUser -> getUser, ListURLs -> listURLs, HTTPClient -> httpClient,
// URLs -> urls.
//
// Only the leading run of capitals is folded. When that run is an initialism
// followed by a real word, its final capital starts that word and is kept; when
// it is followed by a single trailing letter (a plural `s`), the whole run is the
// initialism and folds entirely. That distinction is what separates `HTTPClient`
// from `URLs` without an initialism dictionary.
func LowerCamel(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)

	end := 0
	for end < len(runes) && unicode.IsUpper(runes[end]) {
		end++
	}
	if end == 0 {
		return s
	}

	// keepLast is the number of trailing capitals to preserve.
	keepLast := 0
	if end < len(runes) && end >= 3 {
		// Length of the lowercase word that follows the capital run.
		rest := 0
		for i := end; i < len(runes) && unicode.IsLower(runes[i]); i++ {
			rest++
		}
		if rest > 1 {
			keepLast = 1
		}
	}

	for i := 0; i < end-keepLast; i++ {
		runes[i] = unicode.ToLower(runes[i])
	}
	return string(runes)
}

// UpperCamel ensures a leading capital, for hook names: getUser -> UseGetUser.
func UpperCamel(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// ClientName is the exported class name for a service: UserService -> UserServiceClient.
func ClientName(serviceName string) string {
	return serviceName + "Client"
}

// ClientVarName is the TS variable name for a service client instance.
func ClientVarName(serviceName string) string {
	return LowerCamel(serviceName)
}

// KeysName is the query key factory name: UserService -> userServiceKeys.
func KeysName(serviceName string) string {
	return LowerCamel(serviceName) + "Keys"
}

// HookName is the hook for one method (or one extra binding).
func HookName(methodClientName string) string {
	return "use" + UpperCamel(methodClientName)
}
