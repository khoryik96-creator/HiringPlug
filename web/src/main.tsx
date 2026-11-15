import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import "@fontsource-variable/inter";
import "@fontsource/barlow-condensed/latin-600.css";
import "@fontsource/barlow-condensed/latin-700.css";
import "@fontsource/barlow-condensed/latin-800.css";
import { App } from "./App";
import { guardNumberFields } from "./numericInput";
import { guardRequiredFields } from "./requiredFields";
import "./theme.css";
import "./styles.css";
import "./candidate-workspace.css";

guardNumberFields();
guardRequiredFields();

const root = document.getElementById("root");
if (!root) throw new Error("index.html has no #root element");
createRoot(root).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
