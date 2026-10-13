# sqlmux

A terminal database client organized the way tmux is: sessions, windows, panes and tabs. vim keys everywhere, mouse too. Written in Go with Bubble Tea v2.

![sqlmux](assets/screenshot.png)

> **Status: early development.** PostgreSQL and MySQL. The UI is in English; `language = "zh"` in `config.toml` switches it to Chinese. Milestones are tracked in [specs/plan.md](specs/plan.md), and the design is in [specs/tech-design.md](specs/tech-design.md) (Chinese).

## What works today

- Schema tree: schemas → tables / views → columns, plus the open workspace.
- Table tabs: WHERE with completion and history, ORDER, LIMIT, paging, column picker, transpose.
- Inline editing: edit cells, then save everything in one transaction. Enum, boolean and time columns get pickers.
- SQL queries: a vim editor that is diff-tested against nvim, with completion, formatting, a schema picker, and results in a bottom pane.
- Command palette (`C-p`) and quick SQL (`;`).
- Every key can be rebound, per pane type; `SPC` is the leader. Themes and Nerd Font icons are configurable (`icons = "ascii"` if you have no Nerd Font).

## Build

Homebrew: `brew install kackerx/tap/sqlmux`. npm (no Go needed, the binary comes as a platform package): `npm install -g sqlmux`, or `npx sqlmux` to try it. With Go 1.26+: `go install github.com/kackerx/sqlmux/cmd/sqlmux@latest` installs the latest tagged version (v0.2.0 and later). Each release on the [Releases page](https://github.com/kackerx/sqlmux/releases) also has the binaries for macOS and Linux (amd64 and arm64) as `sqlmux_<version>_<os>_<arch>.tar.gz`, with `checksums.txt` to verify them. To build from a checkout: with [just](https://github.com/casey/just) installed, `just build` puts the binary in `bin/`; `just --list` shows the other recipes (tests, lint, e2e, the sample databases). Without it:

```sh
go build -o bin/sqlmux ./cmd/sqlmux
```

## Try it

Start the bundled PostgreSQL with sample data:

```sh
just db-up            # or: docker compose up -d --wait postgres
```

Run `bin/sqlmux`. With no connections configured it opens a form (name, engine, host, port, user, password, database); the `+` at the top of the sidebar opens the same form later. Passwords entered there go to the OS keychain (macOS Keychain, or `secret-tool` on Linux desktops) and `connections.toml` only gets the command that reads them back.

You can also write `~/.config/sqlmux/connections.toml` by hand:

```toml
[[connection]]
name     = "seed"
engine   = "postgres"           # postgres | mysql
dsn      = "postgres://sqlmux@localhost:55432/sqlmux?sslmode=disable"
password = "sqlmux"
```

Where the password comes from, first one set wins:

| field | example | notes |
|---|---|---|
| `password_cmd` | `"security find-generic-password -s sqlmux-seed -w"` | any command that prints it: `pass`, `op`, `bw`, `secret-tool` |
| `password_env` | `"SEED_PW"` | name of an environment variable |
| `password` | `"sqlmux"` | plain text; your call, sqlmux neither warns nor changes the file's permissions |
| in the DSN / `~/.pgpass` | `postgres://u:p@…` | handled by the driver |

`${VAR}` inside `dsn` or `password` is replaced from the environment, so `dsn = "${DATABASE_URL}"` works too; each connection names its own variables.

`bin/sqlmux <name>` opens that connection; without a name it restores the layout you quit from. Press `C-c` twice to quit.

## Keys

`sqlmux keys` lists every action and its keys; Keys live in `~/.config/sqlmux/keymaps.toml`: `sqlmux keys --format toml` prints a full one, with the defaults commented out and your changes as live lines. `sqlmux keys --check` reports conflicts in it.

## License

[MIT](LICENSE)
