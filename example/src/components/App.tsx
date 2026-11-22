// The example UI: a shelf of books with filtering, selection and a create form.
//
// It is deliberately small. The point of the example is the layer between the proto
// contract and the component: types come from the generated `_type.ts`, requests go
// through the generated client, and cache state comes from the generated hooks. The
// components below contain no URLs, no fetch calls and no hand-written types.

import { useState } from "react";

import { formatMoney } from "@/lib/format";
import { BookDetail } from "@/components/BookDetail";
import { BookList } from "@/components/BookList";
import { CreateBookForm } from "@/components/CreateBookForm";

const SHELF_ID = "shelf-1";

// The enum is generated as a string literal union by default, so its members are
// plain strings rather than a runtime object. The values are the proto enum names,
// which is exactly what protojson puts on the wire.
const FORMATS = ["BOOK_FORMAT_HARDCOVER", "BOOK_FORMAT_PAPERBACK", "BOOK_FORMAT_EBOOK"] as const;

export function App() {
  const [selectedBookId, setSelectedBookId] = useState<string | undefined>();
  const [keyword, setKeyword] = useState("");
  const [format, setFormat] = useState<(typeof FORMATS)[number] | undefined>();

  return (
    <main className="app">
      <header className="app__header">
        <h1>Bookstore</h1>
        <p className="app__subtitle">
          Generated TypeScript SDK · Axios client · TanStack Query hooks
        </p>
      </header>

      <section className="app__controls">
        <label>
          Search
          <input
            type="search"
            value={keyword}
            placeholder="title or author"
            onChange={(event) => setKeyword(event.target.value)}
          />
        </label>

        <label>
          Format
          <select
            value={format ?? ""}
            onChange={(event) => {
              const value = event.target.value;
              // The empty option means "no filter" rather than the zero enum value,
              // so it becomes undefined and the query parameter is omitted entirely.
              setFormat(value === "" ? undefined : (value as (typeof FORMATS)[number]));
            }}
          >
            <option value="">any</option>
            {FORMATS.map((value) => (
              <option key={value} value={value}>
                {value.replace("BOOK_FORMAT_", "").toLowerCase()}
              </option>
            ))}
          </select>
        </label>
      </section>

      <div className="app__panes">
        <BookList
          shelfId={SHELF_ID}
          keyword={keyword}
          format={format}
          selectedBookId={selectedBookId}
          onSelect={setSelectedBookId}
        />

        <div className="app__side">
          {selectedBookId ? (
            <BookDetail
              bookId={selectedBookId}
              onClose={() => setSelectedBookId(undefined)}
            />
          ) : (
            <p className="app__hint">Select a book to see its details.</p>
          )}

          <CreateBookForm
            shelfId={SHELF_ID}
            onCreated={(bookId) => setSelectedBookId(bookId)}
          />
        </div>
      </div>

      <footer className="app__footer">
        <p>
          Money amounts are rendered from the cross-file{" "}
          <code>common.v1.Money</code> type:{" "}
          {formatMoney({ currencyCode: "USD", units: "12", nanos: 990_000_000 })}
        </p>
      </footer>
    </main>
  );
}
