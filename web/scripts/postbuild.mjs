import fs from "node:fs";

// Ensure `web/dist/.keep` exists so `//go:embed dist/*` always has at least one file
// even when the repo is checked out without building the UI.
fs.mkdirSync("dist", { recursive: true });
fs.writeFileSync("dist/.keep", "");
