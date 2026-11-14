# markdown

A public Go library that renders GitHub Flavored Markdown in the terminal, with the kitty protocols for heading sizes, images and Mermaid diagrams, and a text fallback everywhere else. The README lists the packages.

Its first users are rexkit and the apps built on it, which run in Rex tabs, so check output in Rex as well as in kitty (`cmd/mdview`).

## Rules

- **Public repo.** No internal hostnames, IPs, accounts or tokens. Not in code, tests, test data, docs or commit messages.
- **Generic code only.** No package imports an app or rexkit. An app-specific need becomes an option or a hook, not a special case.
- **No network.** The library never fetches anything itself: images come through the caller's fetch function. Tests never reach the network.
- **The core doesn't import Bubble Tea.** Only `teamd` and `cmd/mdview` do.
- **Imports:** Charm v2 (`charm.land/lipgloss/v2`, `charm.land/bubbletea/v2`) and `github.com/charmbracelet/x/ansi`. Never the v1 `github.com/charmbracelet/...` paths.
- **Pictures as text.** Kitty images use Unicode placeholders only, never direct placements. Pictures are sent from `Prepare`, never while rendering a view, and deleted on `Close`.
- **Tests:** `mise run check` before every commit (gofmt, vet, tests). Rendered output has golden files in `testdata/`; rewrite them with `go test ./... -update` and read the diff before committing.
- **Output changes are breaking for apps.** Apps compare screen snapshots byte for byte. A change to what the renderer draws is a `feat` with a note in the PR, or `feat!` when callers must change.
- **Dependency versions:** keep `chroma` and `goldmark` in step with rexkit and the apps. A version bump can change highlighted output.

## Commits and releases

- **Conventional Commits** for every commit and PR title (`feat:`, `fix:`, `docs:`, `chore:`, `ci:`, `test:`, `refactor:`). PRs are squash-merged, so the PR title is the changelog line.
- Name the Linear issue in the body (`Part of NIN-12` / `Fixes NIN-12`). The Linear project is markdown.
- **Releases:** release-please keeps a release PR open, and merging it tags `vX.Y.Z`. Pre-1.0, `feat` bumps the minor version. It's a library, so nothing is built or uploaded.
- **Actions** are pinned to commit SHAs with the version as a comment. Renovate keeps them current.
