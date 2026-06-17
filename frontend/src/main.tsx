import ReactDOM from "react-dom/client";
import { RouterProvider, createRouter } from "@tanstack/react-router";
import { routeTree } from "./routeTree.gen";
import {
  MutationCache,
  QueryCache,
  QueryClient,
  QueryClientProvider,
} from "@tanstack/react-query";
import { ThemeProvider } from "next-themes";
import { toast } from "sonner";
const router = createRouter({
  routeTree,
  defaultPreload: "intent",
  scrollRestoration: true,
});

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}

const rootElement = document.getElementById("app")!;
const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 1000 * 60 * 5, // 5 minutes: keep cache fresh to avoid immediate refetching
      refetchOnWindowFocus: false, // disable automatic refetch on window focus
      refetchOnReconnect: false,  // disable automatic refetch on network reconnect
    },
  },
  queryCache: new QueryCache({
    onError: (error: Error) => {
      toast.error(error.message);
    },
  }),
  mutationCache: new MutationCache({
    onError: (error: Error) => {
      toast.error(error.message);
    },
    onSuccess: (_data, _variables, _context, mutation) => {
      const msg = (mutation.meta as Record<string, unknown>)?.successMessage;
      if (typeof msg === "string") toast.success(msg);
    },
  }),
});
if (!rootElement.innerHTML) {
  const root = ReactDOM.createRoot(rootElement);
  root.render(
    <QueryClientProvider client={queryClient}>
      <ThemeProvider
        attribute="class"
        defaultTheme="light"
        enableSystem={false}
      >
        <RouterProvider router={router} />
      </ThemeProvider>
    </QueryClientProvider>,
  );
}
