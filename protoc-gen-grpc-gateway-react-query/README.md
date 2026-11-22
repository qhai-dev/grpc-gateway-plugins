# protoc-gen-grpc-gateway-react-query

Generates TanStack Query v5 hooks from protobuf services.

Emits `<base>_react_query.ts` for each proto in `file_to_generate`. This plugin
implements the shared contract in [`../CONTRACT.md`](../CONTRACT.md).

Hooks take the generated Axios client as a parameter, so the module never reaches
for a global instance and the same SDK can point at several hosts or
configurations:

```ts
import axios from "axios";
import { UserServiceClient } from "./gen/user/v1/user_api.js";
import { useGetUser } from "./gen/user/v1/user_react_query.js";

const client = new UserServiceClient(axios.create({ baseURL: "/api" }));

function UserPage({ id }: { id: string }) {
  const { data, isLoading } = useGetUser(client, { id });
  if (isLoading) return <div>Loading...</div>;
  return <div>{data?.name}</div>;
}
```

## Build

```sh
go build -o bin/protoc-gen-grpc-gateway-react-query ./cmd/protoc-gen-grpc-gateway-react-query
```

## Use with protoc

```sh
protoc \
  --plugin=protoc-gen-rq=bin/protoc-gen-grpc-gateway-react-query \
  -I proto -I path/to/google/api \
  --rq_out=import_extension=.js:gen \
  user/v1/user.proto
```

## Query keys

Each service exports a key factory. The key contains the **whole request**, not
just an id: keying a list query on one field would serve the wrong cache entry for
a different filter.

```ts
export const userServiceKeys = {
  all: ["UserService"] as const,

  getUser: (request: GetUserRequest | undefined) =>
    [...userServiceKeys.all, "getUser", request] as const,

  listUsers: (request: ListUsersRequest | undefined) =>
    [...userServiceKeys.all, "listUsers", request] as const,
} as const;
```

The factory is exported so an application can invalidate by prefix:

```ts
queryClient.invalidateQueries({ queryKey: userServiceKeys.all });
```

React Query v5 hashes object keys structurally, and `undefined` properties are
dropped by `JSON.stringify`, which matches "field not set". `bigint` is
deliberately unsupported, because `JSON.stringify` throws on it and the key would
be unusable.

## Query hooks

```ts
export function useGetUser(
  client: UserServiceClient,
  request: GetUserRequest | undefined,
  options?: Omit<UseQueryOptions<User, Error, User>, "queryKey" | "queryFn">,
) {
  const { enabled, ...rest } = options ?? {};

  return useQuery({
    ...rest,
    queryKey: userServiceKeys.getUser(request),
    queryFn: ({ signal }) => {
      if (request === undefined) {
        return Promise.reject(new Error("request is required"));
      }
      return client.getUser(request, { signal });
    },
    enabled: enabled ?? request !== undefined,
  });
}
```

- `request` may be `undefined`, so `useGetUser(client, id ? { id } : undefined)`
  does not fire until the id exists. `request_optional=false` makes it required.
- `signal` is forwarded to axios, so changing parameters or unmounting cancels the
  in-flight request.
- `queryKey` and `queryFn` are omitted from `options`: they belong to the generated
  code. Everything else passes through, including `enabled`, which overrides the
  default.

## Mutation hooks

```ts
export function useCreateUser(
  client: UserServiceClient,
  options?: UseMutationOptions<CreateUserResponse, Error, CreateUserRequest>,
) {
  return useMutation({
    ...options,
    mutationFn: (request: CreateUserRequest) => client.createUser(request),
  });
}
```

`...options` comes first so the caller cannot replace `mutationFn`.

## Query vs mutation

By HTTP method: `GET` is a query, everything else is a mutation. An endpoint that
uses `POST` for a read is overridden explicitly:

```
queries=user.v1.UserService.Search,user.v1.UserService.Validate
```

## Options

| Option | Default | Meaning |
| --- | --- | --- |
| `hook_prefix` | `none` | `none`, or `service` to prefix hooks (`useUserServiceGetUser`) |
| `request_optional` | `true` | whether `request` may be `undefined` |
| `queries` | none | comma-separated fully qualified methods to force to queries |
| `additional_bindings` | `ignore` | must match the Axios plugin |

Plus the shared contract parameters listed in `../CONTRACT.md`.

## Name collisions

Two services in one proto file may declare the same method name. The hooks and
keys live in the same module, so names are made unique deterministically, and
`hook_prefix=service` is the explicit way to scope every hook to its service. The
Axios plugin applies the same suffixing in `client_style=functions` mode, so both
generated APIs agree.

## Not generated

No automatic cache invalidation, no optimistic updates, no infinite or paginated
queries. Those are business semantics rather than proto HTTP contracts; the key
factory is exported so an application can express them at the call site.

## Tests

```sh
go test ./...
```

Golden files live in `testdata/golden`; regenerate with
`go test ./internal/gen -update`.

`acceptance/` holds the runtime harness. It runs the generated client against a
recording axios adapter and asserts the URL, method and body for path encoding,
zero values, query encoding, body/query exclusion and `response_body`.
