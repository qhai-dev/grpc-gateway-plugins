// Book hooks and mutations, produced by the React Query plugin.
//
// The generated module is imported straight through: there is no wrapper hook layer,
// because the generated hooks already do the two things a wrapper would add —
// client injection and query-key management.

import { useQueryClient } from "@tanstack/react-query";

import { bookstoreClient, bookstoreServiceKeys } from "@/api/books";
import {
  useCreateBook,
  useDeleteBook,
  useGetBook,
  useListBooks,
  useUpdateBook,
} from "@/gen/bookstore/v1/bookstore_react_query.js";

// useBook fetches one book. `request` is optional in the generated hook, so passing
// undefined while the selection is still loading leaves the query idle instead of
// firing a request with an empty id.
export function useBook(bookId: string | undefined) {
  return useGetBook(bookstoreClient, bookId ? { id: bookId } : undefined);
}

export function useBooks(request: {
  shelfId: string;
  page?: number;
  pageSize?: number;
  keyword?: string;
}) {
  return useListBooks(bookstoreClient, request);
}

// useCreateBook shows the one piece of business logic the generator leaves out on
// purpose: which cached queries a write invalidates. That is a product decision, so
// it lives here rather than in generated code.
export function useCreateBookAndRefresh(shelfId: string) {
  const queryClient = useQueryClient();

  return useCreateBook(bookstoreClient, {
    onSuccess: () => {
      // Invalidating the service-wide prefix clears every book query, which is the
      // coarse but always-correct choice.
      void queryClient.invalidateQueries({ queryKey: bookstoreServiceKeys.all });
    },
    onSettled: () => {
      void queryClient.invalidateQueries({
        queryKey: bookstoreServiceKeys.listBooks({ shelfId }),
      });
    },
  });
}

export function useUpdateBookAndRefresh() {
  const queryClient = useQueryClient();

  return useUpdateBook(bookstoreClient, {
    onSuccess: (response) => {
      // A response_body-unwrapped method still returns the message here, so the
      // updated id is available without a second request.
      void queryClient.invalidateQueries({ queryKey: bookstoreServiceKeys.all });
      return response;
    },
  });
}

export function useDeleteBookAndRefresh() {
  const queryClient = useQueryClient();

  return useDeleteBook(bookstoreClient, {
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: bookstoreServiceKeys.all });
    },
  });
}
