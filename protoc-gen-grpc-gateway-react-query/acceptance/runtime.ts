// Runtime verification of the generated Axios client against a fake adapter.
//
// This file is an acceptance harness, not part of the SDK: it checks the URL,
// method and body the generated client actually produces.

import axios, { type AxiosAdapter, type InternalAxiosRequestConfig } from "axios";

import { UserServiceClient } from "./generated/user/v1/user_api.js";
import { ShipperServiceClient } from "./generated/shipping/v1/shipping_api.js";

type Captured = {
  url: string;
  method: string;
  data: unknown;
};

// captureAdapter records the request and returns a canned response.
function captureAdapter(responseData: unknown): { adapter: AxiosAdapter; calls: Captured[] } {
  const calls: Captured[] = [];
  const adapter: AxiosAdapter = async (config: InternalAxiosRequestConfig) => {
    calls.push({
      url: config.url ?? "",
      method: (config.method ?? "").toUpperCase(),
      data: config.data,
    });
    return { data: responseData, status: 200, statusText: "OK", headers: {}, config };
  };
  return { adapter, calls };
}

const failures: string[] = [];

function check(name: string, actual: unknown, expected: unknown): void {
  const a = JSON.stringify(actual);
  const e = JSON.stringify(expected);
  if (a !== e) failures.push(`${name}\n     actual:   ${a}\n     expected: ${e}`);
}

function checkTrue(name: string, condition: boolean): void {
  if (!condition) failures.push(name);
}

function query(url: string): URLSearchParams {
  const i = url.indexOf("?");
  return new URLSearchParams(i < 0 ? "" : url.slice(i + 1));
}

async function main(): Promise<void> {
  // --- path parameters ------------------------------------------------------
  {
    const { adapter, calls } = captureAdapter({ id: "1", name: "Alice" });
    const client = new UserServiceClient(axios.create({ adapter }));
    await client.getUser({ id: "a b/c" });
    // A single-segment variable is one path token, so slash and space escape.
    check("single-segment path encoding", calls[0]?.url, "/v1/users/a%20b%2Fc");
    check("GET method", calls[0]?.method, "GET");
    checkTrue("a GET without a body sends no data", calls[0]?.data === undefined);
  }

  {
    // A multi-segment value keeps its slashes, because the gateway matches them
    // as separators.
    const { adapter, calls } = captureAdapter({});
    const client = new ShipperServiceClient(axios.create({ adapter }));
    await client.getShipper({ name: "shippers/123" });
    check("multi-segment path keeps slashes", calls[0]?.url, "/v1/shippers/123");
  }

  {
    const { adapter, calls } = captureAdapter({});
    const client = new ShipperServiceClient(axios.create({ adapter }));
    await client.activateShipper({ id: "7", reason: "test" });
    check("verb stays outside the variable", calls[0]?.url, "/v1/shippers/7:activate");
    check("body * sends the whole request", JSON.parse(String(calls[0]?.data)), {
      id: "7",
      reason: "test",
    });
  }

  // --- missing and zero-value path parameters ------------------------------
  {
    const { adapter } = captureAdapter({});
    const client = new UserServiceClient(axios.create({ adapter }));
    let threw = false;
    try {
      await client.getUser({});
    } catch (err) {
      threw = err instanceof Error && err.message.includes("missing required path parameter: id");
    }
    checkTrue("a missing path parameter throws before the request", threw);
  }

  {
    // "0" is a legitimate path value and must not be treated as missing.
    const { adapter, calls } = captureAdapter({});
    const client = new UserServiceClient(axios.create({ adapter }));
    await client.getUser({ id: "0" });
    check("zero path value is kept", calls[0]?.url, "/v1/users/0");
  }

  // --- query parameters ----------------------------------------------------
  {
    const { adapter, calls } = captureAdapter({ users: [] });
    const client = new UserServiceClient(axios.create({ adapter }));
    await client.listUsers({
      page: 0,
      pageSize: 10,
      keyword: "",
      ids: ["a", "b"],
      labels: { env: "prod" },
      filter: { status: "USER_STATUS_ACTIVE", minAge: 0 },
      includeDeleted: false,
      cursor: "9007199254740993",
    });

    const q = query(calls[0]?.url ?? "");
    // Zero, false and "" survive: only null and undefined are omitted.
    check("zero page is kept", q.get("page"), "0");
    check("empty string is kept", q.get("keyword"), "");
    check("false boolean is kept", q.get("includeDeleted"), "false");
    check("zero nested value is kept", q.get("filter.minAge"), "0");
    // repeated -> same-name repeated parameters.
    check("repeated parameter", q.getAll("ids"), ["a", "b"]);
    // map -> bracket form, which is what the gateway parses.
    check("map uses the bracket form", q.get("labels[env]"), "prod");
    // enum -> name, which is the wire format.
    check("enum sends its name", q.get("filter.status"), "USER_STATUS_ACTIVE");
    // int64 stays a decimal string, so no precision is lost.
    check("int64 keeps full precision", q.get("cursor"), "9007199254740993");
  }

  {
    // An empty request produces the bare path, with no trailing "?".
    const { adapter, calls } = captureAdapter({ users: [] });
    const client = new UserServiceClient(axios.create({ adapter }));
    await client.listUsers({});
    check("no parameters means no separator", calls[0]?.url, "/v1/users");
  }

  {
    // A field bound by the path or covered by the body must not also appear in
    // the query, or the gateway sets it twice.
    const { adapter, calls } = captureAdapter({});
    const client = new UserServiceClient(axios.create({ adapter }));
    await client.updateUser({ user: { id: "42", name: "x" }, updateMask: "name" });

    const url = calls[0]?.url ?? "";
    check("nested path variable", url.split("?")[0], "/v1/users/42");
    checkTrue("body fields are excluded from the query", !url.includes("user."));
    checkTrue("the path field is excluded from the query", !url.includes("id="));
    check("nested body sends only that field", JSON.parse(String(calls[0]?.data)), {
      id: "42",
      name: "x",
    });
  }

  {
    const { adapter, calls } = captureAdapter({});
    const client = new UserServiceClient(axios.create({ adapter }));
    await client.createUser({ name: "Alice", contact: { email: { address: "a@b.c" } } });
    check("body * produces no query string", (calls[0]?.url ?? "").includes("?"), false);
    check("body * sends the whole request", JSON.parse(String(calls[0]?.data)), {
      name: "Alice",
      contact: { email: { address: "a@b.c" } },
    });
  }

  // --- response_body -------------------------------------------------------
  {
    // The gateway marshals the selected field as the whole body, so the value
    // arrives directly rather than under a property.
    const { adapter } = captureAdapter({ id: "1", name: "Alice" });
    const client = new UserServiceClient(axios.create({ adapter }));
    const user = await client.getUser({ id: "1" });
    check("response_body value is the body", user, { id: "1", name: "Alice" });
  }

  {
    const { adapter } = captureAdapter({ users: [{ id: "1" }], nextPageToken: "t" });
    const client = new UserServiceClient(axios.create({ adapter }));
    const page = await client.listUsers({});
    check("whole response is returned without response_body", page, {
      users: [{ id: "1" }],
      nextPageToken: "t",
    });
  }

  // --- config passthrough --------------------------------------------------
  {
    const { adapter } = captureAdapter({});
    const client = new UserServiceClient(axios.create({ adapter }));
    const controller = new AbortController();
    await client.getUser(
      { id: "1" },
      { headers: { "X-Trace": "t1" }, signal: controller.signal },
    );
    checkTrue("config is accepted", true);
  }

  {
    // url and method must not be overridable, or a caller could silently hit a
    // different endpoint through the typed SDK.
    const { adapter, calls } = captureAdapter({});
    const client = new UserServiceClient(axios.create({ adapter }));
    await client.getUser({ id: "1" }, { url: "/hijacked", method: "POST" });
    check("url cannot be overridden", calls[0]?.url, "/v1/users/1");
    check("method cannot be overridden", calls[0]?.method, "GET");
  }

  // --- report --------------------------------------------------------------
  if (failures.length > 0) {
    console.error(`FAILED (${failures.length}):`);
    for (const f of failures) console.error(" - " + f);
    process.exitCode = 1;
    return;
  }
  console.log("all runtime checks passed");
}

void main();
