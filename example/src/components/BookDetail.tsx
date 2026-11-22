// The detail pane: one book, fetched through the generated hook.
//
// The hook is called with `undefined` when no id is selected, which is what the
// generated `enabled` default is for: the query stays idle instead of firing a
// request for an empty id.

import { useBook } from "@/hooks/books";
import { describeAvailability, formatMoney, formatTimestamp } from "@/lib/format";

type Props = {
  bookId: string;
  onClose: () => void;
};

export function BookDetail({ bookId, onClose }: Props) {
  const query = useBook(bookId);

  return (
    <section className="pane">
      <header className="pane__header">
        <h2>Details</h2>
        <button type="button" onClick={onClose}>
          Close
        </button>
      </header>

      {query.isPending ? <p>Loading…</p> : null}

      {query.isError ? (
        <div className="pane--error">
          <p>Could not load book {bookId}.</p>
          <pre>{query.error.message}</pre>
        </div>
      ) : null}

      {query.data ? (
        <dl className="detail">
          <dt>Title</dt>
          <dd>{query.data.title ?? "(untitled)"}</dd>

          <dt>Author</dt>
          <dd>{query.data.author ?? "—"}</dd>

          <dt>Format</dt>
          <dd>{query.data.format ?? "—"}</dd>

          <dt>Price</dt>
          {/* Money is a cross-file type, imported from common/v1/money.proto. */}
          <dd>{formatMoney(query.data.price)}</dd>

          <dt>Published</dt>
          <dd>{formatTimestamp(query.data.publishedAt)}</dd>

          <dt>Pages</dt>
          <dd>{query.data.pageCount ?? "—"}</dd>

          <dt>ISBN-13</dt>
          {/* A proto3 `optional` field: still a plain optional property, not a union. */}
          <dd>{query.data.isbn13 ?? "not recorded"}</dd>

          <dt>Availability</dt>
          {/* A oneof: exactly one arm is set, so the helper can narrow safely. */}
          <dd>{describeAvailability(query.data)}</dd>

          <dt>Tags</dt>
          <dd>
            {(query.data.tags ?? []).length === 0
              ? "—"
              : (query.data.tags ?? []).join(", ")}
          </dd>
        </dl>
      ) : null}
    </section>
  );
}
