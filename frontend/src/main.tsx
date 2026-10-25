import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "@/app/App";
import { ErrorBoundary } from "@/app/ErrorBoundary";
import { Providers } from "@/app/providers";
import "@/shared/styles/globals.css";

const root = document.getElementById("root");
if (!root) {
  throw new Error("#root element missing from index.html");
}

createRoot(root).render(
  <StrictMode>
    <Providers>
      <ErrorBoundary>
        <App />
      </ErrorBoundary>
    </Providers>
  </StrictMode>,
);
