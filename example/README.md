# Bookstore example

A complete, runnable project built with the three generators in this repository. It is
the answer to "what does using this toolchain actually look like": one proto module,
three generated files per proto, and a React app that never writes a URL, a request
type or a cache key by hand.

**Buf drives generation.** `buf generate` is the only way this project produces its SDK:
there is no `protoc` command to run and no plugin binary to build first.

```
example/
├── buf.yaml                             # Buf module: proto/, googleapis from the BSR
├── buf.gen.yaml                         # the three plugins, invoked with `go run`
├── buf.lock                             # pins buf.build/googleapis/googleapis
├── proto/
│   ├── bookstore/v1/bookstore.proto     # the service: every annotation exercised
│   └── common/v1/money.proto            # a cross-file type, no service
├── src/
│   ├── gen/                             # generated, checked in
│   │   ├── bookstore/v1/bookstore_type.ts
│   │   ├── bookstore/v1/bookstore_api.ts
│   │   ├── bookstore/v1/bookstore_react_query.ts
│   │   └── common/v1/…                  # one types file, two header-only files
│   ├── lib/http.ts                      # the axios instance (application-owned)
│   ├── lib/format.ts                    # money, timestamp, oneof narrowing
│   ├── api/books.ts                     # the one place a client is constructed
│   ├── hooks/books.ts                   # generated hooks + invalidation policy
│   ├── components/                      # UI: list, detail, create form
│   ├── contract.test.ts                 # wire-format assertions
│   └── main.tsx
├── mock-server/server.mjs               # stands in for grpc-gateway
└── scripts/
    ├── buf.mjs                          # locates the pinned Buf, forwards argv
    ├── generate.mjs                     # npm run generate
    └── dev.mjs                          # npm run dev:full
```

## Prerequisites

Node 20+, Go 1.23+, and that is all: Buf comes from `devDependencies`, so `npm install`
provides a pinned version and no system install is needed.

This is still a Go requirement even though the output is TypeScript. `buf.gen.yaml`
invokes the plugins with `go run`, which compiles them from the parent repository on
demand. That removes a build step that used to be a prerequisite, and more importantly
removes the failure mode where a stale binary silently generated output from code that
no longer matched the working tree.

## Run it

```sh
npm install
npm run dev:full
```

`dev:full` starts the mock gateway on `:8080` and Vite on `:5173`. Open
<http://localhost:5173>.

`npm run dev` alone runs only Vite, which is useful with a real grpc-gateway backend on
`:8080`. The Vite proxy forwards `/api/*` to it, so the generated URLs (`/v1/...`) and
the axios `baseURL` (`/api`) stay identical between mock and real.

## Generate

```sh
npm run generate
```

That is `buf generate` with two things wrapped around it: it locates the pinned Buf
binary, and it clears `src/gen` first so a deleted or renamed proto cannot leave an
orphaned file behind that still type-checks.

| Command | Does |
| --- | --- |
| `npm run generate` | `buf generate` → `src/gen` |
| `npm run lint:proto` | `buf lint` |
| `npm run deps:update` | `buf dep update` → rewrites `buf.lock` |
| `npm run check` | generate, then typecheck, then test |

`npm run generate` deliberately does **not** run `buf dep update`. A code change should
not silently move a dependency version; `deps:update` is the explicit command for that.

## Why Buf rather than protoc

The plugins consume a `CodeGeneratorRequest` and do not care how it was produced, so
either works. Buf is what this example uses because it removes three things a bare
`protoc` invocation needs:

- **`-I` paths.** `protoc -I proto -I ../_protodeps -I "$(dirname $(which protoc))/include"`
  is easy to get subtly wrong, and the failure is an unhelpful "file does not exist".
- **A vendored copy of `google/api`.** `buf.yaml` declares
  `buf.build/googleapis/googleapis` as a dependency, and `buf.lock` pins the exact
  commit, so two machines cannot compile against different definitions of the same
  annotation.
- **A build step.** `local: [go, run, -C, ../protoc-gen-..., ./cmd/...]` compiles the
  plugin from source as part of generation.

The trade-off is that generation needs Go on `PATH` and the first run compiles each
plugin. A warm build cache makes later runs fast.

## What the proto exercises

`bookstore.proto` is not a toy. Every annotation the generators support appears, because
the example is also the acceptance test:

| Annotation | Where | What it shows |
| --- | --- | --- |
| `{id}` path variable | `GetBook`, `DeleteBook` | one encoded path token |
| nested path variable | `UpdateBook` (`{book.id}`) | `request.book?.id` with optional chaining |
| `:verb` suffix | `ArchiveBook` (`/v1/books/{id}:archive`) | the verb stays outside the variable |
| `response_body` | `GetBook` (`"book"`) | the promise resolves to `Book`, not a wrapper |
| `body: "*"` | `CreateBook`, `SearchBooks` | whole request is the body, no query string |
| field `body` | `UpdateBook` (`"book"`) | body is `request.book`; `update_mask` stays a query parameter |
| `additional_bindings` | `ImportBooks` | ignored by default; `methods` adds `importBooksBinding2` |
| query: scalar | `ListBooks` (`page`, `pageSize`) | `page=1&pageSize=10` |
| query: bool | `includeOutOfPrint` | `false` is still sent |
| query: repeated | `authorIds`, `formats` | `authorIds=a&authorIds=b` |
| query: map | `labels` | `labels[lang]=en` |
| query: nested message | `filter` | `filter.author=`, `filter.inStock=` |
| query: enum | `formats`, `filter.format` | sent by name |
| query: int64 | `minPriceUnits` | decimal string, no precision loss |
| query: Timestamp | `filter.publishedAfter` | RFC3339 string |

The type mapping surfaces in the UI as well:

- `int64` (`Money.units`, `totalCount`) is a `string`, formatted with `BigInt`.
- `bytes` (`ImportBooksRequest.csv`) is a base64 `string`.
- `Timestamp` is an RFC3339 `string`, formatted in `lib/format.ts`.
- `optional string isbn13` stays a plain optional property, not a union.
- The `availability` oneof is a mutually exclusive union, narrowed with `in` in
  `describeAvailability`. It also has an arm for "no member set", which is what the third
  seeded book exercises.

## The division of responsibility

The example is arranged so the boundary is visible:

| Concern | Owner | File |
| --- | --- | --- |
| Message and enum types | generated | `src/gen/**/*_type.ts` |
| URL, method, query encoding, body | generated | `src/gen/**/*_api.ts` |
| Query keys, `useQuery` / `useMutation` | generated | `src/gen/**/*_react_query.ts` |
| axios instance, auth, error mapping | application | `src/lib/http.ts` |
| which client, which host | application | `src/api/books.ts` |
| what a write invalidates | application | `src/hooks/books.ts` |
| presentation | application | `src/components/**` |

The generators intentionally do not do the last four. A generated file that knew where
the axios instance lives, or which queries a create invalidates, would have to be edited
by hand after every regeneration.

## The mock server

`mock-server/server.mjs` implements only the routes `bookstore.proto` declares, and it
decodes the request the way `runtime.PopulateQueryParameters` does: dotted names for
nested messages, `name[key]=value` for maps, repeated parameters, enum names, and int64
as a string. That is deliberate: the mock fails loudly if the generated client's encoding
changes, which is the same guarantee the Go conformance check in
`../protoc-gen-grpc-gateway-axios/conformance/` provides.

One behavior worth knowing: it answers `500` when no route matches, not `404`, so a URL
mistake is loud rather than indistinguishable from a missing resource. A missing *book*
is still a `404`.

## Tests

```sh
npm run test         # contract tests
npm run typecheck    # strict tsc over app and generated code
npm run build        # production bundle
npm run check        # all of the above, after regenerating
```

`src/contract.test.ts` records the requests the generated client emits with an axios
adapter, and asserts:

- a single-segment variable is one encoded token, and a missing one throws before
  sending;
- `0`, `false` and `""` are sent, not dropped;
- maps use the bracket form, nested messages dotted names, enums their names, int64
  decimal strings;
- path-bound and body-covered fields are absent from the query;
- `body: "*"` sends the request and produces no query string;
- `response_body` resolves to the field value;
- `url` and `method` cannot be overridden through config;
- an aborted signal reaches axios;
- query keys contain the whole request and share a service-wide prefix.

The suite passes with `strict`, `exactOptionalPropertyTypes`,
`noUncheckedIndexedAccess`, `noUnusedLocals` and `noUnusedParameters`.

## Wiring it to a real backend

Replace the mock with a grpc-gateway server and keep everything else:

1. Point the Vite proxy (`vite.config.ts`) at the gateway, or set `baseURL` in
   `src/lib/http.ts` to the gateway's origin.
2. Delete `mock-server/` and the `mock` / `dev:full` scripts.
3. If the proto lives in the backend repository, move `buf.yaml` and `buf.gen.yaml`
   there and generate into the frontend as a build step. The `local:` paths in
   `buf.gen.yaml` would then point at installed plugin binaries, or use `remote:` with
   plugins published to the [Buf Schema Registry](https://buf.build).

The generated code itself needs no changes. That is the point of injecting the axios
instance instead of creating one.
