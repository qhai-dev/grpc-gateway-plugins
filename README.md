# Protobuf → TypeScript SDK generator plugins

Three independent protoc plugins that generate a typed TypeScript client from
protobuf definitions and their `google.api.http` annotations:

| Project | Generates | Artifact |
| --- | --- | --- |
| [`protoc-gen-grpc-gateway-typescript`](protoc-gen-grpc-gateway-typescript/) | Message and enum types | `<base>_pb.ts` |
| [`protoc-gen-grpc-gateway-axios`](protoc-gen-grpc-gateway-axios/) | Axios HTTP client | `<base>_api.ts` |
| [`protoc-gen-grpc-gateway-react-query`](protoc-gen-grpc-gateway-react-query/) | TanStack Query v5 hooks | `<base>_react_query.ts` |

Each project is a self-contained Go module with its own command, internal
packages and tests. Nothing is shared through a common module: the contract lives
in [`CONTRACT.md`](CONTRACT.md), and each project implements it independently, so
any one project can be copied out and built alone.

## Why the three plugins do not depend on each other

They all read the same proto descriptors and derive each other's file paths from
the naming contract. No plugin reads another's output, so Buf and Bazel can run
them in any order or in parallel. What they must agree on is exactly the contract:
file suffixes, field names and import paths.

## Usage

```sh
protoc \
  --plugin=protoc-gen-ts=protoc-gen-grpc-gateway-typescript \
  --plugin=protoc-gen-api=protoc-gen-grpc-gateway-axios \
  --plugin=protoc-gen-rq=protoc-gen-grpc-gateway-react-query \
  -I proto -I path/to/google/api \
  --ts_out=import_extension=.js:gen \
  --api_out=import_extension=.js:gen \
  --rq_out=import_extension=.js:gen \
  user/v1/user.proto
```

For Buf, see [`buf.gen.yaml`](buf.gen.yaml); the shared options are repeated in all
three entries on purpose, because a suffix changed on one plugin alone breaks the
imports the other two emit.

## Buf

The example is a Buf project: it has its own `buf.yaml`, `buf.gen.yaml` and `buf.lock`,
and `buf generate` is the only way it produces its SDK. There is no `protoc` command to
run and no plugin binary to build first, because `buf.gen.yaml` invokes the plugins with
`go run` from the parent repository.

```sh
cd example
npm install      # also installs a pinned Buf, no system install needed
npm run generate # buf generate -> src/gen
npm run check    # generate, typecheck, test
```

See [`example/README.md`](example/README.md) for the detail, including why Go is still
required and what Buf removes compared with a bare `protoc` invocation.

The repository root keeps a second Buf module over `example/proto`, so the plugins can be
exercised without the example's Node toolchain:

```sh
make buf-build     # build the three binaries into .buf/bin
make buf-generate  # buf generate into .buf/gen
make buf-check     # assert the root module and example/ agree byte for byte
make buf-lint      # buf lint both modules
```

`make buf-check` is the point of having two entry points: if the root module and
`example/buf.yaml` ever drift apart, their outputs diverge and the check fails. They
currently produce byte-identical files.

The generated code needs `axios` and, for the hooks, `@tanstack/react-query` v5
and React 18 or later. It never creates an axios instance; the application injects
one.

```ts
import axios from "axios";
import { UserServiceClient } from "./gen/user/v1/user_api.js";
import { useGetUser } from "./gen/user/v1/user_react_query.js";

const http = axios.create({ baseURL: "/api" });
const userClient = new UserServiceClient(http);

const { data } = useGetUser(userClient, { id: "1001" });
```

## Verification

```sh
make test          # Go unit tests, per project
make typecheck     # generate all three, compile under strict TypeScript
make runtime       # exercise the generated client against a recording adapter
make conformance   # feed its query strings to grpc-gateway's own parser
make buf-check     # buf generate must match the protoc output byte for byte
make all
```

`make typecheck` is the check that matters most: it compiles all three generated
files together under `strict`, `exactOptionalPropertyTypes`,
`noUncheckedIndexedAccess`, `verbatimModuleSyntax` and `module: NodeNext`, which
is where a contract mismatch between the plugins surfaces.

Two projects carry extra checks that need their dependencies:

- `protoc-gen-grpc-gateway-react-query/acceptance/runtime.ts` asserts the URL,
  method and body the generated client produces.
- `protoc-gen-grpc-gateway-axios/conformance/` feeds those query strings to
  `runtime.PopulateQueryParameters` and asserts the decoded proto, which is the
  check that catches gateway incompatibility before it becomes a runtime 400.

The [`example/`](example/) project is the end-to-end counterpart: a bookstore API
with a React app, generated code checked in, and a mock gateway that decodes
requests the way grpc-gateway does. `cd example && npm install && npm run dev:full`.

## Go workspace

`go.work` ties the four modules together so a single command can build or test
every one against the local sources:

```sh
go build ./protoc-gen-grpc-gateway-typescript/...   # and the other three
go test  ./protoc-gen-grpc-gateway-axios/...
```

The workspace is a convenience, not a requirement: each module still builds on its
own, and `GOWORK=off` proves it. `go.work` also carries one `replace` for
`google.golang.org/genproto`, because in workspace mode the build list is the union
of every module's requirements, which surfaces a pre-split monolith version pulled
in by grpc's opencensus dependency. The comment in `go.work` explains it.

## Layout

```
.
├── CONTRACT.md                     # the rules all three plugins implement
├── buf.yaml                        # Buf module over example/proto
├── buf.gen.yaml                    # all three plugins in one Buf run
├── buf.lock                        # pins googleapis from the BSR
├── go.work                         # the four modules, for local development
├── Makefile                        # cross-project verification
├── _protodeps/google/api/          # annotations.proto and http.proto for protoc
├── docs/                           # design documents
├── example/                        # bookstore app: proto → SDK → React UI
├── protoc-gen-grpc-gateway-typescript/
├── protoc-gen-grpc-gateway-axios/
└── protoc-gen-grpc-gateway-react-query/
```

Each project follows the same internal shape:

```
internal/
├── contract/     # file naming, import paths, field names, parameters
├── httprule/     # google.api.http templates, path/query/body resolution
├── ir/           # shared intermediate representation
├── typeref/      # proto type → TypeScript type
├── ts/           # deterministic TypeScript writer
├── params/       # plugin parameter parsing
├── request/      # CodeGeneratorRequest preparation
├── plugin/       # protoc plugin protocol
└── gen/          # this project's generator
```

## Implementation notes

Both reference implementations reviewed in `docs/v1-review.md` are avoided where
they disagree with grpc-gateway:

- Query parameters are built explicitly in a `URLSearchParams`, not passed as
  axios `params`, whose serialization the gateway does not parse.
- `0`, `false` and `""` are sent; only `null` and `undefined` are omitted.
- Maps use `name[key]=value`; repeated fields repeat the parameter name.
- A multi-segment path variable is encoded per segment, so `shippers/123` keeps
  its slash and still matches the route.
- Path-bound and body-covered fields are excluded from the query, with their
  descendants.
- No `@ts-nocheck`: the generated code is required to pass a strict compile.

Two documented behaviours are implemented as the gateway actually behaves, which
differs from `docs/v1.md`; see the "Deliberate departures" section of
[`CONTRACT.md`](CONTRACT.md).
