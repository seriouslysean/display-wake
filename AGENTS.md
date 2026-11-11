# AGENTS.md

display-wake is a macOS launchd agent: it wakes the display once when macOS defers a monitor reconnect. [docs/how-it-works.md](docs/how-it-works.md) explains the loop it ends.

## Commands

- Install: none beyond Go 1.26 or later; standard library only.
- Before a PR: `scripts/check` passes (gofmt -s, go vet, go test). CI's `test` job runs the same script.
- `git config core.hooksPath .githooks` once per clone runs it before every push.

## Where things live

- `internal/wake`: parsing log lines and deciding when to wake; tests and real log fixtures in `testdata/`.
- `cmd/display-wake`: the macOS commands (`log`, `caffeinate`, `ioreg`, `pmset`, `launchctl`) and the plist.

## Rules

- Before writing or changing a test, read https://github.com/splitleafhq/.github/blob/main/docs/testing.md.
- Fixtures are real `log stream --style compact` lines. Add one when macOS changes its wording, and keep the old ones.
- Contributing, commits and PRs follow the shared [CONTRIBUTING.md](https://github.com/seriouslysean/.github/blob/main/CONTRIBUTING.md).
- No personal data, machine names or home paths in code, tests, docs or fixtures; examples use `test-` names.
