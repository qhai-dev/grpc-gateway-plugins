// Application entry point: mounts the app and provides the QueryClient.
//
// A single QueryClient per application is the React Query convention, and the
// generated hooks read it from context, so nothing in the SDK needs to know about it.

import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ReactQueryDevtools } from "@tanstack/react-query-devtools";

import { App } from "@/components/App";
import "./index.css";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      // Retrying a 404 or a 401 is pointless, and grpc-gateway returns real status
      // codes, so the decision can be made from the error the interceptor threw.
      retry: (failureCount, error) => {
        const message = error instanceof Error ? error.message : "";
        if (message.startsWith("401:") || message.startsWith("404:")) return false;
        return failureCount < 2;
      },
      staleTime: 30_000,
      refetchOnWindowFocus: false,
    },
  },
});

const container = document.getElementById("root");
if (!container) {
  throw new Error("#root element is missing from index.html");
}

createRoot(container).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <App />
      <ReactQueryDevtools initialIsOpen={false} />
    </QueryClientProvider>
  </StrictMode>,
);
