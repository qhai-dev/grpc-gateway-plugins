# protoc-gen-grpc-gateway-typescript

Generates TypeScript types from protobuf messages and enums.

Emits `<base>_pb.ts` for each proto in `file_to_generate`, where `<base>` is the
proto path without `.proto`: `user/v1/user.proto` → `user/v1/user_pb.ts`.

This plugin implements the shared contract in [`../CONTRACT.md`](../CONTRACT.md).
Well-known types are inlined as TypeScript primitives, and `google/protobuf/*`
is never generated, even when Buf runs with `include_imports`.

## Build

```sh
go build -o bin/protoc-gen-grpc-gateway-typescript ./cmd/protoc-gen-grpc-gateway-typescript
```

## Use with protoc

```sh
protoc \
  --plugin=protoc-gen-ts=bin/protoc-gen-grpc-gateway-typescript \
  -I proto -I path/to/google/api \
  --ts_out=import_extension=.js:gen \
  user/v1/user.proto
```

Protos do not need `option go_package`; the plugin synthesizes an internal
mapping, so it works in projects that have no Go code.

## Example

```proto
message GetUserRequest {
  string id = 1;
}
```

```ts
export type GetUserRequest = {
  id?: string;
};
```

Every singular field is optional: in the JSON form this SDK speaks, an unset
scalar and a zero scalar are both absent. That is what lets a caller build a
partial request, and it makes `0`, `false` and `""` distinguishable from "not
set".

## Options

| Option | Default | Meaning |
| --- | --- | --- |
| `types_suffix` | `_pb.ts` | output suffix |
| `import_extension` | (empty) | extension appended to relative imports |
| `use_proto_names` | `false` | emit proto field names instead of `json_name` |
| `long_type` | `string` | `string` or `number` for 64-bit integers |
| `nested_name_separator` | `_` | `Outer_Inner` separator |
| `enum_style` | `union` | `union`, `const` or `enum` |
| `oneof_style` | `union` | `union` or `flat` |
| `emit_comments` | `true` | render proto leading comments as JSDoc |
| `M<proto>=<module>` | none | map an external proto to a module specifier |

Plus the shared contract parameters listed in `../CONTRACT.md`.

### Oneof

The default form makes exclusivity a compile-time property:

```ts
export type Payment =
  | { card: Card; bank?: never; cash?: never }
  | { bank: Bank; card?: never; cash?: never }
  | { cash: Cash; card?: never; bank?: never };
```

`request.payment = { card, bank }` is a type error. A message with several
oneofs needs the cartesian product; past 64 arms the generator falls back to flat
optional members and says so in a comment, because 27 arms of `?: never` are
worse than a slightly weaker type.

`proto3 optional` creates a synthetic oneof that is *not* a business oneof, so an
`optional string nickname = 1` stays `nickname?: string` and never becomes a
union.

## Tests

```sh
go test ./...
```

Golden files live in `testdata/golden` and are regenerated with:

```sh
go test ./internal/gen -update
```

`testdata/fixture.pb` is a checked-in descriptor set, so the tests do not need
`protoc` installed and cannot drift from what protoc actually sends.
