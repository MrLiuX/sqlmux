#!/usr/bin/env bash
# Records one or more tapes into renders/: ./record.sh sqlite redis …  (names without .tape; none = all).
# Needs vhs, ttyd, the "Maple Mono NF CN" font and sqlmux on PATH (SQLMUX=/path/to/binary overrides);
# the PG / MySQL tapes need docker compose up in the main worktree, the redis tape a redis-server on 56379
# seeded with testdata/seed/redis.txt into db 2 (see ../STORYBOARD.md).
set -euo pipefail
cd "$(dirname "$0")"
bin=${SQLMUX:-$(command -v sqlmux)}
names=("$@"); [[ ${#names[@]} -eq 0 ]] && names=($(ls *.tape | sed 's/\.tape$//'))
for n in "${names[@]}"; do
  rm -rf /tmp/sqlmux-demo && mkdir -p /tmp/sqlmux-demo/bin   # fresh per tape: no last.toml, no history; the paths on screen are /tmp/sqlmux-demo
  cp -R ../config /tmp/sqlmux-demo/config && cp ../play.db /tmp/sqlmux-demo/ && ln -s "$bin" /tmp/sqlmux-demo/bin/sqlmux
  if [[ $n == redis ]]; then   # the seed again into db 2 / 3, so every recording edits the same data
    redis-cli -p 56379 -n 2 flushdb >/dev/null; redis-cli -p 56379 -n 3 flushdb >/dev/null
    sed -e 's/^SELECT 1$/SELECT 3/' -e 's/^SELECT 0$/SELECT 2/' ../../../testdata/seed/redis.txt | redis-cli -p 56379 -n 2 >/dev/null
  fi
  vhs "$n.tape"
done
