import React from "react";
import ReactDOM from "react-dom/client";
import { App } from "./ui/App";
import "./ui/styles.css";

const savedTheme = window.localStorage.getItem("forge-drop-theme");
if (savedTheme === "dark" || savedTheme === "light") {
  document.documentElement.setAttribute("data-theme", savedTheme);
} else {
  document.documentElement.setAttribute("data-theme", "light");
}

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);
