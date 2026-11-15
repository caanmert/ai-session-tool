import React from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import "./themes.css";
import "./style.css";
import "./ui/controls.css";
import "./ui/markdown.css";
import "./shell/shell.css";
import "./chat/chat.css";
import "./chat/details.css";
import "./settings/settings.css";
import "./history/history.css";
import "./board/board.css";
createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
