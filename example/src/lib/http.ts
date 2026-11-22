// The axios instance, created once and configured by the application.
//
// This is the file the generated client deliberately does not provide: the SDK
// takes an AxiosInstance rather than creating one, so base URL, interceptors,
// credentials and error handling stay under the application's control.

import axios from "axios";

export const http = axios.create({
  // The generated URLs start with "/v1/...", and axios combines baseURL with a
  // leading-slash url, so this yields "/api/v1/...".
  baseURL: "/api",
  timeout: 10_000,
  headers: { "Content-Type": "application/json" },
});

// A request interceptor is where an application would attach a token.
http.interceptors.request.use((config) => {
  const token = localStorage.getItem("access_token");
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

// Axios rejects on a non-2xx response, which is the documented behaviour: the
// generated code does not swallow errors, so one interceptor decides what the UI
// sees for 401, 404, 5xx and network failures alike.
http.interceptors.response.use(
  (response) => response,
  (error) => {
    if (axios.isAxiosError(error)) {
      const status = error.response?.status;
      const detail = readProblemDetail(error.response?.data);
      // Rethrowing a plain Error keeps the message stable for the UI; the original
      // axios error is preserved as the cause.
      if (detail) {
        throw new Error(`${status ?? "network error"}: ${detail}`, { cause: error });
      }
      if (status === 401) {
        throw new Error("401: not authenticated", { cause: error });
      }
      throw new Error(
        status ? `${status}: request failed` : "network error: request failed",
        { cause: error },
      );
    }
    throw error;
  },
);

// readProblemDetail pulls a message out of common error bodies, including the
// shape grpc-gateway emits.
function readProblemDetail(data: unknown): string | undefined {
  if (typeof data !== "object" || data === null) return undefined;
  const record = data as Record<string, unknown>;
  for (const key of ["message", "error", "detail"]) {
    const value = record[key];
    if (typeof value === "string" && value !== "") return value;
  }
  return undefined;
}
