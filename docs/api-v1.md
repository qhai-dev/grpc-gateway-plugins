# Protobuf TypeScript SDK 代码生成设计文档

## 1. 项目目标

基于 Protobuf 和 `google.api.http` 注解，构建一套面向 TypeScript 前端项目的 SDK 代码生成工具链。

通过三个独立的 Protoc Plugin 自动生成：

1. Protobuf Message、Enum 对应的 TypeScript 类型。
2. 基于 Axios 的 HTTP API 请求函数。
3. 基于 TanStack Query 的 Query Options 和 Mutation Options。

核心设计原则：

- **职责单一**：每个 Plugin 只负责一种代码生成任务。
- **原生使用习惯**：生成代码贴近 TypeScript、Axios 和 TanStack Query 的使用方式。
- **无 Service Class**：API 使用普通对象字面量组织。
- **依赖注入**：Axios 实例由业务项目创建，通过 API 工厂函数注入。
- **无业务路径依赖**：生成器不需要知道业务项目中 Axios 实例的位置。
- **无全局可变状态**：不采用全局 Runtime 注册 Axios 实例。
- **类型安全**：请求参数、响应结果和 Query Key 保持完整的 TypeScript 类型。
- **独立使用**：三个 Plugin 可以独立使用，也可以组合使用。

## 2. 整体架构

采用三个独立的 Protoc Plugin：

| Plugin                                | 职责                | 输出文件     |
| ------------------------------------- | ------------------- | ------------ |
| `protoc-gen-grpc-gateway-typescript`  | TypeScript 类型生成 | `type.ts`    |
| `protoc-gen-grpc-gateway-axios`       | Axios HTTP API 生成 | `api.ts`     |
| `protoc-gen-grpc-gateway-react-query` | TanStack Query 生成 | `queries.ts` |

整体架构：

```text
                 Protobuf Definitions
                         |
                         v
                 CodeGeneratorRequest
                         |
             +-----------+-----------+
             |           |           |
             v           v           v
        TypeScript     Axios      React Query
         Plugin       Plugin        Plugin
             |           |           |
             v           v           v
          type.ts     api.ts     queries.ts
             |           |           |
             +-----------+-----------+
                         |
                         v
                  Frontend Application
```

三个 Plugin 都直接读取 Protoc 提供的 Proto Descriptor，不依赖其他 Plugin 的执行顺序。

## 3. 生成目录结构

统一使用 `gen/` 作为生成代码根目录。

```text
gen/
└── user/
    └── v1/
        ├── type.ts
        ├── api.ts
        └── queries.ts
```

目录规则：

```text
gen/{proto_package}/{version}/
```

例如：

```protobuf
syntax = "proto3";

package user.v1;
```

生成：

```text
gen/user/v1/
```

文件职责：

| 文件         | 说明                                        |
| ------------ | ------------------------------------------- |
| `type.ts`    | Message、Enum 类型定义                      |
| `api.ts`     | Axios API 工厂函数                          |
| `queries.ts` | Query Keys、Query Options、Mutation Options |

不采用以下命名：

- `user_pb.ts`
- `user_service.ts`
- `user_api.ts`
- `user_react_query.ts`

文件名表达代码职责，目录表达 Proto Package 和版本。

## 4. Plugin 一：TypeScript 类型生成

### 4.1 Plugin 名称

`protoc-gen-grpc-gateway-typescript`

### 4.2 职责

将 Protobuf Message 和 Enum 转换为 TypeScript 类型定义。

输出：

`gen/user/v1/type.ts`

不负责：

- HTTP 请求函数。
- Axios 配置。
- TanStack Query。
- Service Class。
- Zod Schema。
- 运行时数据校验。

### 4.3 示例

输入：

```protobuf
syntax = "proto3";

package user.v1;

message User {
  string id = 1;
  string name = 2;
  int32 age = 3;
}

message GetUserRequest {
  string id = 1;
}

message GetUserResponse {
  User user = 1;
}

enum UserStatus {
  USER_STATUS_UNSPECIFIED = 0;
  USER_STATUS_ACTIVE = 1;
  USER_STATUS_DISABLED = 2;
}
```

生成：

```typescript
export interface User {
  id: string;
  name: string;
  age: number;
}

export interface GetUserRequest {
  id: string;
}

export interface GetUserResponse {
  user?: User;
}

export enum UserStatus {
  USER_STATUS_UNSPECIFIED = 0,
  USER_STATUS_ACTIVE = 1,
  USER_STATUS_DISABLED = 2,
}
```

### 4.4 类型映射规范

| Protobuf 类型 | TypeScript 类型                            |
| ------------- | ------------------------------------------ |
| `string`      | `string`                                   |
| `bool`        | `boolean`                                  |
| `int32`       | `number`                                   |
| `uint32`      | `number`                                   |
| `sint32`      | `number`                                   |
| `fixed32`     | `number`                                   |
| `int64`       | 根据配置映射为 `string` 或 `bigint`        |
| `uint64`      | 根据配置映射为 `string` 或 `bigint`        |
| `float`       | `number`                                   |
| `double`      | `number`                                   |
| `bytes`       | 根据配置映射为 `Uint8Array` 或其他约定类型 |
| `repeated T`  | `T[]`                                      |
| `map<K, V>`   | `Record<K, V>` 或适当的映射类型            |
| `enum`        | TypeScript Enum 或联合类型                 |
| `message`     | 对应 Message 类型                          |

需要额外处理：

- Proto3 Optional。
- Oneof。
- Nested Message。
- Nested Enum。
- Google Well-Known Types。
- Timestamp。
- Duration。
- Struct。
- Value。
- Any。
- FieldMask。

具体类型映射应形成独立配置规范，保证生成结果稳定。

## 5. Plugin 二：Axios API 生成

### 5.1 Plugin 名称

`protoc-gen-grpc-gateway-axios`

### 5.2 职责

根据 Protobuf Service、RPC Method 和 `google.api.http` 注解生成 Axios HTTP API。

输出：

`gen/user/v1/api.ts`

不负责：

- 创建 Axios 实例。
- 管理 Token。
- 配置请求拦截器。
- 管理全局错误处理。
- 生成 Service Class。
- 生成 Query 或 Mutation。

### 5.3 API 组织方式

采用普通对象字面量组织 API，不使用 Class。

参考 TanStack Query 官方示例中的 API 组织方式：

```typescript
export const postsAPI = {
  getPosts: async () => {},
  getPost: async () => {},
};
```

但考虑到 Axios 实例需要由业务项目管理，生成代码采用工厂函数创建 API 对象。

### 5.4 Axios 实例注入

生成代码不导入任何业务项目中的 Axios 模块，也不创建全局 Axios 实例。

通过 `createUserAPI(http)` 注入 Axios 实例。

生成的 `api.ts`：

```typescript
import type { AxiosInstance } from "axios";

import type {
  GetUserRequest,
  GetUserResponse,
  CreateUserRequest,
  CreateUserResponse,
} from "./type";

export function createUserAPI(http: AxiosInstance) {
  return {
    getUser: async (request: GetUserRequest): Promise<GetUserResponse> => {
      const { data } = await http.get<GetUserResponse>(
        `/v1/users/${encodeURIComponent(request.id)}`,
      );

      return data;
    },

    createUser: async (request: CreateUserRequest): Promise<CreateUserResponse> => {
      const { data } = await http.post<CreateUserResponse>("/v1/users", request);

      return data;
    },
  };
}
```

这里的设计约定：

1. 导出 `create{Service}API` 工厂函数。
2. 工厂函数接收 `AxiosInstance`。
3. 返回普通对象字面量。
4. 每个 RPC Method 对应一个异步函数。
5. 函数参数使用对应的 Protobuf Request 类型。
6. 函数返回值使用对应的 Protobuf Response 类型。
7. 不在每个 API 方法中重复传入 Axios 实例。
8. 不创建全局 Axios 实例。
9. 不依赖业务项目的模块路径。

### 5.5 业务项目初始化

业务项目负责创建 Axios 实例：

```typescript
// src/lib/http.ts

import axios from "axios";

export const http = axios.create({
  baseURL: "/api",
  timeout: 10000,
});
```

在统一 API 初始化层创建 API 对象：

```typescript
// src/api/index.ts

import { http } from "@/lib/http";

import { createUserAPI } from "@/gen/user/v1/api";

export const userAPI = createUserAPI(http);
```

业务代码可以直接调用：

```typescript
import { userAPI } from "@/api";

const result = await userAPI.getUser({
  id: "1001",
});
```

### 5.6 HTTP 注解解析

输入：

```protobuf
service UserService {
  rpc GetUser(GetUserRequest) returns (GetUserResponse) {
    option (google.api.http) = {
      get: "/v1/users/{id}"
    };
  }

  rpc CreateUser(CreateUserRequest) returns (CreateUserResponse) {
    option (google.api.http) = {
      post: "/v1/users"
      body: "*"
    };
  }
}
```

需要解析：

| 注解                  | 生成行为                   |
| --------------------- | -------------------------- |
| `get`                 | `http.get()`               |
| `post`                | `http.post()`              |
| `put`                 | `http.put()`               |
| `patch`               | `http.patch()`             |
| `delete`              | `http.delete()`            |
| `body: "*"`           | 整个 Request 作为请求 Body |
| `body: "field"`       | 指定字段作为请求 Body      |
| URL Path 参数         | 替换为 Request 对应字段    |
| Query 参数            | 从 Request 中提取并序列化  |
| `additional_bindings` | 支持额外 HTTP 绑定         |

对于同一个 RPC 存在多个 HTTP Binding 的情况，需要制定统一的生成规则，避免 API 名称冲突。

### 5.7 HTTP 参数映射

输入：

```protobuf
rpc ListUsers(ListUsersRequest) returns (ListUsersResponse) {
  option (google.api.http) = {
    get: "/v1/users"
  };
}

message ListUsersRequest {
  int32 page = 1;
  int32 page_size = 2;
  string keyword = 3;
}
```

生成：

```typescript
listUsers: async (
  request: ListUsersRequest,
): Promise<ListUsersResponse> => {
  const { data } = await http.get<ListUsersResponse>(
    "/v1/users",
    {
      params: request,
    },
  );

  return data;
},
```

对于复杂嵌套参数、Repeated 参数、特殊字符和空值，需要制定统一的序列化策略。

## 6. Plugin 三：TanStack Query 生成

### 6.1 Plugin 名称

`protoc-gen-grpc-gateway-react-query`

### 6.2 职责

基于 RPC Method 生成 TanStack Query 相关代码。

输出：

`gen/user/v1/queries.ts`

主要生成：

- Query Keys。
- Query Options。
- Mutation Options。

不负责：

- 创建 Axios 实例。
- 创建 Axios API。
- 生成 React Service Class。
- 强制生成自定义 React Hooks。
- 管理 QueryClient。
- 自动推断业务缓存失效策略。

### 6.3 依赖注入设计

由于 `api.ts` 导出的是 `createUserAPI` 工厂函数，而不是全局 `userAPI` 实例，因此 `queries.ts` 也采用工厂函数接收 API 对象。

这样生成代码不需要依赖业务项目中的 API 初始化路径。

生成的 `queries.ts`：

```typescript
import { queryOptions, mutationOptions } from "@tanstack/react-query";

import type { createUserAPI } from "./api";

type UserAPI = ReturnType<typeof createUserAPI>;

export function createUserQueries(userAPI: UserAPI) {
  return {
    all: () => ["users"] as const,

    detail: (id: string) =>
      queryOptions({
        queryKey: ["users", "detail", id] as const,
        queryFn: () => userAPI.getUser({ id }),
      }),
  };
}

export function createUserMutations(userAPI: UserAPI) {
  return {
    create: () =>
      mutationOptions({
        mutationFn: userAPI.createUser,
      }),
  };
}
```

### 6.4 Query Key 设计

Query Key 应当具备：

1. 稳定性。
2. 唯一性。
3. 可组合性。
4. 便于缓存失效。
5. 与请求参数保持一致。

建议采用以下结构：

```typescript
export function createUserQueries(userAPI: UserAPI) {
  return {
    all: () => ["users"] as const,

    lists: () => ["users", "list"] as const,

    list: (request: ListUsersRequest) =>
      queryOptions({
        queryKey: ["users", "list", request] as const,
        queryFn: () => userAPI.listUsers(request),
      }),

    details: () => ["users", "detail"] as const,

    detail: (id: string) =>
      queryOptions({
        queryKey: ["users", "detail", id] as const,
        queryFn: () => userAPI.getUser({ id }),
      }),
  };
}
```

对于同一个 RPC，Query Key 应当能够区分不同请求参数。

具体生成策略需要避免对象属性顺序、默认值和可选字段造成不必要的缓存分裂。

### 6.5 Mutation Options 设计

Mutation 不需要像 Query 一样自动生成缓存 Key。

示例：

```typescript
export function createUserMutations(userAPI: UserAPI) {
  return {
    create: () =>
      mutationOptions({
        mutationFn: userAPI.createUser,
      }),
  };
}
```

缓存失效由业务项目根据实际业务关系处理。

例如：

```typescript
const queryClient = useQueryClient();

const createUser = useMutation({
  ...userMutations.create(),

  onSuccess: () => {
    queryClient.invalidateQueries({
      queryKey: userQueries.all(),
    });
  },
});
```

不建议在生成器中默认推断所有 Mutation 对应的缓存失效关系，因为这属于业务语义，而不是 Proto HTTP 契约。

## 7. 前端项目完整使用方式

推荐业务项目采用统一初始化层。

目录结构：

```text
src/
├── lib/
│   └── http.ts
├── api/
│   └── index.ts
└── gen/
    └── user/
        └── v1/
            ├── type.ts
            ├── api.ts
            └── queries.ts
```

### 7.1 HTTP Client

```typescript
// src/lib/http.ts

import axios from "axios";

export const http = axios.create({
  baseURL: "/api",
  timeout: 10000,
});
```

### 7.2 API 和 Query 初始化

```typescript
// src/api/index.ts

import { http } from "@/lib/http";

import { createUserAPI } from "@/gen/user/v1/api";

import { createUserQueries, createUserMutations } from "@/gen/user/v1/queries";

export const userAPI = createUserAPI(http);

export const userQueries = createUserQueries(userAPI);

export const userMutations = createUserMutations(userAPI);
```

### 7.3 查询数据

```typescript
import { useQuery } from "@tanstack/react-query";

import { userQueries } from "@/api";

export function UserDetail() {
  const { data, isLoading } = useQuery(
    userQueries.detail("1001"),
  );

  if (isLoading) {
    return <div>Loading...</div>;
  }

  return <div>{data?.user?.name}</div>;
}
```

### 7.4 创建数据

```typescript
import { useMutation, useQueryClient } from "@tanstack/react-query";

import { userMutations, userQueries } from "@/api";

export function CreateUser() {
  const queryClient = useQueryClient();

  const mutation = useMutation({
    ...userMutations.create(),

    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: userQueries.all(),
      });
    },
  });

  return (
    <button
      onClick={() => mutation.mutate({ name: "Alice" })}
    >
      Create User
    </button>
  );
}
```

### 7.5 直接调用 API

API 层不强制依赖 TanStack Query：

```typescript
import { userAPI } from "@/api";

const result = await userAPI.getUser({
  id: "1001",
});
```

API 可以用于普通业务函数、非 React 环境、自定义请求流程和单元测试。

## 8. 配置设计

由于 Axios 实例通过参数注入，生成器不再需要 `http_import` 或 `http_export` 配置。

Plugin 不需要知道：

- Axios 实例定义在哪里。
- 业务项目使用什么路径别名。
- Axios 实例导出的变量名称。
- 业务项目如何配置拦截器。

建议配置项：

| 配置项             | 所属 Plugin | 说明                      |
| ------------------ | ----------- | ------------------------- |
| `int64`            | TypeScript  | 64 位整数映射策略         |
| `bytes`            | TypeScript  | Bytes 映射策略            |
| `enum`             | TypeScript  | Enum 生成策略             |
| `query_key_prefix` | React Query | Query Key 前缀            |
| `query_options`    | React Query | 是否生成 Query Options    |
| `mutation_options` | React Query | 是否生成 Mutation Options |

配置项应保持精简，避免过早引入大量不必要的定制能力。

## 9. 技术实现架构

建议三个 Plugin 共享一套内部基础库，避免重复解析 Proto Descriptor 和 HTTP Annotation。

推荐内部模块：

```text
internal/
├── descriptor/
│   ├── parser.go
│   ├── message.go
│   ├── service.go
│   └── method.go
│
├── http/
│   ├── annotation.go
│   ├── binding.go
│   ├── path.go
│   └── query.go
│
├── typescript/
│   ├── naming.go
│   ├── type_mapping.go
│   ├── import.go
│   └── writer.go
│
├── generator/
│   ├── typescript.go
│   ├── axios.go
│   └── react_query.go
│
└── plugin/
    ├── typescript.go
    ├── axios.go
    └── react_query.go
```

每个 Plugin 独立实现标准 Protoc Plugin 协议：

```text
CodeGeneratorRequest
        |
        v
   Descriptor Parser
        |
        v
     Generator
        |
        v
CodeGeneratorResponse
```

生成器应当避免依赖本地工作目录、网络请求或其他不可声明的外部输入，以便支持 Bazel 的 Hermetic Build 和远程缓存。

## 10. 代码生成约束

### 10.1 文件生成

- 每个 Plugin 只生成自己负责的文件。
- 三个 Plugin 使用相同的目录映射规则。
- 不允许多个 Plugin 写入同一个文件。
- 生成结果应当具有确定性。
- 输入未变化时，生成结果应保持一致。

### 10.2 命名规范

Proto RPC：

```protobuf
rpc GetUser(GetUserRequest) returns (GetUserResponse);
```

对应 API：

```typescript
userAPI.getUser();
```

对应 Query：

```typescript
userQueries.detail();
```

对于不同 Service 中出现相同 RPC 名称的情况，应制定命名空间和冲突处理规则，避免 API 名称覆盖。

### 10.3 错误处理

Axios 层保留 Axios 原生错误行为。

不在生成代码中统一吞掉异常，也不强制转换为自定义错误对象。

项目可以通过 Axios Interceptor 统一处理：

- HTTP 状态码。
- Token 过期。
- 业务错误码。
- 网络错误。
- 请求超时。

### 10.4 请求取消

TanStack Query 的 Query Function 可以接收 `AbortSignal`。

生成器应支持将 `signal` 从 Query Function 传递到 API 层，再传递给 Axios。

例如 API：

```typescript
getUser: async (
  request: GetUserRequest,
  config?: Pick<AxiosRequestConfig, "signal">,
): Promise<GetUserResponse> => {
  const { data } = await http.get<GetUserResponse>(
    `/v1/users/${encodeURIComponent(request.id)}`,
    config,
  );

  return data;
},
```

Query：

```typescript
detail: (id: string) =>
  queryOptions({
    queryKey: ["users", "detail", id] as const,
    queryFn: ({ signal }) =>
      userAPI.getUser({ id }, { signal }),
  }),
```

这样可以与 TanStack Query 的请求取消机制配合。

## 11. 测试策略

### 11.1 TypeScript Plugin

覆盖：

- 基础类型映射。
- Enum。
- Nested Message。
- Repeated。
- Map。
- Oneof。
- Optional。
- Well-Known Types。
- 64 位整数。

### 11.2 Axios Plugin

覆盖：

- GET。
- POST。
- PUT。
- PATCH。
- DELETE。
- URL Path 参数。
- Query 参数。
- Request Body。
- Response Body。
- `additional_bindings`。
- 特殊字符编码。
- 多 Service 命名冲突。
- Axios 实例注入。

### 11.3 React Query Plugin

覆盖：

- Query Key 生成。
- Query Options 生成。
- Mutation Options 生成。
- 多 RPC。
- 多 Service。
- 请求参数类型。
- Query Key 冲突。
- AbortSignal 传递。
- API 工厂函数类型兼容性。

### 11.4 集成测试

建立完整示例：

```text
e2e/
├── proto/
│   └── user/
│       └── v1/
│           └── user.proto
│
└── frontend/
    ├── src/
    │   ├── lib/
    │   │   └── http.ts
    │   ├── api/
    │   │   └── index.ts
    │   └── gen/
    │       └── user/
    │           └── v1/
    │               ├── type.ts
    │               ├── api.ts
    │               └── queries.ts
    └── package.json
```

验证生成代码能够通过 TypeScript 编译，并且可以在真实 React 项目中正常使用。

## 12. 开发阶段规划

### Phase 1：基础类型生成

实现：

- Protoc Plugin 基础框架。
- Proto Descriptor 解析。
- Message 类型映射。
- Enum 类型映射。
- 文件路径和命名规则。

交付：

`protoc-gen-grpc-gateway-typescript`

### Phase 2：Axios API 生成

实现：

- Service 和 RPC 解析。
- `google.api.http` Annotation 解析。
- HTTP Method 映射。
- URL Path 参数替换。
- Query 参数序列化。
- Request Body 映射。
- Axios API 工厂函数生成。

交付：

`protoc-gen-grpc-gateway-axios`

### Phase 3：TanStack Query 生成

实现：

- Query Key 生成。
- Query Options 生成。
- Mutation Options 生成。
- API 工厂函数类型引用。
- AbortSignal 传递。

交付：

`protoc-gen-grpc-gateway-react-query`

### Phase 4：工具链集成

实现：

- Buf 集成。
- Bazel Rule 集成。
- E2E 测试。
- 示例项目。
- 文档和使用说明。

## 13. 最终设计总结

三个独立 Plugin：

```text
protoc-gen-grpc-gateway-typescript
protoc-gen-grpc-gateway-axios
protoc-gen-grpc-gateway-react-query
```

统一生成：

```text
gen/
└── user/
    └── v1/
        ├── type.ts
        ├── api.ts
        └── queries.ts
```

职责边界：

```text
type.ts
  └── TypeScript 类型定义

api.ts
  └── Axios API 工厂函数
      └── createUserAPI(http)

queries.ts
  └── TanStack Query 工厂函数
      ├── createUserQueries(userAPI)
      └── createUserMutations(userAPI)
```

业务项目负责初始化依赖：

```typescript
const userAPI = createUserAPI(http);

const userQueries = createUserQueries(userAPI);

const userMutations = createUserMutations(userAPI);
```

最终业务代码：

```typescript
// API
userAPI.getUser({ id: "1001" });

// Query
useQuery(userQueries.detail("1001"));

// Mutation
useMutation(userMutations.create());
```

整体设计遵循：

**Protobuf 定义接口契约，TypeScript Plugin 生成类型，Axios Plugin 生成请求函数，React Query Plugin 生成缓存和请求状态管理配置。**

生成器负责生成能力，业务项目负责提供依赖和完成初始化。

三个 Plugin 各司其职，生成代码保持轻量、可读、无业务路径耦合，并尽量贴近前端开发者熟悉的使用习惯。
