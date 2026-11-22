// A tiny mock of the grpc-gateway routes the example proto declares.
//
// It exists so the example runs without a Go backend: presets `npm run dev` to a
// server that answers the exact URLs the generated client produces. The important
// property is that it *decodes the request the way grpc-gateway does* — dotted query
// names, `name[key]=value` maps, repeated parameters, enum names, RFC3339 timestamps
// — because that is what proves the generated client and the gateway agree.
//
// It is not a general-purpose gateway. It implements only what bookstore.proto
// annotates, and it is deliberately explicit about where each parameter comes from.

import { createServer } from "node:http";

const PORT = Number(process.env.PORT ?? 8080);

// ---------------------------------------------------------------------------
// In-memory data
// ---------------------------------------------------------------------------

/** @type {Array<Record<string, unknown>>} */
let books = [
  {
    id: "b-1",
    title: "The Art of Computer Programming",
    author: "Donald Knuth",
    format: "BOOK_FORMAT_HARDCOVER",
    price: { currencyCode: "USD", units: "120", nanos: 0 },
    publishedAt: "1968-01-01T00:00:00Z",
    pageCount: 650,
    isbn13: "978-0201896831",
    inPrint: { stock: 12 },
    tags: ["classic", "algorithms"],
  },
  {
    id: "b-2",
    title: "Designing Data-Intensive Applications",
    author: "Martin Kleppmann",
    format: "BOOK_FORMAT_PAPERBACK",
    price: { currencyCode: "USD", units: "45", nanos: 500000000 },
    publishedAt: "2017-03-16T00:00:00Z",
    pageCount: 616,
    outOfPrint: { reason: "superseded by the second edition" },
    tags: ["distributed-systems"],
  },
  {
    id: "b-3",
    title: "Structure and Interpretation of Computer Programs",
    author: "Harold Abelson",
    format: "BOOK_FORMAT_EBOOK",
    price: { currencyCode: "USD", units: "0", nanos: 0 },
    publishedAt: "1985-07-25T00:00:00Z",
    pageCount: 657,
    // No availability member: this is the "oneof unset" case the generated union
    // now models, and it must survive a round trip.
    tags: [],
  },
];

let nextId = 4;

// ---------------------------------------------------------------------------
// Query decoding, mirroring runtime.PopulateQueryParameters
// ---------------------------------------------------------------------------

/**
 * Parses a query string into grpc-gateway's view of it.
 *
 * A dotted name addresses a nested field, and `name[key]` addresses a map entry whose
 * key may contain any character. Both must be read before anything else, because the
 * dot and the bracket are structural, not part of a field name.
 */
function decodeQuery(url) {
  /** @type {Map<string, string[]>} */
  const params = new URLSearchParams(url.search);

  /** @type {{ scalars: Map<string, string[]>, maps: Map<string, Map<string, string>> }} */
  const out = { scalars: new Map(), maps: new Map() };

  for (const [rawKey, value] of params.entries()) {
    const mapMatch = /^(.*)\[([^\]]*)\]$/.exec(rawKey);
    if (mapMatch) {
      const [, field, key] = mapMatch;
      if (field === undefined || key === undefined) continue;
      const entries = out.maps.get(field) ?? new Map();
      entries.set(key, value);
      out.maps.set(field, entries);
      continue;
    }
    const values = out.scalars.get(rawKey) ?? [];
    values.push(value);
    out.scalars.set(rawKey, values);
  }
  return out;
}

function scalar(query, name) {
  return query.scalars.get(name)?.[0];
}

function repeated(query, name) {
  return query.scalars.get(name) ?? [];
}

function integer(query, name) {
  const raw = scalar(query, name);
  if (raw === undefined) return undefined;
  const parsed = Number.parseInt(raw, 10);
  if (Number.isNaN(parsed)) {
    throw new HttpError(400, `invalid integer for ${name}: ${raw}`);
  }
  return parsed;
}

function boolean(query, name) {
  const raw = scalar(query, name);
  if (raw === undefined) return undefined;
  if (raw === "true") return true;
  if (raw === "false") return false;
  throw new HttpError(400, `invalid boolean for ${name}: ${raw}`);
}

class HttpError extends Error {
  constructor(status, message) {
    super(message);
    this.status = status;
  }
}

// ---------------------------------------------------------------------------
// Route handlers
// ---------------------------------------------------------------------------

function getBook(id) {
  const book = books.find((candidate) => candidate.id === id);
  if (!book) throw new HttpError(404, `book ${id} not found`);
  return book;
}

function listBooks(url) {
  const query = decodeQuery(url);

  // Every filter is read from the form the generated client emits, so a change in
  // that form breaks this server loudly rather than silently returning everything.
  const page = integer(query, "page") ?? 1;
  const pageSize = integer(query, "pageSize") ?? 10;
  const keyword = scalar(query, "keyword");
  const authorIds = repeated(query, "authorIds");
  const formats = repeated(query, "formats");
  const includeOutOfPrint = boolean(query, "includeOutOfPrint");
  const minPriceUnits = scalar(query, "minPriceUnits");
  const filterAuthor = scalar(query, "filter.author");
  const filterFormat = scalar(query, "filter.format");
  const labels = query.maps.get("labels") ?? new Map();

  // The path-bound shelf id arrives as a path segment, never as a query parameter.
  if (scalar(query, "shelfId") !== undefined) {
    throw new HttpError(400, "shelfId must be bound from the path, not the query");
  }

  let matches = books.filter((book) => {
    if (keyword !== undefined && keyword !== "") {
      const haystack = `${book.title ?? ""} ${book.author ?? ""}`.toLowerCase();
      if (!haystack.includes(keyword.toLowerCase())) return false;
    }
    if (authorIds.length > 0) {
      const wanted = new Set(authorIds);
      if (!wanted.has(String(book.author))) return false;
    }
    if (formats.length > 0 && !formats.includes(String(book.format))) return false;
    if (filterFormat !== undefined && book.format !== filterFormat) return false;
    if (filterAuthor !== undefined && book.author !== filterAuthor) return false;
    if (includeOutOfPrint === false && "outOfPrint" in book) return false;
    if (minPriceUnits !== undefined) {
      // int64 arrives as a decimal string, so the comparison is on BigInt and never
      // rounds a value past 2^53.
      const price = /** @type {{ units?: string }} */ (book.price ?? {});
      if (BigInt(price.units ?? "0") < BigInt(minPriceUnits)) return false;
    }
    if (labels.size > 0) {
      const tags = new Set(/** @type {string[]} */ (book.tags ?? []));
      for (const [key, value] of labels) {
        if (!tags.has(value) && !tags.has(key)) return false;
      }
    }
    return true;
  });

  const start = (page - 1) * pageSize;
  matches = matches.slice(start, start + pageSize);

  return {
    books: matches,
    nextPageToken: start + pageSize < books.length ? String(page + 1) : "",
    // total_count is an int64, which protojson emits as a string.
    totalCount: String(books.length),
  };
}

function createBook(body) {
  if (!body || typeof body !== "object") {
    throw new HttpError(400, "body must be an object");
  }
  if (typeof body.title !== "string" || body.title === "") {
    throw new HttpError(400, "title is required");
  }
  const book = {
    id: `b-${nextId++}`,
    title: body.title,
    author: body.author ?? "",
    format: body.format ?? "BOOK_FORMAT_UNSPECIFIED",
    price: body.price ?? { currencyCode: "USD", units: "0", nanos: 0 },
    publishedAt: new Date().toISOString(),
    pageCount: 0,
    inPrint: { stock: 1 },
    tags: [],
  };
  books = [...books, book];
  return { book };
}

function updateBook(id, body) {
  const existing = getBook(id);
  // body: "book" means the body is the book itself, and update_mask arrives as a
  // query parameter. Only the masked fields are applied.
  const index = books.indexOf(existing);
  const updated = { ...existing, ...body, id: existing.id };
  books = books.map((candidate, i) => (i === index ? updated : candidate));
  return { book: updated };
}

// ---------------------------------------------------------------------------
// Server
// ---------------------------------------------------------------------------

const routes = [
  { method: "GET", pattern: /^\/v1\/books\/([^/]+)\/?$/, handler: (m, _url, _body) => getBook(decodeURIComponent(m[1])) },
  { method: "GET", pattern: /^\/v1\/shelves\/([^/]+)\/books\/?$/, handler: (_m, url) => listBooks(url) },
  { method: "POST", pattern: /^\/v1\/shelves\/([^/]+)\/books\/?$/, handler: (_m, _url, body) => createBook(body) },
  { method: "PATCH", pattern: /^\/v1\/books\/([^/]+)\/?$/, handler: (m, _url, body) => updateBook(decodeURIComponent(m[1]), body) },
  { method: "DELETE", pattern: /^\/v1\/books\/([^/]+)\/?$/, handler: (m) => {
    const book = getBook(decodeURIComponent(m[1]));
    books = books.filter((candidate) => candidate.id !== book.id);
    return {};
  } },
  { method: "POST", pattern: /^\/v1\/books:search\/?$/, handler: (_m, _url, body) => {
    const query = String(body?.query ?? "").toLowerCase();
    const found = books.filter((book) => JSON.stringify(book).toLowerCase().includes(query));
    return { books: found, matched: String(found.length) };
  } },
  { method: "POST", pattern: /^\/v1\/books\/([^/]+):archive\/?$/, handler: (m) => {
    const book = getBook(decodeURIComponent(m[1]));
    return { book };
  } },
  { method: "POST", pattern: /^\/v1\/books:import\/?$/, handler: (_m, _url, body) => ({
    imported: String(body?.csv ?? "").length > 0 ? 1 : 0,
    errors: [],
  }) },
  { method: "POST", pattern: /^\/v1\/legacy\/books:import\/?$/, handler: () => ({ imported: 0, errors: ["legacy route"] }) },
];

function readBody(req) {
  return new Promise((resolve, reject) => {
    const chunks = [];
    req.on("data", (chunk) => chunks.push(chunk));
    req.on("end", () => {
      const raw = Buffer.concat(chunks).toString("utf8");
      if (raw === "") return resolve(undefined);
      try {
        resolve(JSON.parse(raw));
      } catch (error) {
        reject(new HttpError(400, `invalid JSON body: ${String(error)}`));
      }
    });
    req.on("error", reject);
  });
}

const server = createServer(async (req, res) => {
  // The example's axios instance prefixes every URL with "/api", so the mock strips
  // it again. A real deployment would let grpc-gateway's own mux handle this.
  const rawUrl = req.url ?? "/";
  const url = new URL(rawUrl.replace(/^\/api/, ""), `http://${req.headers.host ?? "localhost"}`);

  const send = (status, payload) => {
    const body = JSON.stringify(payload);
    res.writeHead(status, {
      "Content-Type": "application/json",
      "Content-Length": Buffer.byteLength(body),
    });
    res.end(body);
  };

  // grpc-gateway answers CORS preflights, and so must a mock used from a browser.
  if (req.method === "OPTIONS") {
    res.writeHead(204, {
      "Access-Control-Allow-Origin": "*",
      "Access-Control-Allow-Methods": "GET,POST,PUT,PATCH,DELETE,OPTIONS",
      "Access-Control-Allow-Headers": "Content-Type,Authorization",
    });
    res.end();
    return;
  }

  try {
    for (const route of routes) {
      if (route.method !== req.method) continue;
      const match = route.pattern.exec(url.pathname);
      if (!match) continue;

      const body = req.method === "GET" || req.method === "DELETE" ? undefined : await readBody(req);
      const result = route.handler(match, url, body);
      send(200, result);
      return;
    }
    send(404, { code: 5, message: `no route for ${req.method} ${url.pathname}` });
  } catch (error) {
    if (error instanceof HttpError) {
      send(error.status, { code: error.status, message: error.message });
      return;
    }
    send(500, { code: 13, message: String(error) });
  }
});

server.listen(PORT, () => {
  process.stdout.write(`mock grpc-gateway listening on http://localhost:${PORT}\n`);
  process.stdout.write(`routes: ${routes.length}\n`);
});
