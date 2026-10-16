# sqlmux demo video — storyboard and capture notes

Two deliverables from the same tapes (`capture/*.tape`, recorded with `capture/record.sh`):

1. **README GIF**, ~20 s, 1600×900, no captions: scene 1 (SQLite tour) cut to the table + WHERE + JSON view, or scenes 1 + 3 back to back. Keep it under ~5 MB for GitHub.
2. **Feature tour**, 60–90 s, 16:9 1920×1080, for reddit (r/commandline, r/PostgreSQL, r/golang) and the GitHub README/Release: the five scenes below with a title card and one caption per scene. No voice-over; music optional (licensed only).

Facts only (AGENTS.md): no speed numbers, no "fastest", no user counts. What sqlmux is: a terminal database client organised like tmux (sessions / windows / panes), for PostgreSQL, MySQL, SQLite and Redis, with a vim editor diff-tested against nvim, every key rebindable.

## Scenes

| # | Tape | Caption (one line on screen) | What the viewer sees | Status |
|---|---|---|---|---|
| 1 | `sqlite.tape` | One client for PostgreSQL, MySQL, SQLite and Redis | `sqlmux sqlite` opens a file; `/` filters the tree, `↵` opens `t_order`; WHERE `status = 'done'`; `]` twice pages to 3/12 (the default keys; `<` / `>` are the user's own binding); `gj` JSON view; `s` + `am` flash-jumps | recorded, frames checked |
| 2 | `edit.tape` | Edit in place, save as one transaction | PG `t_order`: `i` edits `amount`, `↵` on `status` opens the enum picker, `C-s` saves both rows, toast `Saved 2 row(s)` | written, needs docker (PG) — check the picker and the toast in the frames |
| 3 | `query.tape` | A vim editor that completes your enums | query pane: typing `where o.status = ` lists the enum values; `SPC ll` formats; `↵` runs; results below; `gj` | written, needs docker |
| 4 | `tmux.tape` | tmux inside: sessions, windows, panes, a palette | SQLite: `C-p` `@order` opens `t_order` from the palette; `;` runs quick SQL `select count(*) from t_user` in the palette (table completion, result inline); `SPC %` splits, `c` a query, `↵` runs it and the result opens below (three panes); `SPC c` a new window, `SPC n` back; a new session "reports" from the palette, `SPC )` switches back | recorded, frames checked |
| 5 | `redis.tape` | Redis too: keys as tables, a console that knows the commands | tree by namespace; `hash:user` as a table, edit `name`, `C-s`; console `HGETALL hash:user` with the signature line, reply as a table; `SET … EX ` shows the tokens; `ZRANGE … WITHSCORES` as member / score | recorded, frames checked |

Suggested cut: title card (sqlmux, one line what it is, `brew install kackerx/tap/sqlmux`) → 1 → 4 → 3 → 2 → 5 (3 and 2 only once the PG scenes are recorded) → end card (GitHub URL, `go install` / `brew` / `npm` lines from the README). Trim the tapes' leading second and the long pauses; HyperFrames does the captions, not the terminal.

## Recording

- `capture/record.sh [name…]`: stages `/tmp/sqlmux-demo` (demo config from `config/`, `play.db`, a `bin/sqlmux` symlink, fresh data / state dirs so nothing is restored), reseeds Redis db 2 / 3 before the redis tape, then runs `vhs`. Output in `capture/renders/` (git-ignored). `SQLMUX=/path/to/sqlmux` picks the binary (the acceptance build at `/Users/ctw/proj/sqlmux-verify/bin/sqlmux` or `just build`).
- Databases: PG and MySQL are docker compose in the main worktree (`just db-up`; OrbStack must be running); the demo connects to `sqlmux_play` (`play` / `sqlmux`), which may need rebuilding from `testdata/seed/pg.sql` (`createdb sqlmux_play` + psql). Redis is a local `redis-server --port 56379 --save "" --appendonly no --daemonize yes`, seeded by `record.sh`. SQLite is `play.db` here (a copy of the seed).
- Font `Maple Mono NF CN` (installed), 18 px, 1600×900, padding 12; the theme is `ristretto` (copied into `config/`). Keep these for every tape so the cuts match.
- VHS notes: in the palette's SQL scope the first `↵` accepts the completion and the second runs (`Wait+Screen` on the typed text matches too early: use a `Sleep`); the 3-key leader chords (`SPC wC`) and `tC` did not fire under VHS while `SPC %` / `SPC c` / `SPC )` did, so the session is created from the palette; output paths must not start with a digit; `Type` with `$`/quotes goes in backticks; `Ctrl+s` does not parse, `Ctrl+S` does and sends C-s; `Escape` then `Sleep 100ms` before the next key; `Wait+Screen /text/` waits for the screen, not the clock.
- README GIF (`assets/demo.gif`, ~29 s, 1200 px, ~3 MB): `SQLMUX=$(brew --prefix)/bin/sqlmux capture/record.sh sqlite tmux`, then from `capture/renders/`: `ffmpeg -ss 0.8 -i sqlite.mp4 -ss 0.8 -i tmux.mp4 -filter_complex "[0:v][1:v]concat=n=2:v=1[c];[c]setpts=PTS/1.5,fps=10,scale=1200:-1:flags=lanczos,split[a][b];[a]palettegen=max_colors=96:stats_mode=diff[p];[b][p]paletteuse=dither=none:diff_mode=rectangle" ../../../demo.gif`. Redo it every release.
- After recording, pull frames (`ffmpeg -ss N -i renders/x.mp4 -frames:v 1 f.png`) and check: no glyph boxes, no real paths or credentials (the status bar shows `/tmp/sqlmux-demo/play.db` and `127.0.0.1:56379` — both fine), every caption moment on screen long enough.

## Handoff

- Video: a `claude --agent video-producer` session in this repo; this file is its brief, the tapes its footage. Composition and renders live under `assets/demo/video/` (renders git-ignored).
- README / docs: the oss role (`claude -n sqlmux-oss --agent oss-maintainer --effort high`, rules in `specs/roles/oss.md`) embeds the GIF under the first paragraph, writes the quickstart (install → connections.toml → first table), the keys reference from `sqlmux keys --format toml`, and the config reference from `config.toml`'s comments; the reddit post text goes to the user before posting.
