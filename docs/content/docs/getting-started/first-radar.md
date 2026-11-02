---
title: "Generate a first radar"
weight: 20
---

# Generate a first radar

This walkthrough creates a small React application, scans it, and produces an HTML radar. You do not need Node, npm, a running application, or access to GitHub. The scanner reads the files without executing them.

Run these commands in a POSIX shell from the tech-radar project root, after [building the CLI](installation.md). Keep the same shell open so `$radar_demo` remains available.

## 1. Create a repository to inspect

Create separate directories for the application and the generated report:

```sh
radar_demo=$(mktemp -d)
mkdir -p "$radar_demo/app/src" "$radar_demo/reports"
```

Write the dependency manifest:

```sh
cat > "$radar_demo/app/package.json" <<'APP'
{
  "private": true,
  "dependencies": {"react": "19.1.0", "react-dom": "19.1.0"},
  "devDependencies": {"typescript": "5.8.3"}
}
APP
```

Add a source file that uses React:

```sh
cat > "$radar_demo/app/src/App.tsx" <<'APP'
import React from "react";

export default function App() {
  return <h1>Example application</h1>;
}
APP
```

The manifest establishes declared dependencies. The import gives the scanner separate evidence that React is wired into application source. These are different observations and both remain in the evidence.

## 2. Generate the report

```sh
./tech-radar generate --repo "$radar_demo/app" --name "Example UI" \
  --format html --out "$radar_demo/reports/RADAR.html"
```

The command prints a summary with this shape:

```text
Generated .../reports/RADAR.html: 2 entries, 0 policy exclusions, 0 scan warnings
```

Open `RADAR.html` in your browser. The radar has entries for React and TypeScript. React is suggested for Adopt because the scanner found runtime wiring in source. TypeScript is suggested for Assess because its generic language evidence does not establish the same wiring signal. You can record a different team decision after review.

Each marker has an outer circle on this first report. That means the entry is new to this radar, not that your application recently adopted it. Hover or keyboard-focus a marker for details. Select its number to jump to the corresponding entry.

The report follows your system's light or dark appearance. Its Theme control offers an explicit choice, and print output uses a light background.

## 3. Inspect the evidence

The report links its full evidence file:

```text
reports/
  RADAR.html
  RADAR.evidence.<sha256>.json
```

The JSON records relative source locations, dependency descriptions, language counts, proposed placements, and team decisions. `react` contains evidence for the recognized React package aliases, including `react-dom`. Aliases of one technology are distinct from a group of several technology findings.

You also see `.lock` files beside generated outputs. They coordinate writers and stay on disk after a run. Leave them in place while a process may be using that output.

For JSON on stdout instead of a human report:

```sh
./tech-radar scan --repo "$radar_demo/app"
```

The scan contains two findings and language statistics for the JSON manifest and TypeScript source. No source code or credential values are copied into the evidence descriptions.

## 4. Generate Markdown for a PR

```sh
./tech-radar generate --repo "$radar_demo/app" --name "Example UI" \
  --out "$radar_demo/reports/RADAR.md"
```

Markdown contains a matrix and main-findings table instead of the interactive diagram. It links full evidence just like HTML. Commit the document and the evidence file it names. Markdown and HTML exports with the same report data and basename can share that evidence file.

To share one file, add `--self-contained` when generating Markdown or HTML. That embeds the full evidence and increases the document size.

## A real repository

[tech-radar scans itself](../guides/tech-radar-example/index.md) shows a populated radar from this project, with the raw scan and the display decisions used to name and place its libraries.

## Next step

[Review the proposed placements and record team decisions](../guides/review-and-update.md). For your own project, replace `--repo` with its checkout path and choose an output directory outside the source you are scanning.
