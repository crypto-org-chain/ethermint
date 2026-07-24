# AGENTS.md

Guidance for AI coding agents (and humans) working in this repository. Keep this
file up to date — a Claude Code agent hook runs on `gh pr create`, checks this
file against the branch diff, and auto-corrects any stale facts (see the
`PreToolUse` hook in `.claude/settings.json`).

## Project

Ethermint is an EVM-compatible Cosmos SDK library. `evmd/` is the example chain
binary; the reusable modules live under `x/`, `rpc/`, `server/`, `ante/`, and
`types/`.

- Language: Go (see `go.mod` for the required version).
- Build system: `Makefile`.

## Build, Test, Lint

Run these from the repo root:

| Task            | Command              |
| --------------- | -------------------- |
| Build binaries  | `make build`         |
| Unit tests      | `make test-unit`     |
| Race tests      | `make test-race`     |
| Lint            | `make lint`          |
| Auto-fix lint   | `make lint-fix`      |
| Format code     | `make format`        |
| Regenerate proto| `make proto-gen`     |
| Vuln scan       | `make vulncheck`     |

Prefer running a focused package test while iterating, e.g.
`go test ./x/evm/... -run TestName`.

## Conventions

- Follow the [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md)
  for all Go code. Highlights:
  - **Errors**: `errors.New` for static messages, `fmt.Errorf` for dynamic ones;
    wrap with `%w` so callers can use `errors.Is`/`errors.As`. Exported error
    vars use the `Err` prefix; custom error types use the `Error` suffix. Handle
    each error only once — don't both log and return it.
  - **Naming**: `MixedCaps`, no underscores in package names; prefix unexported
    globals with `_`. Keep initialisms consistent (`ID`, `URL`, `HTTP`).
  - **Interfaces & receivers**: accept interfaces, return concrete types; assert
    compliance with `var _ Iface = (*T)(nil)`. Be consistent about pointer vs.
    value receivers per type.
  - **Structs**: use field names on init, omit zero-value fields, give `make()`
    a capacity hint when the size is known, and return `nil` (not `[]T{}`) for
    empty slices.
  - **Concurrency**: no fire-and-forget goroutines and never spawn one in
    `init()`; give background workers a clear shutdown path.
  - **Control flow**: reduce nesting by handling errors and edge cases early.
- Imports: standard library first, then third-party, then local — `goimports`
  enforces the grouping (`make format`).
- Add a `CHANGELOG.md` entry for user-facing changes (CI reminds you).
- PR titles must follow [Conventional Commits](https://www.conventionalcommits.org/)
  (enforced by `lint-pr.yml`).

## Before Opening a PR

1. `make format`
2. `make lint`
3. `make test-unit`
4. Update `CHANGELOG.md` if the change is user-facing.
5. Update this `AGENTS.md` if agent-relevant workflow, tooling, or conventions
   changed.
