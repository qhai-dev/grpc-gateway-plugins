# Shared contract, in one place.

The three generators must agree on four things or the generated SDK will not
compile. These are the rules every plugin implements, defined in `docs/v1.md`
(v1.1) and verified by the tests in each project.

| Rule | Value |
| --- | --- |
| Types file | `<base>_type.ts` |
| Axios file | `<base>_api.ts` |
| React Query file | `<base>_react_query.ts` |
| `<base>` | proto path with `.proto` removed, e.g. `user/v1/user` |
| File naming is per `file_to_generate` | every proto listed produces all three files |
| Empty files | still emitted with a header, so Bazel can declare outputs ahead of time |

## Field names

`json_name` (lowerCamelCase) by default, to match protojson. `use_proto_names=true`
switches to proto field names, and must be set identically on all three plugins
because the gateway's response field names depend on its own marshaler setting.

## Type mapping

Defined once in `internal/typeref`, used by all three plugins, so the Axios and
React Query files always name types exactly as the types file declares them.

| Proto | TypeScript |
| --- | --- |
| `int32`, `uint32`, `sint32`, `fixed32`, `sfixed32`, `float`, `double` | `number` |
| `int64`, `uint64`, `sint64`, `fixed64`, `sfixed64` | `string` (protojson; `long_type=number` opts out) |
| `bool` | `boolean` |
| `string`, `bytes` | `string` (`bytes` is base64, as in protojson) |
| `enum` | string literal union |
| `map<K,V>` | `Record<string, V>` — JSON keys are always strings |
| `repeated T` | `T[]` |
| `google.protobuf.Timestamp` | `string` (RFC3339) |
| `google.protobuf.Duration` | `string` (e.g. `"3.5s"`) |
| `google.protobuf.FieldMask` | `string` (comma-separated paths) |
| `google.protobuf.Struct` | `Record<string, JsonValue>` |
| `google.protobuf.Value` | `JsonValue` |
| `google.protobuf.ListValue` | `JsonValue[]` |
| `google.protobuf.Empty` | `Record<string, never>` |
| `google.protobuf.Any` | `{ "@type": string; [key: string]: unknown }` |
| `google.protobuf.*Value` | `T \| null` |
| `google.protobuf.NullValue` | `null` |

Well-known types are inlined; `google/protobuf/*.proto` is never generated or
imported, so Buf's `include_imports` does not produce `timestamp_type.ts`.

## Imports

- Same file: the bare type name.
- Another generated proto: `import type * as <alias> from "<relative>"`, where the
  alias folds the whole proto path into one identifier
  (`user/v1/address.proto` → `user_v1_address_type`). A namespace import cannot
  collide when two files export the same type name.
- External modules: `M<proto>=<module>`, same syntax as protoc-gen-go. Mapped
  specifiers never get the relative import extension.
- `import_extension=.js` replaces the `.ts` extension on relative imports only:
  `./user_type.ts` becomes `./user_type.js`, and `user.types.ts` becomes
  `user.types.js`. Package specifiers are untouched.

## HTTP semantics

These follow grpc-gateway's own runtime, not a guess:

- **Path**: a single-segment variable is `encodeURIComponent(String(value))`. A
  multi-segment variable (`{name=shippers/*}`, `{path=**}`) is encoded per
  segment, because the gateway matches its slashes as separators; encoding them
  would produce `shippers%2F123` and miss the route.
- **Presence**: only `== null` marks a value missing. `0`, `false` and `""` are
  legitimate and are sent.
- **Query**: built explicitly into a `URLSearchParams`, never `params: request`.
  Repeated fields repeat the parameter name; maps use `name[key]=value`; nested
  messages use dotted names. Path-bound fields and body-covered fields are
  excluded, together with their descendants.
- **`body`**: `""` sends nothing, `"*"` sends the request, `"a.b"` sends
  `request.a?.b`.
- **`response_body`**: the gateway marshals only that field as the whole HTTP
  body, so the response type is the field's type and there is nothing to unwrap.

## Parameters

Every plugin accepts the whole shared set, even though it uses only part of it.

| Parameter | Default | Meaning |
| --- | --- | --- |
| `types_suffix` | `_type.ts` | types file suffix |
| `api_suffix` | `_api.ts` | Axios file suffix |
| `query_suffix` | `_react_query.ts` | React Query file suffix |
| `import_extension` | (empty) | extension appended to relative imports |
| `use_proto_names` | `false` | use proto field names |
| `empty_files` | `emit` | `emit` or `skip` |
| `M<proto>=<module>` | none | external type module mapping, repeatable |
| `long_type` | `string` | `string` or `number` |
| `nested_name_separator` | `_` | `Outer_Inner` separator |

Plugin 1 also takes `enum_style` (`union`/`const`/`enum`), `oneof_style`
(`union`/`flat`) and `emit_comments`.

Plugin 2 also takes `client_style` (`class`/`functions`), `additional_bindings`
(`ignore`/`methods`), `strict_http` and `streaming` (`ignore`/`error`).

Plugin 3 also takes `hook_prefix` (`none`/`service`), `request_optional`,
`queries` (comma-separated fully qualified methods to treat as queries) and
`additional_bindings`, which must match plugin 2.

## Deliberate departures from `docs/v1.md`

Two documented behaviours do not match grpc-gateway, and the implementation
follows the gateway:

1. **`response_body`** (§4.10) is described as unwrapping `response.data.user`.
   The gateway's generated `XXX_ResponseBody()` returns the field and
   `ForwardResponseMessage` marshals that return value as the entire body, so the
   HTTP body is already the field value. The generated client therefore uses the
   field's type as the axios generic and returns `response.data` unchanged.
2. **`long_type=bigint`** is excluded, as the document requires: `JSON.stringify`
   throws on a `BigInt`, which would break every React Query key.

One documented behaviour is implemented differently on purpose:

3. **`oneof_style=union`** emits the cartesian product of a message's oneofs, and
   falls back to flat optional members (with a comment) past 64 arms. Multiple
   oneofs in one message would otherwise need a product anyway, and a message with
   three oneofs of three members each is 27 arms of `?: never`.
