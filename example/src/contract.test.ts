// Contract tests for the example: they assert that the generated SDK and the wire
// format agree, from the TypeScript side.
//
// The point is not to re-test the plugins (each plugin has its own Go tests) but to
// prove that what the generated client sends is what grpc-gateway parses, using the
// example's own protos. A recording adapter captures the request, so no server is
// needed.

import { describe, expect, it } from "vitest";
import axios, { type AxiosAdapter, type InternalAxiosRequestConfig } from "axios";

import { BookstoreServiceClient } from "@/gen/bookstore/v1/bookstore_api.js";
import { bookstoreServiceKeys } from "@/gen/bookstore/v1/bookstore_react_query.js";

type Captured = {
  url: string;
  method: string;
  data: unknown;
};

function recordingClient(responseData: unknown) {
  const calls: Captured[] = [];
  const adapter: AxiosAdapter = async (config: InternalAxiosRequestConfig) => {
    calls.push({
      url: config.url ?? "",
      method: (config.method ?? "").toUpperCase(),
      data: config.data,
    });
    return {
      data: responseData,
      status: 200,
      statusText: "OK",
      headers: {},
      config,
    };
  };
  return { client: new BookstoreServiceClient(axios.create({ adapter })), calls };
}

/** Returns the query string of a captured URL as parsed parameters. */
function queryOf(url: string): URLSearchParams {
  const index = url.indexOf("?");
  return new URLSearchParams(index < 0 ? "" : url.slice(index + 1));
}

describe("path variables", () => {
  it("encodes a single-segment variable as one path token", async () => {
    const { client, calls } = recordingClient({});
    await client.getBook({ id: "a b/c" });
    // The slash belongs to the value of a single-segment variable, so it must be
    // escaped or the gateway would match a second path segment.
    expect(calls[0]?.url).toBe("/v1/books/a%20b%2Fc");
  });

  it("keeps a custom verb outside the variable", async () => {
    const { client, calls } = recordingClient({});
    await client.archiveBook({ id: "b1", reason: "damaged" });
    expect(calls[0]?.url).toBe("/v1/books/b1:archive");
    expect(calls[0]?.method).toBe("POST");
  });

  it("uses optional chaining for a nested path variable", async () => {
    const { client, calls } = recordingClient({});
    await client.updateBook({ book: { id: "b1", title: "x" } });
    expect(calls[0]?.url?.split("?")[0]).toBe("/v1/books/b1");
  });

  it("rejects a missing path variable before sending", async () => {
    const { client, calls } = recordingClient({});
    await expect(client.getBook({})).rejects.toThrow(
      "missing required path parameter: id",
    );
    expect(calls).toHaveLength(0);
  });

  it("treats an empty string as a present path value", async () => {
    // Only null/undefined count as missing; "" is a legitimate (if unusual) id, and
    // a truthiness check would have silently changed the URL.
    const { client, calls } = recordingClient({});
    await client.getBook({ id: "" });
    expect(calls[0]?.url).toBe("/v1/books/");
  });
});

describe("query parameters", () => {
  it("sends scalars, repeated values, maps and nested messages", async () => {
    const { client, calls } = recordingClient({});
    await client.listBooks({
      shelfId: "s1",
      page: 2,
      pageSize: 10,
      keyword: "grep",
      authorIds: ["a1", "a2"],
      labels: { lang: "en" },
      filter: { author: "Knuth", format: "BOOK_FORMAT_HARDCOVER", inStock: true },
      formats: ["BOOK_FORMAT_EBOOK"],
      minPriceUnits: "9007199254740993",
    });

    const query = queryOf(calls[0]?.url ?? "");
    expect(query.get("page")).toBe("2");
    expect(query.get("pageSize")).toBe("10");
    expect(query.getAll("authorIds")).toEqual(["a1", "a2"]);
    // Maps use the bracket form, which is what the gateway's query parser expects.
    expect(query.get("labels[lang]")).toBe("en");
    // A nested message is flattened into dotted names.
    expect(query.get("filter.author")).toBe("Knuth");
    expect(query.get("filter.inStock")).toBe("true");
    // An enum is sent by name.
    expect(query.getAll("formats")).toEqual(["BOOK_FORMAT_EBOOK"]);
    // int64 stays a decimal string, so 2^53+1 is not rounded.
    expect(query.get("minPriceUnits")).toBe("9007199254740993");
    // The path-bound shelf id is not repeated as a query parameter.
    expect(query.has("shelfId")).toBe(false);
  });

  it("sends zero values instead of dropping them", async () => {
    const { client, calls } = recordingClient({});
    await client.listBooks({
      shelfId: "s1",
      page: 0,
      includeOutOfPrint: false,
      keyword: "",
      filter: { inStock: false },
    });

    const query = queryOf(calls[0]?.url ?? "");
    // A truthiness test would have omitted all four of these.
    expect(query.get("page")).toBe("0");
    expect(query.get("includeOutOfPrint")).toBe("false");
    expect(query.get("keyword")).toBe("");
    expect(query.get("filter.inStock")).toBe("false");
  });

  it("omits the separator entirely when nothing is set", async () => {
    const { client, calls } = recordingClient({});
    await client.listBooks({ shelfId: "s1" });
    expect(calls[0]?.url).toBe("/v1/shelves/s1/books");
  });
});

describe("body mapping", () => {
  it("sends the whole request for body: *", async () => {
    const { client, calls } = recordingClient({});
    const request = { shelfId: "s1", title: "TAOCP", author: "Knuth" };
    await client.createBook(request);
    expect(JSON.parse(String(calls[0]?.data))).toEqual(request);
    // body: "*" means there is no query string at all.
    expect(calls[0]?.url).not.toContain("?");
  });

  it("sends only the nested field for body: book", async () => {
    const { client, calls } = recordingClient({});
    await client.updateBook({
      book: { id: "b1", title: "new" },
      updateMask: "title",
    });

    // The body is the book, not the wrapper request.
    expect(JSON.parse(String(calls[0]?.data))).toEqual({ id: "b1", title: "new" });
    // update_mask stays a query parameter because the body only covers book.
    expect(queryOf(calls[0]?.url ?? "").get("updateMask")).toBe("title");
  });
});

describe("response_body", () => {
  it("resolves to the field value, which is what the gateway sends", async () => {
    // response_body: "book" makes the gateway marshal the Book as the whole body, so
    // the client returns it directly rather than reading a `book` property.
    const book = { id: "b1", title: "TAOCP", author: "Knuth" };
    const { client } = recordingClient(book);
    await expect(client.getBook({ id: "b1" })).resolves.toEqual(book);
  });
});

describe("config passthrough", () => {
  it("cannot override url or method", async () => {
    const { client, calls } = recordingClient({});
    await client.getBook({ id: "b1" }, { url: "/hijacked", method: "POST" });
    expect(calls[0]?.url).toBe("/v1/books/b1");
    expect(calls[0]?.method).toBe("GET");
  });

  it("accepts an abort signal", async () => {
    const { client } = recordingClient({});
    const controller = new AbortController();
    controller.abort();
    // An already-aborted signal has to reach axios as a cancellation, not be ignored.
    await expect(client.getBook({ id: "b1" }, { signal: controller.signal })).rejects.toMatchObject({
      code: "ERR_CANCELED",
    });
  });
});

describe("query keys", () => {
  it("includes the whole request, so different filters differ", () => {
    const first = bookstoreServiceKeys.listBooks({ shelfId: "s1", keyword: "a" });
    const second = bookstoreServiceKeys.listBooks({ shelfId: "s1", keyword: "b" });
    expect(JSON.stringify(first)).not.toBe(JSON.stringify(second));
    expect(JSON.stringify(first)).toContain('"keyword":"a"');
  });

  it("shares a service-wide prefix for invalidation", () => {
    const key = bookstoreServiceKeys.getBook({ id: "b1" });
    // invalidateQueries({ queryKey: bookstoreServiceKeys.all }) relies on this.
    expect(key[0]).toBe(bookstoreServiceKeys.all[0]);
  });
});
