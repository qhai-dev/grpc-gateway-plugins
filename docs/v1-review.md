# v1 设计评审与调整建议

> 注：本文的结论已合并进 `docs/v1.md`（v1.1）。此文件保留为评审记录和实现细节参考，正式设计以 `docs/v1.md` 为准。

本文基于 `docs/v1.md`，并实际拉取、阅读了两份参考实现的源码：

- [grpc-ecosystem/protoc-gen-grpc-gateway-ts](https://github.com/grpc-ecosystem/protoc-gen-grpc-gateway-ts)（Apache-2.0）
- [go-kratos/protoc-gen-typescript-http](https://github.com/go-kratos/protoc-gen-typescript-http)（MIT，源自 Einride）

对照的语义基准是 [grpc-gateway 的 `runtime.PopulateQueryParameters`](https://github.com/grpc-ecosystem/grpc-gateway/blob/main/runtime/query.go)，因为生成出来的客户端最终必须能被 grpc-gateway 正确解析。

---

## 0. 结论

整体方向可行，三个插件 + 共享解析层 + Buf/Bazel 的分层是合理的。但 v1 文档目前更像“能力清单”，还缺少实现前必须定死的**生成契约**和**grpc-gateway 语义细节**。直接按现文档开工会遇到几类硬问题：

1. **Query 参数生成方式是错的**：`axios.get(url, { params: request })` 与 grpc-gateway 的 query 解析规则不兼容。
2. **类型映射不完整**：bytes、wrappers、Any/Empty、Struct/Value/ListValue、map key、oneof、proto3 optional、嵌套类型命名都没有定死。
3. **跨文件与外部类型 import 没有方案**：这是三插件契约里最容易后期返工的部分。
4. **`response_body`、`additional_bindings` 被列为必须支持，但没有设计**；两份参考实现也都没有完整支持，不能照抄。
5. **React Query 缺少取消、enabled、options 透传**；另外 `bigint` 与 React Query 的 query key 哈希存在冲突。
6. **流式 RPC 没有进入范围声明**；Axios 在浏览器端无法可靠消费 NDJSON 流，v1 应明确不支持。
7. **Bazel 部分存在一个隐藏冲突**：如果 Axios 插件生成共享 runtime 文件，多个 target 会争抢同一个输出路径；建议改为“每个 API 文件内联 helper”。
8. 文档里有几处具体错误（`protogen.Plugin{}` 不能这样构造、`user_connect.ts` 是笔误、§8.6 前后矛盾等）。

下面按优先级展开。

---

## 1. 两份参考实现的真实行为

### 1.1 protoc-gen-grpc-gateway-ts

关键实现点：

| 维度 | 行为 |
| --- | --- |
| 输出 | 每个 proto 一个 `<name>.pb.ts`，另生成共享 `fetch.pb.ts` |
| 类型引用 | 跨文件使用 `import * as module from "..."`，再 `module.Type` 引用 |
| int64 | `string` |
| bytes | `Uint8Array`（请求体用 `b64Encode` replacer 转 base64） |
| enum | 字符串 enum 对象 |
| 字段可选性 | 所有字段 `?:` |
| oneof | 生成 `BaseX` + `OneOf<...>` 联合辅助类型 |
| URL path | 直接 `${req["id"]}` 插入，**不做 encodeURIComponent** |
| Query | **只对 GET 生成**；把请求对象递归 flatten 成 `a.b=value`；**丢弃 0 / false / "" / 空数组** |
| map | 被当作普通对象递归成 `m.k=v`，**不符合 gateway 的 `m[k]=v`** |
| body | 支持 `*` 和字段名；嵌套字段 `"a.b"` 会生成 `req["a.b"]`，**是错的** |
| response_body | 不支持 |
| additional_bindings | 不支持 |
| 流式 | 支持 server streaming，基于 fetch + NDJSON |
| 生成代码质量 | 默认带 `// @ts-nocheck`，除非开启 styling check |

可借鉴：int64 默认 string、跨文件 import 用 namespace、共享 query 编码思路、fetch 流式实现。
不要照抄：GET-only query、丢零值、map 编码、path 不编码、`@ts-nocheck`。

### 1.2 protoc-gen-typescript-http

关键实现点：

| 维度 | 行为 |
| --- | --- |
| 输出 | **每个 proto package 一个 `index.ts`**，package 名直接映射目录 |
| 类型引用 | 不生成 import；遍历 service/message 引用到的类型，**把外部类型也内联到同一个文件** |
| int64 | `number`（有精度风险） |
| bytes | `string` |
| enum | 字符串字面量联合 |
| 字段可选性 | 普通字段 `field: T \| undefined`；`optional`/oneof 字段 `field?: T` |
| oneof | 扁平可选字段，**不表达互斥** |
| URL path | 直接 `${request.name}` 插入，**不做编码** |
| Query | 所有 HTTP 方法都生成；支持嵌套 `a.b=v`、repeated、map `m[k]=v` |
| Query 零值 | 用 `if (request.x)` 判断，**会漏掉 0 / false / ""** |
| body | 支持 `*`、字段名、嵌套字段（`request?.a?.b`） |
| response_body | 不支持 |
| additional_bindings | 解析进 IR，但**不生成额外方法** |
| 流式 | SSE / WebSocket，抽象成 `ClientTransport` |
| 生成代码质量 | 固定带 `// @ts-nocheck` |

可借鉴：嵌套 body、map query、`ClientTransport` 抽象、WKT 映射。
不要照抄：package 级单文件 + 外部类型内联（会重复类型、破坏类型同一性）、int64=number、truthiness 判断、全局 `seen` 的 query 字段遍历（同一消息类型在多个分支出现时只会展开第一个分支，是 bug）。

### 1.3 对照表

| 维度 | grpc-gateway-ts | typescript-http | v1 文档 |
| --- | --- | --- | --- |
| 输出粒度 | 每 proto 一个文件 | 每 package 一个文件 | 每 proto 三个文件 |
| 跨文件 import | 有 | 无（内联） | 未定义 |
| int64 | string | number | 可配置，未定默认 |
| bytes | Uint8Array | string | 未提 |
| enum | string enum | string union | 未定 |
| 字段可选性 | `?:` | `T \| undefined` / `?:` | 未定 |
| oneof | 联合辅助类型 | 扁平可选 | 未定 |
| query 方法范围 | 仅 GET | 所有方法 | 未明确 |
| query 编码 | 部分正确 | 较完整但有 truthiness bug | **`params: request`，错误** |
| path 编码 | 无 | 无 | 有 `encodeURIComponent` |
| body | 支持 `*`/字段 | 支持 `*`/字段/嵌套 | 支持 |
| response_body | 无 | 无 | 要求支持 |
| additional_bindings | 无 | 只解析 | 要求支持 |
| 流式 | fetch NDJSON | SSE/WS | 未提 |
| 请求取消 | 无 | 无 | 未提 |
| React Query | 无 | 无 | 有 |

---

## 2. P0：必须调整

### 2.1 Query 参数不能是 `params: request`

v1 文档 §4.6 写的是：

```ts
this.http.get("/v1/users", { params: request });
```

Axios 默认的 params 序列化是 `a[]=1&a[]=2`、`obj[key]=value`，而 grpc-gateway 的规则是：

| Proto 类型 | Query 编码 |
| --- | --- |
| string | 原样 |
| bool | `true` / `false` |
| int32 / uint32 / float / double | 十进制 |
| int64 / uint64 / fixed64 / sfixed64 / sint64 | 十进制字符串（默认 string 映射） |
| bytes | base64 字符串 |
| enum | **枚举名**（gateway 也接受数字，但客户端应发名字） |
| Timestamp | RFC3339Nano 字符串 |
| Duration | `3.5s` 这样的字符串 |
| FieldMask | 逗号分隔的路径字符串 |
| repeated T | 同名重复参数：`ids=1&ids=2` |
| message | 递归展开：`filter.status=ACTIVE` |
| map<K,V> | **`labels[key]=value`**，key 和 value 都 URL 编码 |
| oneof | 当前设置的成员按普通字段编码 |
| optional | 仅当值不是 `null`/`undefined` 时发送 |

另外三个关键规则：

1. **path 已绑定的字段必须从 query 中排除**，且要排除其后代字段。
2. **body 覆盖的字段必须从 query 中排除**。`body: "*"` 时不发任何 query；`body: "user"` 时排除 `user` 及其所有后代。
3. **不能用 truthiness 判断**。`0`、`false`、`""` 都是合法值，应该发送。用 `!= null` 判断；只有 `undefined`/`null` 才省略。这一点两份参考实现都做错了。

建议生成显式代码，而不是依赖 Axios params：

```ts
const query = new URLSearchParams();
if (request.page != null) query.append("page", String(request.page));
if (request.pageSize != null) query.append("pageSize", String(request.pageSize));
if (request.keyword != null) query.append("keyword", request.keyword);
if (request.filter?.status != null) {
  query.append("filter.status", String(request.filter.status));
}
(request.ids ?? []).forEach((v) => query.append("ids", String(v)));
Object.entries(request.labels ?? {}).forEach(([k, v]) => {
  query.append(`labels[${k}]`, String(v));
});

const qs = query.toString();
const url = `/v1/users${qs ? `?${qs}` : ""}`;
```

这样做的好处：

- 与 gateway 的 `url.ParseQuery` + `PopulateQueryParameters` 完全对齐；
- 不依赖 Axios 全局 `paramsSerializer`；
- 生成代码可读、可调试；
- 不需要共享 runtime 文件（对 Bazel 友好，见 §4.2）。

### 2.2 Path 参数需要编码，且要区分单段/多段模板

v1 文档的 `${encodeURIComponent(request.id)}` 是对的，但只适用于默认的单段变量 `{id}`。

HTTP 模板有两种：

- `{id}`：默认等价于 `{id=*}`，值必须是单段。用 `encodeURIComponent(String(value))`。
- `{name=shippers/*}` / `{name=**}`：值本身包含 `/`。**不能整体 encode**，否则 `shippers/123` 变成 `shippers%2F123`，gateway 会按单段匹配而失败。应按段编码：

```ts
const encodePathSegments = (v: unknown) =>
  String(v).split("/").map(encodeURIComponent).join("/");
```

生成器需要根据 `HttpRule` 的变量子模板决定用哪一种。建议在 `internal/httprule` 里把变量建模成：

```go
type Variable struct {
    FieldPath FieldPath
    MultiSeg  bool // 子模板包含 "/" 或 "**"
}
```

同时，path 参数要做存在性校验，但只能用 `== null`，不能 `if (!request.id)`，否则 `0`、`false` 会被误判为缺失。

### 2.3 `body`、`response_body`、`additional_bindings` 需要明确设计

**body**：

- `""`：无 body。
- `"*"`：整个 request。
- `"user"`：`request.user`。
- `"user.profile"`：`request.user?.profile`，注意嵌套空值传播。

不要生成 `request["user.profile"]`（grpc-gateway-ts 的错误做法）。

**response_body**：

gateway 在写响应时只输出 response message 的指定字段。生成端必须解包：

```ts
return this.http
  .request<GetUserResponse>({ ...config, url, method: "GET" })
  .then((r) => r.data.user);
```

返回值类型也要相应变成 `User`，React Query 的 data 类型同理。这个信息必须进入共享 IR，否则 Axios 和 React Query 两个插件会不一致。

**additional_bindings**：

`google.api.http` 允许一个 RPC 有多个 HTTP 绑定。gateway 服务端会为每个绑定注册路由，但两份参考实现都没有生成对应的客户端方法。v1 要求“正确处理”，建议定成：

- **默认只生成 primary binding**（额外绑定通常是为旧客户端保留的别名，新 SDK 调用主路由即可）。
- 提供 `additional_bindings=methods` 选项，开启后为每个额外绑定生成方法，命名必须确定且两个插件共享：
  - primary：`getUser`
  - 第 2 个绑定：`getUserBinding2`
  - 第 3 个绑定：`getUserBinding3`
- 每个绑定有自己的 `body`、`response_body`、path/query 规则，不能复用 primary 的。
- `additional_bindings` 不能嵌套，解析时忽略嵌套即可。

这个策略需要写进 v1，否则实现时 Axios 插件和 React Query 插件会各自发明命名。

### 2.4 类型映射必须补全并定默认值

建议的完整矩阵：

| Proto | 默认 TS | 说明 |
| --- | --- | --- |
| `double` / `float` | `number` | JSON 的 NaN/Infinity 是字符串，暂不支持 |
| `int32` / `uint32` / `sint32` / `fixed32` / `sfixed32` | `number` | |
| `int64` / `uint64` / `sint64` / `fixed64` / `sfixed64` | `string` | 与 protojson 一致；`number` 可选但需明确精度风险 |
| `bool` | `boolean` | |
| `string` | `string` | |
| `bytes` | `string` | base64；与 protojson 一致。`Uint8Array` 放后续 |
| `enum` | 字符串字面量联合 | `enum_style=union` 默认；`const` / `enum` 可选 |
| `message` | 具名 type | |
| `repeated T` | `T[]` | |
| `map<K,V>` | `Record<string, V>` | **JSON 的 key 永远是 string**，不要生成 `{[key:number]:V}` |
| `optional` | `field?: T` | |
| `oneof` | 互斥联合 | 见下 |
| `Timestamp` | `string` | RFC3339 |
| `Duration` | `string` | |
| `FieldMask` | `string` | 逗号分隔 |
| `Struct` | `Record<string, JsonValue>` | |
| `Value` | `JsonValue` | 递归联合 |
| `ListValue` | `JsonValue[]` | |
| `Empty` | `Record<string, never>` | |
| wrappers | `T \| null` | |
| `Any` | `{ "@type": string; [key: string]: unknown }` | |
| `NullValue` | `null` | |

`JsonValue` 定义一次：

```ts
type JsonValue =
  | null
  | boolean
  | number
  | string
  | JsonValue[]
  | { [key: string]: JsonValue };
```

**oneof** 建议生成显式互斥联合，而不是扁平可选字段：

```ts
export type Payment =
  | { card: Card; bank?: never; cash?: never }
  | { bank: Bank; card?: never; cash?: never }
  | { cash: Cash; card?: never; bank?: never };
```

注意 `proto3 optional` 会生成 synthetic oneof，必须排除，不能当成业务 oneof。

**嵌套类型命名**：建议 `Outer_Inner`（protoc-gen-go 风格），并检测与顶层类型重名的情况。enum 值用字符串字面量，避免 `allow_alias` 带来的重复问题。

**字段命名**：默认用 `json_name`（lowerCamel），提供 `use_proto_names=true` 对齐 protojson 的 `UseProtoNames`。这个选项必须三插件共享，否则 query/body 的字段名会不一致。

**int64 用 `bigint` 的风险**：React Query 的 query key 会用 `JSON.stringify` 做稳定哈希，`bigint` 会直接抛错；axios 请求体也需要自定义 replacer/reviver。建议 v1 只支持 `string`（默认）和 `number`，`bigint` 延后，或者至少明确“开启 bigint 时 query key 不能用完整 request”。

### 2.5 跨文件与外部类型 import 必须有方案

v1 只说“支持跨文件引用”，但没有定义 import 路径怎么算。`protoc` 插件拿不到 `-I` 信息，这是 grpc-gateway-ts 需要 `ts_import_roots` 的原因。

建议：

1. **文件内引用**：同文件直接使用类型名。
2. **同一次生成中的其他 proto**：`import type * as <alias> from "<relative>";`，alias 由 proto path 生成（例如 `user_v1_address_pb`），避免两个文件导出同名类型时冲突。namespace import 比逐个具名 import 更稳。
3. **外部依赖**（googleapis、公司公共 proto）：提供 `M` 映射，格式与 protoc-gen-go 一致：
   ```
   Mgoogle/type/money.proto=@acme/grpc-types/google/type/money
   ```
   没有映射时回退到相对路径，并在生成结果里写注释提示可能缺失。
4. **Well-known types**：直接内联成 TS 基础类型，不生成也不 import `google/protobuf/*.ts`。
5. **永远跳过** `google/protobuf/*`、`google/api/annotations.proto`、`google/api/http.proto` 的代码生成，即使它们在 `file_to_generate` 里（例如 Buf 开了 `include_imports`）。
6. `import_extension=.js` 只加在相对 import 上，不加在包名 import 上。

`M` 参数在 `protoc` 里是重复出现的，`flag.FlagSet` 不直接支持，需要用自定义 `flag.Value` 收集。

### 2.6 请求取消必须支持

这是 v1 最容易被忽略、但一上 React Query 就会疼的点。

生成的方法必须接受 Axios 配置：

```ts
getUser(
  request: GetUserRequest,
  config?: AxiosRequestConfig,
): Promise<GetUserResponse>
```

React Query 的 `queryFn` 会提供 `signal`：

```ts
queryFn: ({ signal }) => client.getUser(request, { signal })
```

否则页面切换、参数变化时无法取消在途请求，React Query 的取消能力等于废掉。

同时建议保留 `config` 透传能力（headers、timeout、signal、自定义 baseURL 覆盖等），但生成代码不要把 `url`/`method`/`data` 暴露成可被覆盖的字段；合并顺序建议 `{ ...config, url, method, data }`。

### 2.7 流式 RPC 明确 out of scope

v1 没有提流式，这会留下歧义。建议：

- v1 只支持 unary。
- Axios 在浏览器端不能可靠消费 grpc-gateway 的 NDJSON 流；`onDownloadProgress` 不是通用方案。
- server-streaming 可以在后续用独立的 fetch 插件或 `ClientTransport` 方案实现，不应混进 Axios 插件的 v1。
- 生成器遇到 streaming method 时：
  - 默认跳过并在输出文件里写注释；
  - `streaming=error` 时直接报错；
  - 不要静默生成一个错误的 unary 方法。
- client-streaming / bidi 本来就不是 grpc-gateway 的 HTTP 能力，直接跳过。

### 2.8 `protogen` 的正确用法

v1 §6.2 的示例：

```go
plugin := &protogen.Plugin{}
```

是错的，`protogen.Plugin` 不能这样构造。正确做法：

```go
opts := protogen.Options{ParamFunc: params.Set}
gen := opts.New(req)
gen.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL)
```

并且必须声明 `FEATURE_PROTO3_OPTIONAL`，否则 Buf/protoc 在遇到 `optional` 字段时会直接拒绝。

---

## 3. P1：建议调整

### 3.1 共享 IR，而不只是共享 descriptor 解析

v1 §6.3 说共享 descriptor 解析、命名、import、HTTP annotation。建议再进一步，定义一个共享 IR：

```go
package ir

type File struct {
    ProtoPath string
    Package   string
    OutputBase string
    Messages  []*Message
    Enums     []*Enum
    Services  []*Service
    Imports   []*Import
}

type Service struct {
    Name    string
    Methods []*Method
}

type Method struct {
    Name        string
    ClientName  string // getUser
    Input, Output *MessageRef
    Bindings    []*Binding
    Streaming   bool
}

type Binding struct {
    HTTPMethod  string
    Template    *httprule.Template
    Body        httprule.FieldPath
    ResponseBody httprule.FieldPath
    QueryFields []httprule.FieldPath
    IsPrimary   bool
}
```

三个插件都消费 IR，模板层分开。这样能保证：

- Axios 与 React Query 的 method 名、client 名、binding 后缀一致；
- `response_body` 的返回类型两边一致；
- query 字段列表只算一次；
- 类型映射只实现一次。

这是比“共享解析、各写模板”更强的契约。

### 3.2 不要生成共享 runtime 文件，改为每个 API 文件内联 helper

v1 没有明确这一点，但参考实现都用了共享文件（`fetch.pb.ts`、`ClientTransport`）。共享文件在 Buf 下没问题，在 Bazel 下会有输出冲突：

- 每个 proto 的生成 action 如果都尝试写同一个 `grpc_gateway_runtime.ts`，同一个 target 内会重复声明输出；
- 多个 target 生成到同一 package 时也会冲突。

建议：

- query 构建、path 编码都在每个 `_api.ts` 里生成私有 helper（或直接内联语句）；
- 每个文件自包含，Bazel action 之间完全并行；
- 代价是少量重复代码，可接受，也便于 tree-shaking。

如果后续确实需要共享 runtime，再通过 `runtime_dir` 选项显式放到 target 专属目录。

### 3.3 每个 proto 的三种文件都要稳定产出

Bazel 的 action 必须在运行前声明输出。如果插件“有 service 才生成 `_api.ts`”，Starlark 无法预知，只能跑额外的分析。建议：

- 对每个 `file_to_generate` 都生成 `<base>_pb.ts`、`<base>_api.ts`、`<base>_react_query.ts`；
- 没有内容时生成只含 header 注释的文件；
- 提供 `empty_files=skip` 选项给纯 Buf 用户，但 Bazel 默认用 `emit`。

这样 Starlark 的输出声明是纯路径计算，简单且可缓存。

### 3.4 文件名与后缀契约

v1 的 `output_suffix` 太模糊。建议定成：

| 文件 | 默认 |
| --- | --- |
| 类型 | `<base>_pb.ts` |
| Axios | `<base>_api.ts` |
| React Query | `<base>_react_query.ts` |

三个插件都接受同一组契约参数，即使某个插件只用其中一部分：

```
types_suffix=_pb.ts
api_suffix=_api.ts
query_suffix=_react_query.ts
import_extension=.js
use_proto_names=false
```

否则用户只改一个插件的 `output_suffix`，另外两个生成的 import 立刻断掉。这一点必须写进 v1，并在集成测试里覆盖“自定义后缀 + 三插件一起编译”。

`_pb.ts` 与 protoc-gen-es 的默认命名相同，如果项目里同时使用，建议提供 `types_suffix=.types.ts` 之类的示例。默认值可以保持 v1 的 `_pb.ts`。

### 3.5 生成文件不要带 `@ts-nocheck`

两份参考实现都默认关闭类型检查。我们的目标是“生成代码可以通过 TypeScript 编译”，那就应该让它在 `strict` 下真的编译，而不是用 `@ts-nocheck` 掩盖问题。建议：

- 不加 `@ts-nocheck`；
- 只在必要时对单行加 `eslint-disable-next-line`；
- CI 用 `strict + exactOptionalPropertyTypes + noUncheckedIndexedAccess + verbatimModuleSyntax + NodeNext` 编译生成结果。

### 3.6 没有 `google.api.http` 的方法

真实项目里经常混有 gRPC-only 方法。建议：

- 默认跳过，并在输出里写注释；
- `strict_http=true` 时报错；
- 不要生成一个没有 URL 的坏方法。

### 3.7 React Query 生成细节

建议的 hook 形态：

```ts
export const userServiceKeys = {
  all: ["userService"] as const,
  getUser: (request: GetUserRequest | undefined) =>
    [...userServiceKeys.all, "getUser", request] as const,
  listUsers: (request: ListUsersRequest) =>
    [...userServiceKeys.all, "listUsers", request] as const,
};

export function useGetUser(
  client: UserServiceClient,
  request: GetUserRequest | undefined,
  options?: Omit<
    UseQueryOptions<GetUserResponse, Error>,
    "queryKey" | "queryFn" | "enabled"
  >,
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

要点：

- `request` 允许 `undefined`，默认 `enabled: request !== undefined`，方便“id 还没拿到就不请求”。
- `options` 透传 `select`、`staleTime`、`enabled` 等，但不能覆盖 `queryKey`/`queryFn`。
- `queryKey` 必须包含**完整 request**，不能只放 `request.id`。v1 §5.5 的 `detail(id)` 对 list 查询是错的。
- key 工厂按 service 作用域命名，避免不同 service 的 key 冲突。
- mutation hook：

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

- Query/Mutation 分类默认按 HTTP method；提供 `queries=<service.FullName>.<Method>,...` 覆盖，处理“POST 查询”这类情况。
- 同一 proto 文件里多个 service 有同名 method 时，hook 名会冲突；生成器要检测并加 service 前缀，或强制 `hook_prefix=service`。
- v1 不做自动 invalidate，但 key 工厂必须导出，供用户手动 invalidate。

### 3.8 `additional_bindings` 与 React Query

如果开启 extra bindings，React Query 也要为每个 binding 生成 hook 和 key，命名与 Axios 方法一致（`useGetUserBinding2` / `userServiceKeys.getUserBinding2`），否则两个插件对不上。

---

## 4. Bazel 设计修正

### 4.1 Action 依赖

v1 §8.6 说“Bazel 中应明确建立 Proto → TS → Axios → React Query 的依赖”，但后面又说“如果只读 descriptor，不需要串行依赖”。这两句是矛盾的。

实际情况：

- 三个插件都只读 Proto descriptor；
- 它们通过**命名契约**产生 import，不读取彼此的输出；
- 因此三个 action 可以完全并行，不应该人为串行；
- 真正需要聚合的是“编译 target”，它同时依赖三个输出。

建议把 §8.6 改成：**三个生成 action 是兄弟关系；组合 rule 用 depset 聚合输出；`ts_project` 之类的消费 target 同时依赖三者。**

### 4.2 输出声明与 runtime 文件

见 §3.2。每个文件自包含后，Starlark 只需要按 `<base><suffix>` 声明输出，不需要额外声明共享 runtime。

### 4.3 ProtoInfo 细节

v1 说“基于 ProtoInfo”，建议补充：

- `proto_info.direct_sources`：决定对哪些 proto 调插件；
- `proto_info.transitive_sources`：作为 action inputs；
- `proto_info.transitive_proto_path`：拼 `--proto_path`；
- 插件二进制通过 toolchain 提供，用 `--plugin=protoc-gen-...=<path>` 和 `--..._out=<opts>:<outdir>`；
- `google/api/annotations.proto` 必须来自 `proto_library` 依赖，不能假设 protoc 内置；
- 输出路径由 proto path 推导，Starlark 必须和插件用同一套 suffix 契约；
- 多个 target 生成同一 proto 时会出现输出冲突，建议输出放到 target 专属目录，或文档明确“一个 proto 集只建一个 SDK target”。

### 4.4 Toolchain

建议：

```python
grpc_gateway_typescript_toolchain(
    typescript = "//cmd/protoc-gen-grpc-gateway-typescript",
    axios = "//cmd/protoc-gen-grpc-gateway-axios",
    react_query = "//cmd/protoc-gen-grpc-gateway-react-query",
)
```

- 模块自己注册默认 toolchain，用户不需要手写；
- `go_binary` 建议 `pure = "on"`，便于远程执行；
- toolchain type 单独导出；
- 三个二进制可以放在一个 toolchain 里，也可以拆成三个；v1 用一个即可。

### 4.5 组合 rule 的校验

- `axios=True` 必须同时 `typescript=True`；
- `react_query=True` 必须同时 `typescript=True` 且 `axios=True`；
- 在 analysis 阶段报错，不要等到编译时才发现 import 不存在。

### 4.6 “不依赖本地安装的 protoc”

v1 §8.1 的表述容易误解。应改成：**不依赖系统 PATH 里的 protoc/buf；protoc 由 Bazel toolchain 提供，插件由 `go_binary` 构建并由 toolchain 注入。**

---

## 5. 测试策略补充

v1 §10 的单元/golden/集成/Bazel 分类是对的，建议补充三类高价值测试：

### 5.1 Query 语义兼容测试

这是最值得先写的测试，因为它能提前暴露 90% 的网关不兼容问题：

1. TS 侧：用 `axios-mock-adapter` 拦截请求，捕获生成客户端发出的 URL。
2. Go 侧：把同一批 URL 的 query string 喂给 `runtime.PopulateQueryParameters`，断言解码后的 proto 与预期一致。
3. 两边共用一份 JSON fixture，避免规则漂移。

覆盖：

- 嵌套 message、repeated、map、enum、bool、int64、Timestamp、Duration、FieldMask；
- 0 / false / "" 必须被保留；
- path/body 字段必须从 query 中排除；
- `body: "*"` 不产生 query；
- 多段 path 变量 `{name=shippers/*}`。

### 5.2 TypeScript 编译矩阵

至少覆盖：

```json
{
  "strict": true,
  "exactOptionalPropertyTypes": true,
  "noUncheckedIndexedAccess": true,
  "verbatimModuleSyntax": true,
  "module": "NodeNext",
  "moduleResolution": "NodeNext"
}
```

并跑一次 `import_extension=.js` 的场景。

### 5.3 类型断言测试

用 `expect-type` / `tsd` 断言：

- `GetUserRequest` 的字段可选性；
- `response_body` 的返回类型；
- oneof 的互斥性；
- `useGetUser` 的 `data` 类型；
- int64 模式切换后的类型。

### 5.4 Bazel 测试

除了 `bazel build`，建议加：

- 增量构建（改一个 proto，只重跑相关 action）；
- 沙箱构建（不依赖 PATH）；
- 远程缓存（同输入两次构建命中）；
- 输出声明完整性（所有生成文件都在 `DefaultInfo` 里）；
- `ts_project` 消费生成文件并编译。

---

## 6. v1.md 的具体问题清单

| 位置 | 问题 | 建议 |
| --- | --- | --- |
| §3.4 | 输出示例里出现 `user_connect.ts` | 应为 `_api.ts` / `_react_query.ts`，疑似从 Connect 文档复制 |
| §3.5 | 类型清单缺 bytes、wrappers、Any、Empty、NullValue、map key、nested naming | 按 §2.4 补全 |
| §4.5 | 方法没有 `config` 参数 | 加 `AxiosRequestConfig`，支持取消 |
| §4.6 | `params: request` | 改为显式 `URLSearchParams` 构造 |
| §5.5 | `detail(request.id)` 只包含 id | query key 应包含完整 request |
| §5.6 | hook 没有 options / enabled / signal | 按 §3.7 补 |
| §6.2 | `&protogen.Plugin{}` 非法 | 用 `protogen.Options.New(req)` |
| §7.2 | `buf.gen.yaml` 没有 `opt` | 补共享契约参数示例 |
| §8.1 | “不依赖本地安装的 protoc” 表述模糊 | 改成“不依赖系统 PATH 的 protoc” |
| §8.6 | 串行依赖与“不需要读取彼此输出”矛盾 | 改成三个 action 并行，聚合 target 统一依赖 |
| §9 | `output_suffix`、`import_extension` 语义不清 | 改成三插件共享的 `types_suffix`/`api_suffix`/`query_suffix`/`import_extension` |
| §10.3 | 集成流程画成串行生成 | 生成是并行的，编译才需要三者齐全 |
| §12 | out of scope 没提 streaming、proto2/editions | 明确 v1 只支持 unary + proto3 |
| §13 | 示例 import 路径固定 | 说明 `import_extension` 和后缀契约的影响 |

---

## 7. 建议的实施顺序

v1 的 Phase 1→4 顺序基本正确，但建议在 Phase 1 里先做“契约 + 兼容性验证”，再写模板：

1. **Phase 0：契约与语义验证（1–2 天）**
   - 定义 IR、命名契约、文件后缀、import 规则；
   - 实现 `httprule` 解析 + path/query 字段计算；
   - 写一个最小的 Go 测试，用 `runtime.PopulateQueryParameters` 验证编码规则；
   - 写一个包含 path/query/body/response_body/additional_bindings 的 fixture proto。

2. **Phase 1：TypeScript 类型插件**
   - 类型映射、WKT、oneof、optional、map、嵌套类型；
   - 跨文件 namespace import；
   - golden tests + tsc strict 编译。

3. **Phase 2：Axios 插件**
   - path/query/body/response_body/additional_bindings；
   - `config` 透传与取消；
   - TS/Go 双向 query 兼容测试；
   - 跳过无 http rule / streaming 的方法。

4. **Phase 3：React Query 插件**
   - key 工厂、useQuery/useMutation、request optional、enabled、options、signal；
   - hook 命名冲突检测；
   - 与 Axios 插件的命名契约测试。

5. **Phase 4：Bazel**
   - toolchain、ProtoInfo、并行 action、输出声明；
   - 自定义后缀 + `import_extension` 的集成测试；
   - 沙箱/远程缓存验证。

Phase 0 的价值在于：v1 文档最后一段已经指出“最需要提前验证的是 `google.api.http` 到 Axios 请求参数的完整映射，以及三个插件之间的文件命名和 import 约定”。这个验证不应该等 Phase 2 做完才做。

---

## 8. 需要拍板的决策

以下是会影响生成代码 API、后期难以更改的选择。建议按右列默认值推进：

| 决策 | 建议 |
| --- | --- |
| 输出粒度 | 每个 proto 三个文件（v1 原方案），跨文件用 namespace import |
| `additional_bindings` | 默认只生成 primary；`additional_bindings=methods` 时生成 `MethodBinding2...` |
| oneof | 互斥联合类型（`{a: A; b?: never} \| ...`） |
| int64 / bytes 默认 | `string` / `string`（与 protojson 一致）；`number` 可选；`bigint` 延后 |
| 字段可选性 | 所有 singular 字段 `?:`；不区分 proto3 optional（除非后续引入 field_behavior） |
| 流式 | v1 只做 unary；streaming 默认跳过，`streaming=error` 可强制报错 |
| React Query 注入 | 每次调用传入 `client`（v1 原方案）；后续可加 Provider 版本 |
| Query key | 每个 method 一个 key，包含完整 request；service 作用域前缀 |
| 文件后缀 | `_pb.ts` / `_api.ts` / `_react_query.ts`，三插件共享契约参数 |

---

## 9. 一句话建议

v1 的能力划分可以保留，但请把“**生成契约**”（文件名、字段命名、import 规则、类型映射）和“**grpc-gateway 语义**”（path 编码、query 编码、body/response_body 过滤）作为两个独立的、先于模板实现的基础模块，并先用网关自己的 `PopulateQueryParameters` 做兼容性测试。这样后面的 Axios、React Query、Bazel 都不会建立在不稳定的生成协议上。
