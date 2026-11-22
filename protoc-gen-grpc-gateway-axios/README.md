# protoc-gen-grpc-gateway-axios

Generates an Axios HTTP client from protobuf services and their
`google.api.http` annotations.

Emits `<base>_api.ts` for each proto in `file_to_generate`. This plugin
implements the shared contract in [`../CONTRACT.md`](../CONTRACT.md).

The generated code never creates an axios instance. The caller injects one, which
keeps base URL, interceptors, auth, retries and environment configuration under
the application's control:

```ts
import axios from "axios";
import { UserServiceClient } from "./gen/user/v1/user_api.js";

const http = axios.create({ baseURL: "/api", timeout: 10000 });

export const userClient = new UserServiceClient(http);
```

## Build

```sh
go build -o bin/protoc-gen-grpc-gateway-axios ./cmd/protoc-gen-grpc-gateway-axios
```

## Use with protoc

```sh
protoc \
  --plugin=protoc-gen-api=bin/protoc-gen-grpc-gateway-axios \
  -I proto -I path/to/google/api \
  --api_out=import_extension=.js:gen \
  user/v1/user.proto
```

## Example

```proto
rpc ListUsers(ListUsersRequest) returns (ListUsersResponse) {
  option (google.api.http) = {
    get: "/v1/users"
  };
}
```

```ts
async listUsers(
  request: ListUsersRequest,
  config?: AxiosRequestConfig,
): Promise<ListUsersResponse> {
  const query = new URLSearchParams();
  if (request.page != null) {
    query.append("page", String(request.page));
  }
  (request.ids ?? []).forEach((value) => {
    query.append("ids", String(value));
  });
  Object.entries(request.labels ?? {}).forEach(([key, value]) => {
    query.append(`labels[${key}]`, String(value));
  });
  const queryString = query.toString();

  const response = await this.http.request<ListUsersResponse>({
    ...config,
    url: `/v1/users`.concat(queryString === "" ? "" : `?${queryString}`),
    method: "GET",
  });

  return response.data;
}
```

## Why the query is built by hand

Axios's default parameter serialization is `ids[]=1&ids[]=2` and
`filter[status]=...`, which `runtime.PopulateQueryParameters` does not parse.
The rules the generated code follows:

| Proto | Encoding |
| --- | --- |
| repeated `T` | the same name repeated: `ids=1&ids=2` |
| `map<K,V>` | `labels[key]=value` — brackets, both halves encoded |
| nested message | dotted names: `filter.status=ACTIVE` |
| enum | the enum name |
| `int64` | a decimal string, so precision is not lost |
| absent | only `null` and `undefined` |

`0`, `false` and `""` are therefore sent, not dropped. Path-bound fields and
body-covered fields are excluded together with their descendants, so the gateway
never sets a field twice.

`conformance/` contains the Go-side check: it feeds these query strings to
grpc-gateway's own `PopulateQueryParameters` and asserts the decoded proto.

## Options

| Option | Default | Meaning |
| --- | --- | --- |
| `client_style` | `class` | `class` (`UserServiceClient`) or `functions` |
| `additional_bindings` | `ignore` | `ignore`, or `methods` to also generate `MethodBinding2` |
| `strict_http` | `false` | error on a method without `google.api.http` |
| `streaming` | `ignore` | ignore streaming RPCs, or `error` |

Plus the shared contract parameters listed in `../CONTRACT.md`.

## What is skipped

- Methods with no `google.api.http` rule: skipped with a comment, unless
  `strict_http=true`.
- Streaming RPCs: skipped with a comment, unless `streaming=error`. Axios cannot
  reliably consume grpc-gateway's NDJSON stream, so generating a unary method
  would be a silent lie.

## Notes on specific annotations

- `body: "user.profile"` sends `request.user?.profile`, never
  `request["user.profile"]`.
- `response_body: "user"` makes the HTTP body the value of that field, because
  grpc-gateway marshals the selected field as the whole response. The generated
  method therefore returns the field's type and passes it as the axios generic.
- A `:verb` suffix stays outside the interpolated variable and is not encoded.
- `additional_bindings` are ignored by default; with `methods`, the second
  binding becomes `methodNameBinding2`, identically in the React Query plugin.

## Tests

```sh
go test ./...
```

Golden files live in `testdata/golden`; regenerate with
`go test ./internal/gen -update`.
