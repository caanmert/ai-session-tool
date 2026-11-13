# ai-session-tool

## Workflow

- **Board:** [ai-session-tool](https://github.com/users/caanmert/projects/5) with columns Todo → In Progress → In Review → Done. Keep issues open and In Review while their PR is unmerged; Done follows merge and satisfied acceptance criteria.
- **Base branch:** `main`. Create one branch per issue, named `<n>-<short-kebab-slug>`, and open PRs against `main`. Never commit to `main` directly.
- **Labels:** `bug`, `enhancement`, `documentation`. GitHub's other default labels (`question`, `help wanted`, `good first issue`, `duplicate`, `invalid`, `wontfix`, `accessibility`) are also available.
- **Product direction:** see [the desktop roadmap](docs/ROADMAP.md) and [roadmap issue #11](https://github.com/caanmert/ai-session-tool/issues/11). Keep feature work scoped to its child issue and validate interactions between related PRs before calling them ready.

## Checks

Before calling a change done, run what CI runs (`.github/workflows/ci.yml`) for the parts you touched:

- **CLI (repo root):** `go vet ./...` and `go test -race ./...`. CI also lints with golangci-lint v2.5.
- **Desktop app:** the Go code embeds `desktop/frontend/dist`, so build the frontend first. In `desktop/frontend`, run `npm ci`, `npm test` and `npm run build`. Then in `desktop/`, run `go vet ./...` and `go test -race ./...`.
- **Packaging:** `python3 -m unittest discover -s scripts -v`.
- **Live agent checks:** the authenticated checks of the desktop chat backends are opt-in and use real model usage. See `docs/DESKTOP.md`.
