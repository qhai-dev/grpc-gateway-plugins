// The single place where the generated SDK is bound to the application.
//
// Nothing else in the app creates a client, so there is exactly one place to change
// when a second API host appears:
//
//   const adminClient = new BookstoreServiceClient(adminHttp);
//
// Every generated method takes an optional AxiosRequestConfig, so a call site can
// still pass a signal, extra headers or a timeout for one request.

import { http } from "@/lib/http";
import { BookstoreServiceClient } from "@/gen/bookstore/v1/bookstore_api.js";

export const bookstoreClient = new BookstoreServiceClient(http);

// Re-exported so components import types from one module rather than reaching into
// the generated tree.
export type {
  Book,
  BookFilter,
  BookFormat,
  CreateBookRequest,
  InPrint,
  ListBooksRequest,
  ListBooksResponse,
  OutOfPrint,
  SearchBooksRequest,
  UpdateBookRequest,
} from "@/gen/bookstore/v1/bookstore_type.js";

// Money lives in another proto file, so it is a separate module: this re-export is
// the point where the generated namespace import becomes a plain named one for the
// application.
export type { Money } from "@/gen/common/v1/money_type.js";

export { bookstoreServiceKeys } from "@/gen/bookstore/v1/bookstore_react_query.js";
