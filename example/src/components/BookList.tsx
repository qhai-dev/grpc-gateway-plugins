// The list pane: a paginated, filtered list of books.
//
// It calls the generated useListBooks hook directly. Notice what is absent: no URL,
// no useState for loading or error, no cache key, and no request type of its own.

import { useState } from "react";

import type { BookFormat } from "@/api/books";
import { useBooks } from "@/hooks/books";
import { describeAvailability } from "@/lib/format";

type Props = {
  shelfId: string;
  keyword: string;
  format: BookFormat | undefined;
  selectedBookId: string | undefined;
  onSelect: (bookId: string) => void;
};

export function BookList({ shelfId, keyword, format, selectedBookId, onSelect }: Props) {
  // Page is local UI state, not part of the request contract, so it is a plain
  // useState; the request object it feeds is rebuilt on every render and the
  // generated query key hashes it structurally.
  const [page, setPage] = useState(1);

  const query = useBooks({
    shelfId,
    page,
    pageSize: 10,
    // An empty keyword is still sent, which is what the generated client does with
    // "" — the gateway receives keyword= and treats it as the empty filter.
    keyword,
  });

  // The filter is applied on the client here only to show the enum query parameter
  // in the generated client's signature; a real app would send it as filter.format.
  const books = (query.data?.books ?? []).filter(
    (book) => format === undefined || book.format === format,
  );

  if (query.isPending) {
    return <section className="pane">Loading books…</section>;
  }

  if (query.isError) {
    return (
      <section className="pane pane--error">
        <p>Could not load books.</p>
        <pre>{query.error.message}</pre>
        <button type="button" onClick={() => void query.refetch()}>
          Retry
        </button>
      </section>
    );
  }

  return (
    <section className="pane">
      <header className="pane__header">
        <h2>Books</h2>
        <span className="pane__meta">
          {books.length} shown
          {query.data?.totalCount ? ` · ${query.data.totalCount} total` : ""}
          {query.isFetching ? " · refreshing" : ""}
        </span>
      </header>

      {books.length === 0 ? (
        <p className="pane__empty">No books match this filter.</p>
      ) : (
        <ul className="book-list">
          {books.map((book) => (
            <li key={book.id}>
              <button
                type="button"
                className={
                  book.id === selectedBookId ? "book-list__item is-selected" : "book-list__item"
                }
                onClick={() => {
                  if (book.id) onSelect(book.id);
                }}
              >
                <span className="book-list__title">{book.title ?? "(untitled)"}</span>
                <span className="book-list__author">{book.author}</span>
                <span className="book-list__meta">
                  {book.format?.replace("BOOK_FORMAT_", "").toLowerCase()} ·{" "}
                  {describeAvailability(book)}
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}

      <footer className="pane__footer">
        <button
          type="button"
          disabled={page <= 1}
          onClick={() => setPage((current) => Math.max(1, current - 1))}
        >
          Previous
        </button>
        <span>page {page}</span>
        <button
          type="button"
          disabled={!query.data?.nextPageToken}
          onClick={() => setPage((current) => current + 1)}
        >
          Next
        </button>
      </footer>
    </section>
  );
}
