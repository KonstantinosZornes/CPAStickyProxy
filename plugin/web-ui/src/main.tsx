import { createRoot } from "react-dom/client";
import { App } from "./App";
import { initThemeSync } from "./theme";

initThemeSync();
createRoot(document.getElementById("root")!).render(<App />);
