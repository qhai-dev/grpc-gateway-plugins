// The create form: the generated mutation hook plus local form state.
//
// The request body type is the generated CreateBookRequest, so a field that the proto
// does not declare cannot be added by accident, and a field whose type is wrong is a
// compile error rather than a runtime 400.

import { useState } from "react";

import type { CreateBookRequest } from "@/api/books";
import { useCreateBookAndRefresh } from "@/hooks/books";

type Props = {
  shelfId: string;
  onCreated: (bookId: string) => void;
};

export function CreateBookForm({ shelfId, onCreated }: Props) {
  const [title, setTitle] = useState("");
  const [author, setAuthor] = useState("");
  const [priceUnits, setPriceUnits] = useState("0");

  const mutation = useCreateBookAndRefresh(shelfId);

  function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();

    // The whole request is the body because the proto declares body: "*", so the
    // generated client sends exactly this object.
    const request: CreateBookRequest = {
      shelfId,
      title,
      author,
      // Money is a cross-file message; units is an int64 and therefore a string.
      price: { currencyCode: "USD", units: priceUnits, nanos: 0 },
    };

    mutation.mutate(request, {
      onSuccess: (response) => {
        const id = response.book?.id;
        if (id) onCreated(id);
        setTitle("");
        setAuthor("");
        setPriceUnits("0");
      },
    });
  }

  return (
    <section className="pane">
      <header className="pane__header">
        <h2>Add a book</h2>
      </header>

      <form className="form" onSubmit={handleSubmit}>
        <label>
          Title
          <input
            required
            value={title}
            onChange={(event) => setTitle(event.target.value)}
          />
        </label>

        <label>
          Author
          <input
            required
            value={author}
            onChange={(event) => setAuthor(event.target.value)}
          />
        </label>

        <label>
          Price (whole units)
          <input
            type="number"
            min="0"
            step="1"
            value={priceUnits}
            onChange={(event) => setPriceUnits(event.target.value)}
          />
        </label>

        <button type="submit" disabled={mutation.isPending}>
          {mutation.isPending ? "Saving…" : "Save"}
        </button>

        {mutation.isError ? (
          <p className="pane--error">{mutation.error.message}</p>
        ) : null}
        {mutation.isSuccess ? <p className="pane__hint">Saved.</p> : null}
      </form>
    </section>
  );
}
